package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"github.com/go-chi/chi/v5"
)

// QueueHandler gerencia requisições HTTP REST para controle dinâmico de membros de filas telefônicas.
//
// @pattern Strategy / HTTP Controller
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type QueueHandler struct {
	queueMgr *core.AgentQueueManager
}

func NewQueueHandler(queueMgr *core.AgentQueueManager) *QueueHandler {
	return &QueueHandler{queueMgr: queueMgr}
}

// AddMember adiciona dinamicamente um atendente à fila do Asterisk.
//
// @preExecution Validação de JSON e parâmetros obrigatórios (campaign_id, agent_id).
// @postExecution Disparo de AMI QueueAdd e registro em Redis.
func (h *QueueHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	var req domain.QueueMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo JSON da requisição é inválido").WriteJSON(w)
		return
	}

	if req.CampaignID == "" || req.AgentID == "" {
		domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "campaign_id e agent_id são obrigatórios").WriteJSON(w)
		return
	}

	if err := h.queueMgr.AddMember(r.Context(), &req); err != nil {
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
		"data": map[string]interface{}{
			"message":     "Operador adicionado à fila com sucesso",
			"campaign_id": req.CampaignID,
			"agent_id":    req.AgentID,
			"paused":      req.Paused,
		},
	})
}

// RemoveMember remove o atendente da fila do Asterisk.
func (h *QueueHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	var req domain.QueueMemberRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo JSON da requisição é inválido").WriteJSON(w)
		return
	}

	if req.CampaignID == "" || req.AgentID == "" {
		domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "campaign_id e agent_id são obrigatórios").WriteJSON(w)
		return
	}

	if err := h.queueMgr.RemoveMember(r.Context(), &req); err != nil {
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
		"data": map[string]interface{}{
			"message":     "Operador removido da fila com sucesso",
			"campaign_id": req.CampaignID,
			"agent_id":    req.AgentID,
		},
	})
}

// PauseMember altera o status de pausa do atendente na fila.
func (h *QueueHandler) PauseMember(w http.ResponseWriter, r *http.Request) {
	var req domain.QueueMemberPauseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo JSON da requisição é inválido").WriteJSON(w)
		return
	}

	if req.CampaignID == "" || req.AgentID == "" {
		domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "campaign_id e agent_id são obrigatórios").WriteJSON(w)
		return
	}

	if err := h.queueMgr.PauseMember(r.Context(), &req); err != nil {
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
		"data": map[string]interface{}{
			"message":     "Status de pausa do operador atualizado com sucesso",
			"campaign_id": req.CampaignID,
			"agent_id":    req.AgentID,
			"paused":      req.Paused,
			"reason":      req.Reason,
		},
	})
}

// GetQueueMembers exibe a fila especificada.
func (h *QueueHandler) GetQueueMembers(w http.ResponseWriter, r *http.Request) {
	queueID := chi.URLParam(r, "queue_id")
	if queueID == "" {
		domain.NewErrBadRequest("MISSING_QUEUE_ID", "queue_id é obrigatório").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"queue_id": queueID,
			"status":   "active",
		},
	})
}

// SetPresence atualiza o estado de presença do atendente (ready, pause, tabulando, leave, offline).
//
// @pattern Strategy / Facade (Agent Presence Controller)
// @governedBy docs/rules/PREDICTIVE_QUEUES_LIVEKIT.md
// @preExecution Validação de JSON e parâmetros essenciais (action, agent_id).
// @postExecution Atualização atômica na fila Asterisk e no Redis.
func (h *QueueHandler) SetPresence(w http.ResponseWriter, r *http.Request) {
	var req domain.QueuePresenceEvent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo JSON da requisição é inválido").WriteJSON(w)
		return
	}

	if req.Action == "" || req.AgentID == "" {
		domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "action e agent_id são obrigatórios").WriteJSON(w)
		return
	}

	if err := h.queueMgr.SetPresence(r.Context(), &req); err != nil {
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
		"data": map[string]interface{}{
			"message":     "Estado de presença atualizado com sucesso",
			"action":      req.Action,
			"campaign_id": req.CampaignID,
			"agent_id":    req.AgentID,
			"reason":      req.Reason,
		},
	})
}

