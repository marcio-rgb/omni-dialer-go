package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

type ReportHandler struct {
	repo  ports.ReportRepository
	cache ports.CachePort
}

func NewReportHandler(repo ports.ReportRepository, cache ports.CachePort) *ReportHandler {
	return &ReportHandler{
		repo:  repo,
		cache: cache,
	}
}

// GetCallsSummary manipula requisições de métricas de chamadas com buffer de cache estrito de 15 minutos.
//
// @pattern Adapter (HTTP Handler / Cached Reporting)
// @governedBy docs/rules/CAMPAIGN_SATURATION.md#4-política-de-cache-de-15-minutos-para-métricas-operacionais
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de `tenant_id` e janela temporal ISO 8601 (`start_date`, `end_date`)
// - Verificação de cache no Redis por chave SHA-256 com TTL de 900 segundos
//
// @postExecution
// - Agregação em passada única na tabela `cdrs` no PostgreSQL
// - Gravação do resultado no cache Redis por 15 minutos
// - Resposta em formato RFC 7807 em caso de erro ou payload consolidado
func (h *ReportHandler) GetCallsSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Extrai tenant_id obrigatório (query param ou header)
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}

	if tenantID == "" {
		errResp := domain.NewErrBadRequest("MISSING_TENANT_ID",
			"O parâmetro obrigatório 'tenant_id' não foi informado via query parameter (?tenant_id=...) nem via header HTTP (X-Tenant-Id).")
		errResp.WriteJSON(w)
		return
	}

	// 2. Extrai e normaliza start_date e end_date opcionais
	var startDate, endDate time.Time
	var err error

	startDateStr := r.URL.Query().Get("start_date")
	if startDateStr != "" {
		startDate, err = time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			errResp := domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'start_date' deve ser ISO 8601 UTC (ex: 2026-09-09T00:00:00Z)",
				domain.InvalidParam{Name: "start_date", Reason: "formato inválido"})
			errResp.WriteJSON(w)
			return
		}
	} else {
		// Default: Início do dia atual 00:00:00 UTC
		now := time.Now().UTC()
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	endDateStr := r.URL.Query().Get("end_date")
	if endDateStr != "" {
		endDate, err = time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			errResp := domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'end_date' deve ser ISO 8601 UTC (ex: 2026-09-09T06:30:00Z)",
				domain.InvalidParam{Name: "end_date", Reason: "formato inválido"})
			errResp.WriteJSON(w)
			return
		}
	} else {
		// Default: Instante atual
		endDate = time.Now().UTC()
	}

	if startDate.After(endDate) {
		errResp := domain.NewErrUnprocessable("INVALID_DATE_RANGE",
			fmt.Sprintf("A data inicial 'start_date' (%s) não pode ser posterior à data final 'end_date' (%s).",
				startDate.Format(time.RFC3339), endDate.Format(time.RFC3339)),
			domain.InvalidParam{Name: "start_date", Reason: "posterior a end_date"})
		errResp.WriteJSON(w)
		return
	}

	var campaignIDPtr *string
	campIDStr := r.URL.Query().Get("campaign_id")
	if campIDStr != "" {
		campaignIDPtr = &campIDStr
	}

	// 3. Gera hash determinístico dos parâmetros para o buffer
	campKey := "all"
	if campaignIDPtr != nil {
		campKey = *campaignIDPtr
	}
	rawHash := fmt.Sprintf("%s|%s|%s", startDate.Format(time.RFC3339), endDate.Format(time.RFC3339), campKey)
	hasher := sha256.New()
	hasher.Write([]byte(rawHash))
	hashKey := hex.EncodeToString(hasher.Sum(nil))

	// 4. Consulta o buffer no Redis (15 minutos = 900s)
	cachedData, ttlRemaining, err := h.cache.GetCallsSummaryBuffer(ctx, tenantID, hashKey)
	if err == nil && cachedData != nil {
		// CACHE HIT: Entrega instantânea da memória (< 1ms)
		cachedData.CacheMeta.Cached = true
		cachedData.CacheMeta.TTLRemainingSeconds = int64(ttlRemaining.Seconds())
		cachedData.CacheMeta.LastRefreshReason = "BUFFER_HIT"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    cachedData,
		})
		return
	}

	// 5. CACHE MISS: Consulta o PostgreSQL dialer_db com agregação em passada única
	freshData, err := h.repo.GetCallsSummary(ctx, tenantID, startDate, endDate, campaignIDPtr)
	if err != nil {
		errResp := domain.NewErrInternal(fmt.Sprintf("Falha ao processar agregação analítica no banco: %s", err.Error()))
		errResp.WriteJSON(w)
		return
	}

	// 6. Preenche metadados do buffer e salva no Redis com TTL = 900s (15 min)
	now := time.Now().UTC()
	bufferTTL := 900 * time.Second
	freshData.CacheMeta = domain.CacheMetaDTO{
		Cached:                 false,
		CachedAt:               now.Format(time.RFC3339),
		CacheExpiresAt:         now.Add(bufferTTL).Format(time.RFC3339),
		TTLRemainingSeconds:    900,
		BufferDurationSeconds:  900,
		RefreshIntervalMinutes: 15,
		LastRefreshReason:      "FRESH_CALCULATION",
	}

	_ = h.cache.SetCallsSummaryBuffer(ctx, tenantID, hashKey, freshData, bufferTTL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    freshData,
	})
}

