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

// Demand processa uma rodada de demanda de discagem preditiva calculando pacing e overdialing.
//
// @pattern Strategy (Context / Predictive)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#1-algoritmo-de-pacing-e-equações-de-overdialing
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de DTO obrigatório (`tenant_id`, `campaign_id`)
// - Verificação de pausa e agentes disponíveis em cache Redis
//
// @postExecution
// - Reserva atômica de leads via stored procedure `fn_audit_claim_predictive_batch`
// - Disparo de comandos `Originate` via socket AMI do Asterisk
// - Retorno de status operacional e canais alocados
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

// GetPacing retorna a taxa configurada de chamadas simultâneas por operador disponível.
//
// @pattern Strategy (Context / Predictive)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#1-algoritmo-de-pacing-e-equações-de-overdialing
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
//
// @postExecution
// - Retorna JSON com o valor numérico de min_channels_per_agent atual
func (h *PredictiveHandler) GetPacing(w http.ResponseWriter, r *http.Request) {
	minRatio := h.engine.GetMinChannelsPerAgent()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"min_channels_per_agent": minRatio,
			"description":            "Taxa de chamadas simultaneas disparadas por operador disponivel",
		},
	})
}

// SetPacing atualiza dinamicamente a taxa de chamadas simultâneas por operador disponível.
//
// @pattern Strategy (Context / Predictive)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#1-algoritmo-de-pacing-e-equações-de-overdialing
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de payload numérico: `min_channels_per_agent` deve estar entre 1 e 50
//
// @postExecution
// - Atualização atômica em memória via `engine.SetMinChannelsPerAgent`
// - Resposta RFC 7807 em caso de validação inválida
func (h *PredictiveHandler) SetPacing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MinChannelsPerAgent int `json:"min_channels_per_agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo JSON da requisição é inválido").WriteJSON(w)
		return
	}

	if body.MinChannelsPerAgent < 1 || body.MinChannelsPerAgent > 50 {
		domain.NewErrBadRequest("INVALID_PACING_RATIO", "min_channels_per_agent deve ser um inteiro entre 1 e 50").WriteJSON(w)
		return
	}

	h.engine.SetMinChannelsPerAgent(body.MinChannelsPerAgent)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"min_channels_per_agent": body.MinChannelsPerAgent,
			"message":                "Taxa de discagem por operador disponivel atualizada com sucesso",
		},
	})
}
