package postgres

import (
	"context"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentRepo implementa ports.AgentRepository para PostgreSQL.
//
// @pattern Repository (Unit of Work)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type AgentRepo struct {
	pool *pgxpool.Pool
}

var _ ports.AgentRepository = (*AgentRepo)(nil)

func NewAgentRepo(pool *pgxpool.Pool) *AgentRepo {
	return &AgentRepo{pool: pool}
}

// UpsertAgent cadastra ou atualiza os dados cadastrais do agente.
//
// @preExecution Validação de pool e dados obrigatórios (tenant_id, agent_id, agent_name).
// @postExecution Inserção atômica com ON CONFLICT (tenant_id, agent_id).
func (r *AgentRepo) UpsertAgent(ctx context.Context, agent *domain.TenantAgent) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	if agent.TenantID == "" {
		agent.TenantID = "default"
	}
	if agent.AgentID == "" || agent.AgentName == "" {
		return fmt.Errorf("agent_id e agent_name são obrigatórios")
	}

	query := `
		INSERT INTO tenants_agents (tenant_id, agent_id, agent_name, is_active, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
			agent_name = EXCLUDED.agent_name,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()
		RETURNING id, created_at, updated_at;
	`
	return r.pool.QueryRow(ctx, query, agent.TenantID, agent.AgentID, agent.AgentName, agent.IsActive).
		Scan(&agent.ID, &agent.CreatedAt, &agent.UpdatedAt)
}