// ListCDRs lista registros detalhados de CDR com paginação e filtros operacionais.
//
// @pattern Adapter (HTTP Handler / REST Collection)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de `tenant_id` obrigatório (query param ou header X-Tenant-Id)
// - Sanitização de paginação (page >= 1, 1 <= limit <= 100)
//
// @postExecution
// - Consulta paginada no PostgreSQL com ordenação decrescente por created_at
// - Resposta em formato RFC 7807 em caso de erro ou payload consolidado
func (h *ReportHandler) ListCDRs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Tenant ID obrigatório
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID",
			"O parâmetro obrigatório 'tenant_id' não foi informado via query parameter (?tenant_id=...) nem via header HTTP (X-Tenant-Id).").WriteJSON(w)
		return
	}

	filter := domain.CDRFilter{
		TenantID: tenantID,
		Page:     1,
		Limit:    20,
	}

	// 2. Paginação
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p >= 1 {
			filter.Page = p
		}
	}
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l >= 1 {
			if l > 100 {
				l = 100
			}
			filter.Limit = l
		}
	}

	// 3. Filtros opcionais
	if campID := r.URL.Query().Get("campaign_id"); campID != "" {
		filter.CampaignID = &campID
	}
	if phone := r.URL.Query().Get("phone"); phone != "" {
		filter.Phone = &phone
	}
	if disp := r.URL.Query().Get("disposition"); disp != "" {
		d := domain.CallDisposition(disp)
		filter.Disposition = &d
	}
	if q := r.URL.Query().Get("q"); q != "" {
		filter.Search = &q
	} else if search := r.URL.Query().Get("search"); search != "" {
		filter.Search = &search
	}

	if startStr := r.URL.Query().Get("start_date"); startStr != "" {
		if st, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartDate = &st
		} else {
			domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'start_date' deve ser ISO 8601 UTC (ex: 2026-09-09T00:00:00Z)",
				domain.InvalidParam{Name: "start_date", Reason: "formato inválido"}).WriteJSON(w)
			return
		}
	}
	if endStr := r.URL.Query().Get("end_date"); endStr != "" {
		if et, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndDate = &et
		} else {
			domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'end_date' deve ser ISO 8601 UTC (ex: 2026-09-09T06:30:00Z)",
				domain.InvalidParam{Name: "end_date", Reason: "formato inválido"}).WriteJSON(w)
			return
		}
	}

	if filter.StartDate != nil && filter.EndDate != nil && filter.StartDate.After(*filter.EndDate) {
		domain.NewErrUnprocessable("INVALID_DATE_RANGE",
			"A data inicial 'start_date' não pode ser posterior à data final 'end_date'.",
			domain.InvalidParam{Name: "start_date", Reason: "posterior a end_date"}).WriteJSON(w)
		return
	}

	// 4. Executa listagem
	res, err := h.repo.ListCDRs(ctx, filter)
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao consultar CDRs: %s", err.Error())).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    res,
	})
}

// GetCDR obtém os detalhes de um CDR específico por ID.
//
// @pattern Adapter (HTTP Handler / REST Resource)
func (h *ReportHandler) GetCDR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID",
			"O parâmetro obrigatório 'tenant_id' não foi informado via query parameter nem via header HTTP.").WriteJSON(w)
		return
	}

	cdrID := chi.URLParam(r, "id")
	if cdrID == "" {
		domain.NewErrBadRequest("MISSING_CDR_ID", "O identificador 'id' do CDR é obrigatório na URL.").WriteJSON(w)
		return
	}

	cdr, err := h.repo.GetCDRByID(ctx, tenantID, cdrID)
	if err != nil {
		domain.NewErrNotFound("CDR_NOT_FOUND", fmt.Sprintf("Registro de CDR '%s' não encontrado.", cdrID)).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    cdr,
	})
}
