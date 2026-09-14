package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

// AudioWordsHandler gerencia as rotas de pré-renderização e preview de áudios
//
// @pattern Adapter (Driver HTTP)
// @governedBy /docs/rules/ARCHITECT.md
type AudioWordsHandler struct {
	manager *core.AudioWordManager
}

// NewAudioWordsHandler instancia o handler HTTP para áudios
//
// @pattern Adapter (Constructor)
// @governedBy /docs/rules/ARCHITECT.md
func NewAudioWordsHandler(manager *core.AudioWordManager) *AudioWordsHandler {
	return &AudioWordsHandler{manager: manager}
}

// UpsertWords recebe o dicionário de frases-base, nomes e convênios e sintetiza em cache
//
// @pattern Adapter (Route Handler)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação do schema JSON através de `req.Validate()`
//
// @postExecution
// - Gravação física dos arquivos WAV ausentes no cache de storage
// - Retorno de métricas com status HTTP 200 OK
func (h *AudioWordsHandler) UpsertWords(w http.ResponseWriter, r *http.Request) {
	var req domain.UpsertAudioWordsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON invalido").WriteJSON(w)
		return
	}

	res, err := h.manager.UpsertWords(r.Context(), req)
	if err != nil {
		domain.NewErrBadRequest("VALIDATION_ERROR", err.Error()).WriteJSON(w)
		return
	}

	type upsertWordsResponseWrapper struct {
		Success bool                            `json:"success"`
		Data    *domain.UpsertAudioWordsResponse `json:"data"`
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(upsertWordsResponseWrapper{
		Success: true,
		Data:    res,
	})
}

// Preview concatena a saudação com o nome e convênio e devolve o WAV binário completo
//
// @pattern Adapter (Route Handler)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Extração de parâmetros via Query String (GET) ou JSON Body (POST)
// - Validação de parâmetros obrigatórios (`name`, `work_word`)
//
// @postExecution
// - Retorno do stream binário com cabeçalho `Content-Type: audio/wav`
func (h *AudioWordsHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var req domain.AudioPreviewRequest

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.Name = q.Get("name")
		if req.Name == "" {
			req.Name = q.Get("first_name")
		}
		req.WorkWord = q.Get("work_word")
		if req.WorkWord == "" {
			req.WorkWord = q.Get("work_words")
		}
		if pStr := q.Get("pausa_ms"); pStr != "" {
			cleanStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(pStr), "ms"))
			if p, err := strconv.Atoi(cleanStr); err == nil {
				req.PausaMs = p
			}
		}
		if incStr := q.Get("include_momentinho"); incStr != "" {
			req.IncludeMomentinho = incStr == "true" || incStr == "1"
		} else if momStr := q.Get("momentinho"); momStr != "" {
			req.IncludeMomentinho = momStr == "true" || momStr == "1"
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			domain.NewErrBadRequest("INVALID_JSON", "Payload JSON invalido").WriteJSON(w)
			return
		}
	}

	wavBytes, err := h.manager.GeneratePreview(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "ausente no banco") {
			domain.NewErrNotFound("WORD_NOT_FOUND", err.Error()).WriteJSON(w)
			return
		}
		domain.NewErrBadRequest("CONCATENATION_ERROR", err.Error()).WriteJSON(w)
		return
	}

	fileName := fmt.Sprintf("preview_%s_%s.wav", req.Name, req.WorkWord)
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.Itoa(len(wavBytes)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(wavBytes)
}