// GetAgent busca um agente específico por tenant_id e agent_id.
func (r *AgentRepo) GetAgent(ctx context.Context, tenantID, agentID string) (*domain.TenantAgent, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	query := `
		SELECT id, tenant_id, agent_id, agent_name, is_active, created_at, updated_at
		FROM tenants_agents
		WHERE tenant_id = $1 AND agent_id = $2
	`
	var a domain.TenantAgent
	err := r.pool.QueryRow(ctx, query, tenantID, agentID).Scan(
		&a.ID, &a.TenantID, &a.AgentID, &a.AgentName, &a.IsActive, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// ListAgents lista agentes cadastrados com filtros e paginação.
func (r *AgentRepo) ListAgents(ctx context.Context, tenantID string, isActive *bool, limit, offset int) ([]*domain.TenantAgent, int64, error) {
	if r.pool == nil {
		return nil, 0, fmt.Errorf("postgres: pool não inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := `
		SELECT COUNT(*) FROM tenants_agents 
		WHERE tenant_id = $1 AND ($2::boolean IS NULL OR is_active = $2)
	`
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, tenantID, isActive).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, tenant_id, agent_id, agent_name, is_active, created_at, updated_at
		FROM tenants_agents
		WHERE tenant_id = $1 AND ($2::boolean IS NULL OR is_active = $2)
		ORDER BY agent_name ASC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.pool.Query(ctx, query, tenantID, isActive, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var agents []*domain.TenantAgent
	for rows.Next() {
		var a domain.TenantAgent
		if err := rows.Scan(&a.ID, &a.TenantID, &a.AgentID, &a.AgentName, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		agents = append(agents, &a)
	}
	return agents, total, nil
}

// DeleteAgent desativa logicamente ou remove o agente.
func (r *AgentRepo) DeleteAgent(ctx context.Context, tenantID, agentID string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	query := `DELETE FROM tenants_agents WHERE tenant_id = $1 AND agent_id = $2`
	_, err := r.pool.Exec(ctx, query, tenantID, agentID)
	return err
}

// RecordHistory fecha o estado anterior aberto e registra a nova transição na timeline.
//
// @pattern Unit of Work (Timeline Sourcing)
// @postExecution Encerra registro pendente com duração e insere nova tupla.
func (r *AgentRepo) RecordHistory(ctx context.Context, h *domain.TenantAgentHistory) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	if h.TenantID == "" {
		h.TenantID = "default"
	}
	now := time.Now()
	if h.StartedAt.IsZero() {
		h.StartedAt = now
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Fecha qualquer estado aberto anterior calculando a duração em segundos
	closeQuery := `
		UPDATE tenant_agent_history
		SET ended_at = $1,
		    duration_seconds = GREATEST(0, EXTRACT(EPOCH FROM ($1 - started_at))::int)
		WHERE tenant_id = $2 AND agent_id = $3 AND ended_at IS NULL;
	`
	if _, err := tx.Exec(ctx, closeQuery, h.StartedAt, h.TenantID, h.AgentID); err != nil {
		return fmt.Errorf("falha ao fechar estado anterior do agente: %w", err)
	}

	// 2. Insere a nova transição de estado
	insertQuery := `
		INSERT INTO tenant_agent_history (
			tenant_id, agent_id, campaign_id, status, action, reason, livekit_room, call_id, started_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at;
	`
	err = tx.QueryRow(ctx, insertQuery,
		h.TenantID, h.AgentID, h.CampaignID, h.Status, h.Action, h.Reason, h.LiveKitRoom, h.CallID, h.StartedAt,
	).Scan(&h.ID, &h.CreatedAt)
	if err != nil {
		return fmt.Errorf("falha ao inserir histórico de agente: %w", err)
	}

	return tx.Commit(ctx)
}

// GetAgentHistory consulta a timeline de transições do agente no período especificado.
func (r *AgentRepo) GetAgentHistory(ctx context.Context, tenantID, agentID string, startDate, endDate time.Time, limit, offset int) ([]*domain.TenantAgentHistory, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, tenant_id, agent_id, campaign_id, status, action, reason, livekit_room, call_id, started_at, ended_at, duration_seconds, created_at
		FROM tenant_agent_history
		WHERE tenant_id = $1 AND agent_id = $2
		  AND started_at >= $3 AND started_at <= $4
		ORDER BY started_at DESC
		LIMIT $5 OFFSET $6
	`
	rows, err := r.pool.Query(ctx, query, tenantID, agentID, startDate, endDate, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.TenantAgentHistory
	for rows.Next() {
		var h domain.TenantAgentHistory
		if err := rows.Scan(
			&h.ID, &h.TenantID, &h.AgentID, &h.CampaignID, &h.Status, &h.Action, &h.Reason,
			&h.LiveKitRoom, &h.CallID, &h.StartedAt, &h.EndedAt, &h.DurationSeconds, &h.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, &h)
	}
	return list, nil
}

// GetAgentPerformance consolida chamadas e tempo disponível/pausa, sempre com LEFT JOIN em tenants_agents.
//
// @pattern Aggregator / Reporting
// @postExecution LEFT JOIN em public.tenants_agents com COALESCE(agent_name, 'Não Informado').
func (r *AgentRepo) GetAgentPerformance(ctx context.Context, tenantID string, startDate, endDate time.Time, agentID *string) ([]*domain.AgentPerformanceItem, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	query := `
		WITH call_stats AS (
			SELECT 
				c.agent_id,
				COUNT(*) AS total_calls,
				COUNT(*) FILTER (WHERE c.disposition IN ('ANSWERED', 'DELIVERED')) AS answered_calls,
				COUNT(*) FILTER (WHERE c.billsec_seconds > 40) AS calls_over_40s,
				COALESCE(SUM(c.billsec_seconds), 0) AS total_talk_time_seconds,
				COALESCE(ROUND(AVG(c.billsec_seconds) FILTER (WHERE c.disposition IN ('ANSWERED', 'DELIVERED'))), 0)::bigint AS avg_talk_time_seconds
			FROM cdrs c
			WHERE c.tenant_id = $1
			  AND c.created_at >= $2
			  AND c.created_at <= $3
			  AND c.agent_id IS NOT NULL
			  AND ($4::varchar IS NULL OR c.agent_id = $4)
			GROUP BY c.agent_id
		),
		hist_stats AS (
			SELECT
				h.agent_id,
				COALESCE(SUM(h.duration_seconds) FILTER (WHERE h.status = 'AVAILABLE'), 0)::bigint AS total_available_seconds,
				COALESCE(SUM(h.duration_seconds) FILTER (WHERE h.status = 'PAUSED'), 0)::bigint AS total_paused_seconds
			FROM tenant_agent_history h
			WHERE h.tenant_id = $1
			  AND h.started_at >= $2
			  AND h.started_at <= $3
			  AND ($4::varchar IS NULL OR h.agent_id = $4)
			GROUP BY h.agent_id
		),
		all_agents AS (
			SELECT agent_id FROM call_stats
			UNION
			SELECT agent_id FROM hist_stats
			UNION
			SELECT agent_id FROM tenants_agents WHERE tenant_id = $1 AND ($4::varchar IS NULL OR agent_id = $4)
		)
		SELECT 
			a.agent_id,
			COALESCE(ta.agent_name, 'Não Informado') AS agent_name,
			COALESCE(cs.total_calls, 0) AS total_calls,
			COALESCE(cs.answered_calls, 0) AS answered_calls,
			COALESCE(cs.calls_over_40s, 0) AS calls_over_40s,
			COALESCE(cs.total_talk_time_seconds, 0) AS total_talk_time_seconds,
			COALESCE(cs.avg_talk_time_seconds, 0) AS avg_talk_time_seconds,
			COALESCE(hs.total_available_seconds, 0) AS total_available_seconds,
			COALESCE(hs.total_paused_seconds, 0) AS total_paused_seconds
		FROM all_agents a
		LEFT JOIN tenants_agents ta ON ta.agent_id = a.agent_id AND ta.tenant_id = $1
		LEFT JOIN call_stats cs ON cs.agent_id = a.agent_id
		LEFT JOIN hist_stats hs ON hs.agent_id = a.agent_id
		ORDER BY cs.total_calls DESC NULLS LAST, a.agent_id ASC;
	`

	rows, err := r.pool.Query(ctx, query, tenantID, startDate, endDate, agentID)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar performance de agentes: %w", err)
	}
	defer rows.Close()

	var result []*domain.AgentPerformanceItem
	for rows.Next() {
		var item domain.AgentPerformanceItem
		if err := rows.Scan(
			&item.AgentID, &item.AgentName, &item.TotalCalls, &item.AnsweredCalls,
			&item.CallsOver40s, &item.TotalTalkTimeSeconds, &item.AverageTalkTimeSeconds,
			&item.TotalAvailableSeconds, &item.TotalPausedSeconds,
		); err != nil {
			return nil, fmt.Errorf("falha ao ler linha de performance: %w", err)
		}
		result = append(result, &item)
	}
	return result, nil
}
