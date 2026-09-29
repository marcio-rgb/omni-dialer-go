package domain

import "time"

// TenantAgent representa a entidade soberana de cadastro de agentes por tenant.
//
// @pattern Entity (Data Transfer & Domain Model)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type TenantAgent struct {
	ID        int64     `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AgentID   string    `json:"agent_id"`
	AgentName string    `json:"agent_name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateAgentRequest representa o payload de cadastro/upsert de agente.
type CreateAgentRequest struct {
	TenantID  string `json:"tenant_id,omitempty"`
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	IsActive  *bool  `json:"is_active,omitempty"`
}

// UpdateAgentRequest representa o payload para atualização pontual de agente.
type UpdateAgentRequest struct {
	AgentName *string `json:"agent_name,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

// BatchUpsertAgentsRequest representa o payload para cadastro/sincronização em lote.
type BatchUpsertAgentsRequest struct {
	TenantID string               `json:"tenant_id,omitempty"`
	Agents   []CreateAgentRequest `json:"agents"`
}

// TenantAgentHistory representa o registro forense e histórico de cada transição de estado do operador.
//
// @pattern Entity (Event Sourcing / Timeline Audit)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type TenantAgentHistory struct {
	ID              int64      `json:"id"`
	TenantID        string     `json:"tenant_id"`
	AgentID         string     `json:"agent_id"`
	CampaignID      *string    `json:"campaign_id,omitempty"`
	Status          string     `json:"status"` // AVAILABLE, PAUSED, IN_CALL, OFFLINE
	Action          string     `json:"action"` // ready, pause, tabulando, call_assigned, call_ended, logout, heartbeat_expired
	Reason          *string    `json:"reason,omitempty"`
	LiveKitRoom     *string    `json:"livekit_room,omitempty"`
	CallID          *string    `json:"call_id,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	DurationSeconds int        `json:"duration_seconds"`
	CreatedAt       time.Time  `json:"created_at"`
}

// AgentPerformanceItem consolida a produtividade do operador (CDRs + Disponibilidade).
type AgentPerformanceItem struct {
	AgentID                string `json:"agent_id"`
	AgentName              string `json:"agent_name"`
	TotalCalls             int64  `json:"total_calls"`
	AnsweredCalls          int64  `json:"answered_calls"`
	CallsOver40s           int64  `json:"calls_over_40s"`
	TotalTalkTimeSeconds   int64  `json:"total_talk_time_seconds"`
	AverageTalkTimeSeconds int64  `json:"average_talk_time_seconds"`
	TotalAvailableSeconds  int64  `json:"total_available_seconds"`
	TotalPausedSeconds     int64  `json:"total_paused_seconds"`
}

// AgentPerformanceResponse representa a resposta estruturada do relatório de operadores.
type AgentPerformanceResponse struct {
	TenantID  string                  `json:"tenant_id"`
	StartDate string                  `json:"start_date"`
	EndDate   string                  `json:"end_date"`
	Agents    []*AgentPerformanceItem `json:"agents"`
}
