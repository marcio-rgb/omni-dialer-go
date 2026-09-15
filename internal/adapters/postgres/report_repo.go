package postgres

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReportRepo struct {
	pool *pgxpool.Pool
}

var _ ports.ReportRepository = (*ReportRepo)(nil)

func NewReportRepo(pool *pgxpool.Pool) *ReportRepo {
	return &ReportRepo{pool: pool}
}

func (r *ReportRepo) SaveCDR(ctx context.Context, c *domain.CDR) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		INSERT INTO cdrs (
			id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
			sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
			trunk_used, recording_file, recording_url, transcription, created_at, initiated_at, answered_at, ended_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT (id) DO UPDATE SET
			disposition = EXCLUDED.disposition,
			agent_id = COALESCE(EXCLUDED.agent_id, cdrs.agent_id),
			sip_status = COALESCE(EXCLUDED.sip_status, cdrs.sip_status),
			hangup_cause = COALESCE(EXCLUDED.hangup_cause, cdrs.hangup_cause),
			duration_seconds = EXCLUDED.duration_seconds,
			billsec_seconds = EXCLUDED.billsec_seconds,
			ring_seconds = EXCLUDED.ring_seconds,
			recording_file = COALESCE(EXCLUDED.recording_file, cdrs.recording_file),
			recording_url = COALESCE(EXCLUDED.recording_url, cdrs.recording_url),
			transcription = COALESCE(EXCLUDED.transcription, cdrs.transcription),
			answered_at = COALESCE(EXCLUDED.answered_at, cdrs.answered_at),
			ended_at = EXCLUDED.ended_at
	`
	_, err := r.pool.Exec(ctx, query,
		c.ID, c.TenantID, c.CampaignID, c.Phone, c.AgentID, string(c.CallType), string(c.Disposition),
		c.SIPStatus, c.HangupCause, c.DurationSeconds, c.BillsecSeconds, c.RingSeconds,
		c.TrunkUsed, c.RecordingFile, c.RecordingURL, c.Transcription, c.CreatedAt, c.InitiatedAt, c.AnsweredAt, c.EndedAt,
	)
	if c.CallType == domain.CallTypePredictive && c.CampaignID != nil && c.Phone != "" {
		if c.Disposition == domain.DispositionDelivered || c.Disposition == domain.DispositionAnswered {
			_, _ = r.pool.Exec(ctx, `UPDATE leads SET status = 'COMPLETED', last_dialed_at = $1 WHERE campaign_id = $2 AND phone = $3`, time.Now(), *c.CampaignID, c.Phone)
		} else {
			_, _ = r.pool.Exec(ctx, `UPDATE leads SET status = 'QUEUED', last_dialed_at = $1 WHERE campaign_id = $2 AND phone = $3 AND status != 'COMPLETED'`, time.Now(), *c.CampaignID, c.Phone)
		}
	}
	return err
}

// UpdateCDRTranscription atualiza atomicamente o texto transcrito em tempo real de uma chamada.
//
// @pattern Repository & Unit of Work
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
func (r *ReportRepo) UpdateCDRTranscription(ctx context.Context, cdrID string, transcription string) error {
	if cdrID == "" || transcription == "" {
		return nil
	}
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	query := `UPDATE cdrs SET transcription = $1 WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, transcription, cdrID)
	return err
}

// ListCDRs lista registros de CDR com paginação e filtros dinâmicos indexados.
//
// @pattern Repository (Query Specification)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
func (r *ReportRepo) ListCDRs(ctx context.Context, filter domain.CDRFilter) (*domain.CDRListResponse, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	whereClauses := []string{"tenant_id = $1"}
	args := []any{filter.TenantID}
	argIdx := 2

	if filter.CampaignID != nil && *filter.CampaignID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("campaign_id = $%d", argIdx))
		args = append(args, *filter.CampaignID)
		argIdx++
	}

	if filter.Phone != nil && *filter.Phone != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("phone = $%d", argIdx))
		args = append(args, *filter.Phone)
		argIdx++
	}

	if filter.Disposition != nil && *filter.Disposition != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("disposition = $%d", argIdx))
		args = append(args, string(*filter.Disposition))
		argIdx++
	}

	if filter.Search != nil && *filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(phone ILIKE $%d OR transcription ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+*filter.Search+"%")
		argIdx++
	}

	if filter.StartDate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.StartDate)
		argIdx++
	}

	if filter.EndDate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filter.EndDate)
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM cdrs WHERE %s", whereSQL)
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("falha ao contar cdrs: %w", err)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages == 0 {
		totalPages = 1
	}

	selectSQL := fmt.Sprintf(`
		SELECT id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
		       sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
		       trunk_used, recording_file, recording_url, transcription, created_at, initiated_at, answered_at, ended_at
		FROM cdrs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	selectArgs := append(args, limit, offset)
	rows, err := r.pool.Query(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, fmt.Errorf("falha ao listar cdrs: %w", err)
	}
	defer rows.Close()

	var cdrs []*domain.CDR
	for rows.Next() {
		var c domain.CDR
		var callTypeStr, dispositionStr string
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.CampaignID, &c.Phone, &c.AgentID, &callTypeStr, &dispositionStr,
			&c.SIPStatus, &c.HangupCause, &c.DurationSeconds, &c.BillsecSeconds, &c.RingSeconds,
			&c.TrunkUsed, &c.RecordingFile, &c.RecordingURL, &c.Transcription, &c.CreatedAt, &c.InitiatedAt, &c.AnsweredAt, &c.EndedAt,
		); err != nil {
			return nil, fmt.Errorf("falha ao ler linha de cdr: %w", err)
		}
		c.CallType = domain.CallType(callTypeStr)
		c.Disposition = domain.CallDisposition(dispositionStr)
		cdrs = append(cdrs, &c)
	}

	if cdrs == nil {
		cdrs = make([]*domain.CDR, 0)
	}

	return &domain.CDRListResponse{
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
		CDRs:       cdrs,
	}, nil
}

// GetCDRByID busca um registro de CDR por ID garantindo isolamento de tenant.
//
// @pattern Repository (Identity Query)
func (r *ReportRepo) GetCDRByID(ctx context.Context, tenantID, cdrID string) (*domain.CDR, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
		       sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
		       trunk_used, recording_file, recording_url, transcription, created_at, initiated_at, answered_at, ended_at
		FROM cdrs
		WHERE tenant_id = $1 AND id = $2
		LIMIT 1
	`
	var c domain.CDR
	var callTypeStr, dispositionStr string
	err := r.pool.QueryRow(ctx, query, tenantID, cdrID).Scan(
		&c.ID, &c.TenantID, &c.CampaignID, &c.Phone, &c.AgentID, &callTypeStr, &dispositionStr,
		&c.SIPStatus, &c.HangupCause, &c.DurationSeconds, &c.BillsecSeconds, &c.RingSeconds,
		&c.TrunkUsed, &c.RecordingFile, &c.RecordingURL, &c.Transcription, &c.CreatedAt, &c.InitiatedAt, &c.AnsweredAt, &c.EndedAt,
	)
	if err != nil {
		return nil, err
	}
	c.CallType = domain.CallType(callTypeStr)
	c.Disposition = domain.CallDisposition(dispositionStr)
	return &c, nil
}
