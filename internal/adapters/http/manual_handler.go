package http

import (
	"encoding/json"
	"log"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

type ManualHandler struct {
	engine *core.ManualEngine
}

func NewManualHandler(engine *core.ManualEngine) *ManualHandler {
	return &ManualHandler{engine: engine}
}

// DialManual processa uma chamada manual sob demanda para operador humano com prioridade preemptiva.
//
// @pattern Strategy (Context / Manual)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#2-quota-garantida-de-canais-para-operadores-humanos-humanreservequota
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de DTO obrigatório (`tenant_id`, `agent_id`, `phone`, `sip_route`)
// - Alocação prioritária de canal na cota humana reservada (`HumanReserveQuota`) em `ChannelManager`
//
// @postExecution
// - Disparo de comando `Originate` via socket AMI no contexto `from-dialer-manual`
// - Registro de CorrelationID em `execution_traces`
// - Retorno de confirmação com status HTTP 201 Created
func (h *ManualHandler) DialManual(w http.ResponseWriter, r *http.Request) {
	var req domain.ManualCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[PARTNER-IN] [MANUAL-DIAL] [INVALID_JSON] Falha ao decodificar JSON de %s: %v", r.RemoteAddr, err)
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	log.Printf("[PARTNER-IN] [MANUAL-DIAL] Tenant: %q, Agent: %q, Phone: %q, Route: %q, Trunk: %q, LeadName: %q (From: %s)",
		req.TenantID, req.AgentID, req.Phone, req.SIPRoute, req.TrunkID, req.LeadName, r.RemoteAddr)

	if req.TenantID == "" || req.AgentID == "" || req.Phone == "" || req.SIPRoute == "" {
		log.Printf("[PARTNER-IN] [MANUAL-DIAL] [MISSING_FIELDS] Campos obrigatórios ausentes de %s", r.RemoteAddr)
		domain.NewErrBadRequest("MISSING_FIELDS", "Os campos tenant_id, agent_id, phone e sip_route são obrigatórios.").WriteJSON(w)
		return
	}

	resp, err := h.engine.DialManual(r.Context(), &req)
	if err != nil {
		if prob, ok := err.(*domain.ProblemDetails); ok {
			prob.WriteJSON(w)
			return
		}
		domain.NewErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    resp,
	})
}

// Hangup desliga imediatamente uma chamada telefônica em andamento.
//
// @pattern Strategy / Command
// @governedBy docs/rules/TELEPHONY_POLICIES.md
// @preExecution Validação de identificador (call_id, channel, agent_id, phone).
// @postExecution Disparo de Hangup via AMI e resposta HTTP 200 OK.
func (h *ManualHandler) Hangup(w http.ResponseWriter, r *http.Request) {
	var req domain.HangupCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[PARTNER-IN] [HANGUP] [INVALID_JSON] Falha ao decodificar JSON de %s: %v", r.RemoteAddr, err)
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	log.Printf("[PARTNER-IN] [HANGUP] CallID: %q, Channel: %q, Agent: %q, Phone: %q, Cause: %d (From: %s)",
		req.CallID, req.Channel, req.AgentID, req.Phone, req.Cause, r.RemoteAddr)

	resp, err := h.engine.HangupCall(r.Context(), &req)
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

