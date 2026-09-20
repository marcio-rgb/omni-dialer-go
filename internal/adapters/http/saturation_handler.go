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

// GetIndividual retorna as métricas de penetração e saturação de uma campanha específica.
//
// @pattern Adapter (HTTP Handler / Query)
// @governedBy docs/rules/CAMPAIGN_SATURATION.md#3-escala-de-5-níveis-de-saturação-de-campanha
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de identificadores obrigatórios (`tenant_id`, `campaign_id`)
//
// @postExecution
// - Cálculo dinâmico do nível de saturação (NOVA, OPERACIONAL, ALERTA, CRITICA, ESGOTADA)
// - Resposta padronizada com status HTTP 200 OK
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

// GetConsolidated consolida os níveis de saturação de todas as campanhas ativas do tenant.
//
// @pattern Adapter (HTTP Handler / Batch Query)
// @governedBy docs/rules/CAMPAIGN_SATURATION.md#3-escala-de-5-níveis-de-saturação-de-campanha
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de `tenant_id` obrigatório via header ou query param
//
// @postExecution
// - Agregação em passada única no PostgreSQL
// - Retorno de lista estruturada de campanhas e seus níveis de saturação
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
