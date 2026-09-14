package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

// LeadBatchHandler orquestra a ingestão direta de leads via JSON com pré-renderização de áudio para nomes.
//
// @pattern Adapter (HTTP Handler)
// @governedBy docs/rules/CAMPAIGN_SATURATION.md
type LeadBatchHandler struct {
	leadRepo     ports.LeadRepository
	cache        ports.CachePort
	audioWordMgr *core.AudioWordManager
}

// NewLeadBatchHandler instancia o controlador de carga de leads em lote.
func NewLeadBatchHandler(leadRepo ports.LeadRepository, cache ports.CachePort, audioWordMgr *core.AudioWordManager) *LeadBatchHandler {
	return &LeadBatchHandler{
		leadRepo:     leadRepo,
		cache:        cache,
		audioWordMgr: audioWordMgr,
	}
}

// IngestBatch processa a ingestão de lote de leads, sintetizando nomes inexistentes com acentos e salvando chaves normalizadas.
//
// @pattern Adapter (HTTP Handler)
// @governedBy docs/rules/CAMPAIGN_SATURATION.md
//
// @preExecution
// - Autorização de IP via IPWhitelistMiddleware
// - Validação da campanha e array de leads
//
// @postExecution
// - Pré-síntese no Piper TTS com texto original para nomes ausentes no cache
// - Inserção em batch no PostgreSQL com status 'NEW'
// - Enfileiramento no Redis para o discador preditivo com LeadQueueItem
func (h *LeadBatchHandler) IngestBatch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	campaignID := chi.URLParam(r, "campaign_id")

	var req domain.BatchLeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	if campaignID != "" {
		req.CampaignID = campaignID
	}

	if req.CampaignID == "" || req.TenantID == "" {
		domain.NewErrBadRequest("MISSING_FIELDS", "campaign_id e tenant_id são obrigatórios").WriteJSON(w)
		return
	}

	if len(req.Leads) == 0 {
		domain.NewErrBadRequest("EMPTY_LEADS", "A lista de leads não pode ser vazia").WriteJSON(w)
		return
	}

	ctx := r.Context()
	var dbLeads []*domain.Lead
	var queuePayloads []string
	newSynthesized := 0
	cachedAudios := 0

	for _, item := range req.Leads {
		phone := strings.TrimSpace(item.Phone)
		cpf := strings.TrimSpace(item.CPF)
		name := strings.TrimSpace(item.Name)

		if len(phone) < 8 {
			continue
		}

		// 1. Pré-síntese de áudio para o nome (se fornecido)
		if name != "" && h.audioWordMgr != nil {
			audioKey, isNew, err := h.audioWordMgr.EnsureNameAudio(ctx, name)
			if err != nil {
				// Loga aviso mas não interrompe a importação do lead
				audioKey = domain.Slugify(name)
			}
			item.AudioKey = audioKey
			if isNew {
				newSynthesized++
			} else {
				cachedAudios++
			}
		}

		// 2. Monta struct para PostgreSQL
		dbLeads = append(dbLeads, &domain.Lead{
			CampaignID:    req.CampaignID,
			TenantID:      req.TenantID,
			CPF:           cpf,
			Phone:         phone,
			Name:          name,
			Status:        domain.LeadStatusNew,
			AttemptsCount: 0,
		})

		// 3. Monta payload para o Redis do discador preditivo
		qItem := domain.LeadQueueItem{
			Phone: phone,
			CPF:   cpf,
			Name:  name,
		}
		b, _ := json.Marshal(qItem)
		queuePayloads = append(queuePayloads, string(b))
	}

	if len(dbLeads) == 0 {
		domain.NewErrBadRequest("NO_VALID_LEADS", "Nenhum lead com telefone válido na requisição").WriteJSON(w)
		return
	}

	// 4. Persiste no PostgreSQL em batch
	if h.leadRepo != nil {
		if _, err := h.leadRepo.BatchInsert(ctx, dbLeads); err != nil {
			domain.NewErrInternal(fmt.Sprintf("Falha ao persistir leads no banco de dados: %s", err.Error())).WriteJSON(w)
			return
		}
	}

	// 5. Enfileira no Redis para o discador preditivo
	if h.cache != nil {
		_ = h.cache.PushLeads(ctx, req.CampaignID, queuePayloads)
	}

	resp := domain.BatchLeadResponse{
		CampaignID:           req.CampaignID,
		TotalReceived:        len(req.Leads),
		LeadsQueued:          len(dbLeads),
		NewAudiosSynthesized: newSynthesized,
		CachedAudiosCount:    cachedAudios,
		ElapsedMs:            time.Since(start).Milliseconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    resp,
	})
}
