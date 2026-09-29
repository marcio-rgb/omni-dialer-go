package ports

import (
	"context"
	"time"

	"dialer-go/internal/domain"
)

// AgentRepository define o contrato canônico para gestão de agentes e timeline de presença.
//
// @pattern Repository (Hexagonal Ports)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type AgentRepository interface {
	// Gestão de Cadastro de Agentes (public.tenants_agents)
	UpsertAgent(ctx context.Context, agent *domain.TenantAgent) error
	GetAgent(ctx context.Context, tenantID, agentID string) (*domain.TenantAgent, error)
	ListAgents(ctx context.Context, tenantID string, isActive *bool, limit, offset int) ([]*domain.TenantAgent, int64, error)
	DeleteAgent(ctx context.Context, tenantID, agentID string) error

	// Histórico e Transições de Estado (public.tenant_agent_history)
	RecordHistory(ctx context.Context, history *domain.TenantAgentHistory) error
	GetAgentHistory(ctx context.Context, tenantID, agentID string, startDate, endDate time.Time, limit, offset int) ([]*domain.TenantAgentHistory, error)

	// Relatórios e Produtividade Consolidada
	GetAgentPerformance(ctx context.Context, tenantID string, startDate, endDate time.Time, agentID *string) ([]*domain.AgentPerformanceItem, error)
}
