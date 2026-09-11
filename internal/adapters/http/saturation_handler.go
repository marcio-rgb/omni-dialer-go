package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"github.com/go-chi/chi/v5"
)

type SaturationHandler struct {
	service *core.SaturationService
}

func NewSaturationHandler(service *core.SaturationService) *SaturationHandler {
	return &SaturationHandler{service: service}
}

func (h *SaturationHandler) GetIndividual(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaign_id")
	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}

	if campaignID == "" || tenantID == "" {
		domain.NewErrBadRequest("MISSING_IDENTIFIER", "campaign_id e tenant_id são obrigatórios").WriteJSON(w)
		return
	}

	data, err := h.service.GetCampaignSaturation(r.Context(), tenantID, campaignID)
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
		"data":    data,
	})
}

func (h *SaturationHandler) GetConsolidated(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}

	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "tenant_id é obrigatório via header ou query").WriteJSON(w)
		return
	}

	data, err := h.service.GetConsolidatedSaturation(r.Context(), tenantID)
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
		"data":    data,
	})
}
