package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

// AgentHandler gerencia requisições HTTP REST para cadastro de agentes e relatórios de produtividade.
//
// @pattern Strategy / HTTP Controller
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type AgentHandler struct {
	agentRepo ports.AgentRepository
}

func NewAgentHandler(agentRepo ports.AgentRepository) *AgentHandler {
	return &AgentHandler{agentRepo: agentRepo}
}

// UpsertAgent cadastra ou atualiza os dados do operador (tenants_agents).
//
// @preExecution Validação de JSON e campos obrigatórios (agent_id, agent_name).
// @postExecution Inserção/atualização atômica em public.tenants_agents.
func (h *AgentHandler) UpsertAgent(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	var req domain.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	req.AgentID = strings.TrimSpace(req.AgentID)
	req.AgentName = strings.TrimSpace(req.AgentName)
	if req.AgentID == "" || req.AgentName == "" {
		domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "agent_id e agent_name são obrigatórios").WriteJSON(w)
		return
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	agent := &domain.TenantAgent{
		TenantID:  tenantID,
		AgentID:   req.AgentID,
		AgentName: req.AgentName,
		IsActive:  isActive,
	}

	if err := h.agentRepo.UpsertAgent(r.Context(), agent); err != nil {
		domain.NewErrInternal("Falha ao salvar agente: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    agent,
	})
}

// BatchUpsert cadastra ou atualiza múltiplos operadores em lote.
func (h *AgentHandler) BatchUpsert(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	var req domain.BatchUpsertAgentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	var saved []*domain.TenantAgent
	for _, item := range req.Agents {
		aID := strings.TrimSpace(item.AgentID)
		aName := strings.TrimSpace(item.AgentName)
		if aID == "" || aName == "" {
			continue
		}
		active := true
		if item.IsActive != nil {
			active = *item.IsActive
		}
		ag := &domain.TenantAgent{
			TenantID:  tenantID,
			AgentID:   aID,
			AgentName: aName,
			IsActive:  active,
		}
		if err := h.agentRepo.UpsertAgent(r.Context(), ag); err == nil {
			saved = append(saved, ag)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data": map[string]any{
			"total_processed": len(saved),
			"agents":          saved,
		},
	})
}

// GetAgent busca um operador pelo identificador.
func (h *AgentHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	agentID := chi.URLParam(r, "id")
	if agentID == "" {
		domain.NewErrBadRequest("MISSING_AGENT_ID", "Identificador do agente não informado").WriteJSON(w)
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	agent, err := h.agentRepo.GetAgent(r.Context(), tenantID, agentID)
	if err != nil {
		domain.NewErrInternal("Falha ao buscar agente: " + err.Error()).WriteJSON(w)
		return
	}
	if agent == nil {
		domain.NewErrNotFound("AGENT_NOT_FOUND", "Agente não encontrado").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    agent,
	})
}

// ListAgents lista os agentes com suporte a paginação e filtro de ativo.
func (h *AgentHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := (page - 1) * limit

	var isActive *bool
	if activeStr := r.URL.Query().Get("is_active"); activeStr != "" {
		b := strings.ToLower(activeStr) == "true" || activeStr == "1"
		isActive = &b
	}

	agents, total, err := h.agentRepo.ListAgents(r.Context(), tenantID, isActive, limit, offset)
	if err != nil {
		domain.NewErrInternal("Falha ao listar agentes: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data": map[string]any{
			"total":  total,
			"page":   page,
			"limit":  limit,
			"agents": agents,
		},
	})
}

// DeleteAgent remove ou desativa um operador.
func (h *AgentHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	agentID := chi.URLParam(r, "id")
	if agentID == "" {
		domain.NewErrBadRequest("MISSING_AGENT_ID", "Identificador do agente não informado").WriteJSON(w)
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	if err := h.agentRepo.DeleteAgent(r.Context(), tenantID, agentID); err != nil {
		domain.NewErrInternal("Falha ao excluir agente: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Agente removido com sucesso",
	})
}

// GetAgentHistory consulta a timeline de transições de presença do operador (tenant_agent_history).
func (h *AgentHandler) GetAgentHistory(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	agentID := chi.URLParam(r, "id")
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	startDate := time.Now().Truncate(24 * time.Hour)
	if s := r.URL.Query().Get("start_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startDate = t
		}
	}
	endDate := time.Now()
	if s := r.URL.Query().Get("end_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			endDate = t
		}
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	history, err := h.agentRepo.GetAgentHistory(r.Context(), tenantID, agentID, startDate, endDate, limit, 0)
	if err != nil {
		domain.NewErrInternal("Falha ao consultar histórico: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data": map[string]any{
			"agent_id":   agentID,
			"start_date": startDate.Format(time.RFC3339),
			"end_date":   endDate.Format(time.RFC3339),
			"history":    history,
		},
	})
}

// GetAgentPerformance retorna o relatório consolidado de produtividade do operador (ligações, >40s, tempo disponível).
//
// @pattern Aggregator / Reporting Controller
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (h *AgentHandler) GetAgentPerformance(w http.ResponseWriter, r *http.Request) {
	if h.agentRepo == nil {
		domain.NewErrInternal("Repositório de agentes não inicializado").WriteJSON(w)
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	startDate := time.Now().Truncate(24 * time.Hour)
	if s := r.URL.Query().Get("start_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startDate = t
		}
	}
	endDate := time.Now()
	if s := r.URL.Query().Get("end_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			endDate = t
		}
	}

	var agentIDPtr *string
	if a := strings.TrimSpace(r.URL.Query().Get("agent_id")); a != "" {
		agentIDPtr = &a
	}

	items, err := h.agentRepo.GetAgentPerformance(r.Context(), tenantID, startDate, endDate, agentIDPtr)
	if err != nil {
		domain.NewErrInternal("Falha ao gerar relatório de produtividade: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data": domain.AgentPerformanceResponse{
			TenantID:  tenantID,
			StartDate: startDate.Format(time.RFC3339),
			EndDate:   endDate.Format(time.RFC3339),
			Agents:    items,
		},
	})
}
