package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

type PredictiveHandler struct {
	engine *core.PredictiveEngine
}

func NewPredictiveHandler(engine *core.PredictiveEngine) *PredictiveHandler {
	return &PredictiveHandler{engine: engine}
}

func (h *PredictiveHandler) Demand(w http.ResponseWriter, r *http.Request) {
	var req domain.PredictiveDemandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo JSON da requisição é inválido").WriteJSON(w)
		return
	}

	if req.TenantID == "" || req.CampaignID == "" {
		domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "tenant_id e campaign_id são obrigatórios").WriteJSON(w)
		return
	}

	resp, err := h.engine.ProcessDemand(r.Context(), &req)
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
