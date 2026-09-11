package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

type RefillHandler struct {
	processor *core.MailingProcessor
}

func NewRefillHandler(processor *core.MailingProcessor) *RefillHandler {
	return &RefillHandler{processor: processor}
}

func (h *RefillHandler) Refill(w http.ResponseWriter, r *http.Request) {
	var req domain.RefillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	if req.TenantID == "" || req.CampaignID == "" || req.FileURI == "" {
		domain.NewErrBadRequest("MISSING_FIELDS", "tenant_id, campaign_id e file_uri são obrigatórios").WriteJSON(w)
		return
	}

	resp, err := h.processor.ProcessZipRefill(r.Context(), req.TenantID, req.CampaignID, req.FileURI)
	if err != nil {
		if prob, ok := err.(*domain.ProblemDetails); ok {
			prob.WriteJSON(w)
			return
		}
		domain.NewErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    resp,
	})
}
