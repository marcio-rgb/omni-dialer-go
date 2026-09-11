package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type ToggleHandler struct {
	campaigns ports.CampaignRepository
	cache     ports.CachePort
}

func NewToggleHandler(campaigns ports.CampaignRepository, cache ports.CachePort) *ToggleHandler {
	return &ToggleHandler{
		campaigns: campaigns,
		cache:     cache,
	}
}

func (h *ToggleHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	// Timeout estrito de 15 segundos
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var req domain.ToggleCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error": map[string]string{
				"code":    "INVALID_JSON",
				"message": "Payload JSON inválido",
			},
		})
		return
	}

	if req.CampaignID == "" || req.TenantID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error": map[string]string{
				"code":    "MISSING_FIELDS",
				"message": "campaign_id e tenant_id são obrigatórios.",
			},
		})
		return
	}

	targetStatus := domain.CampaignStatusActive
	if !req.Enable {
		targetStatus = domain.CampaignStatusPaused
	}

	// 1. Atualiza persistência relacional
	camp, err := h.campaigns.SetStatus(ctx, req.TenantID, req.CampaignID, targetStatus)
	if err != nil || camp == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error": map[string]string{
				"code":    "CAMPAIGN_NOT_FOUND",
				"message": "A campanha solicitada não existe ou foi removida.",
			},
		})
		return
	}

	// 2. Sincroniza flag rápida no Redis
	_ = h.cache.SetCampaignPaused(ctx, req.CampaignID, !req.Enable)

	nameStr := ""
	if camp.Name != nil {
		nameStr = *camp.Name
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"id":         camp.ID,
			"tenant_id":  camp.TenantID,
			"name":       nameStr,
			"status":     string(camp.Status),
			"created_at": camp.CreatedAt.Format(time.RFC3339),
		},
	})
}
