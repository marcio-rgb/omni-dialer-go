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

	createdAt := c.CreatedAt
	if createdAt.IsZero() {
		if c.InitiatedAt != nil && !c.InitiatedAt.IsZero() {
			createdAt = *c.InitiatedAt
		} else if c.AnsweredAt != nil && !c.AnsweredAt.IsZero() {
			createdAt = *c.AnsweredAt
		} else {
			createdAt = time.Now()
		}
	}
	initiatedAt := c.InitiatedAt
	if initiatedAt == nil || initiatedAt.IsZero() {
		initiatedAt = &createdAt
	}

	query := `
		INSERT INTO cdrs (
			id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
			sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
			trunk_used, recording_file, recording_url, transcription,
			lead_id, lead_name, lead_cpf, amd_status, amd_cause,
			created_at, initiated_at, answered_at, ended_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
		)
		ON CONFLICT (id) DO UPDATE SET
			created_at = CASE 
				WHEN cdrs.created_at < '2000-01-01'::timestamptz THEN 
					COALESCE(NULLIF(EXCLUDED.created_at, '0001-01-01'::timestamptz), cdrs.answered_at, cdrs.ended_at, NOW())
				ELSE cdrs.created_at 
			END,
			initiated_at = COALESCE(cdrs.initiated_at, EXCLUDED.initiated_at, cdrs.answered_at),
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
			lead_id = COALESCE(EXCLUDED.lead_id, cdrs.lead_id),
			lead_name = COALESCE(EXCLUDED.lead_name, cdrs.lead_name),
			lead_cpf = COALESCE(EXCLUDED.lead_cpf, cdrs.lead_cpf),
			amd_status = COALESCE(EXCLUDED.amd_status, cdrs.amd_status),
			amd_cause = COALESCE(EXCLUDED.amd_cause, cdrs.amd_cause),
			answered_at = COALESCE(EXCLUDED.answered_at, cdrs.answered_at),
			ended_at = EXCLUDED.ended_at
	`
	_, err := r.pool.Exec(ctx, query,
		c.ID, c.TenantID, c.CampaignID, c.Phone, c.AgentID, string(c.CallType), string(c.Disposition),
		c.SIPStatus, c.HangupCause, c.DurationSeconds, c.BillsecSeconds, c.RingSeconds,
		c.TrunkUsed, c.RecordingFile, c.RecordingURL, c.Transcription,
		c.LeadID, c.LeadName, c.LeadCPF, c.AMDStatus, c.AMDCause,
		createdAt, initiatedAt, c.AnsweredAt, c.EndedAt,
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

// SaveCDREvent persiste atomicamente qualquer transição de estado de chamada (Redis Stream Persister).
func (r *ReportRepo) SaveCDREvent(ctx context.Context, e *domain.CDREvent) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	startedAt := e.StartedAt
	if startedAt.IsZero() {
		if e.InitiatedAt != nil && !e.InitiatedAt.IsZero() {
			startedAt = *e.InitiatedAt
		} else if e.Timestamp > 0 {
			startedAt = time.Unix(e.Timestamp, 0)
		} else {
			startedAt = time.Now()
		}
	}
	initiatedAt := e.InitiatedAt
	if initiatedAt == nil || initiatedAt.IsZero() {
		initiatedAt = &startedAt
	}

	query := `
		INSERT INTO cdrs (
			id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
			sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
			trunk_used, recording_file, recording_url, transcription,
			lead_id, lead_name, lead_cpf, amd_status, amd_cause,
			created_at, initiated_at, answered_at, ended_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
		)
		ON CONFLICT (id) DO UPDATE SET
			created_at = CASE 
				WHEN cdrs.created_at < '2000-01-01'::timestamptz THEN 
					COALESCE(NULLIF(EXCLUDED.created_at, '0001-01-01'::timestamptz), cdrs.answered_at, cdrs.ended_at, NOW())
				ELSE cdrs.created_at 
			END,
			initiated_at = COALESCE(cdrs.initiated_at, EXCLUDED.initiated_at, cdrs.answered_at),
			disposition = CASE 
				WHEN EXCLUDED.disposition = 'DIALING' AND cdrs.disposition != 'DIALING' THEN cdrs.disposition 
				ELSE EXCLUDED.disposition 
			END,
			agent_id = COALESCE(EXCLUDED.agent_id, cdrs.agent_id),
			sip_status = COALESCE(EXCLUDED.sip_status, cdrs.sip_status),
			hangup_cause = COALESCE(EXCLUDED.hangup_cause, cdrs.hangup_cause),
			duration_seconds = GREATEST(EXCLUDED.duration_seconds, cdrs.duration_seconds),
			billsec_seconds = GREATEST(EXCLUDED.billsec_seconds, cdrs.billsec_seconds),
			ring_seconds = GREATEST(EXCLUDED.ring_seconds, cdrs.ring_seconds),
			recording_file = COALESCE(EXCLUDED.recording_file, cdrs.recording_file),
			recording_url = COALESCE(EXCLUDED.recording_url, cdrs.recording_url),
			transcription = COALESCE(EXCLUDED.transcription, cdrs.transcription),
			lead_id = COALESCE(EXCLUDED.lead_id, cdrs.lead_id),
			lead_name = COALESCE(EXCLUDED.lead_name, cdrs.lead_name),
			lead_cpf = COALESCE(EXCLUDED.lead_cpf, cdrs.lead_cpf),
			amd_status = COALESCE(EXCLUDED.amd_status, cdrs.amd_status),
			amd_cause = COALESCE(EXCLUDED.amd_cause, cdrs.amd_cause),
			answered_at = COALESCE(EXCLUDED.answered_at, cdrs.answered_at),
			ended_at = COALESCE(EXCLUDED.ended_at, cdrs.ended_at)
	`
	_, err := r.pool.Exec(ctx, query,
		e.CallID, e.TenantID, e.CampaignID, e.Phone, e.AgentID, string(e.CallType), string(e.Disposition),
		e.SIPStatus, e.HangupCause, e.DurationSeconds, e.BillsecSeconds, e.RingSeconds,
		e.TrunkUsed, e.RecordingFile, e.RecordingURL, e.Transcription,
		e.LeadID, e.LeadName, e.LeadCPF, e.AMDStatus, e.AMDCause,
		startedAt, initiatedAt, e.AnsweredAt, e.EndedAt,
	)
	if err == nil && e.CallType == domain.CallTypePredictive && e.CampaignID != nil && e.Phone != "" {
		if e.Disposition == domain.DispositionDelivered || e.Disposition == domain.DispositionAnswered {
			_, _ = r.pool.Exec(ctx, `UPDATE leads SET status = 'COMPLETED', last_dialed_at = $1 WHERE campaign_id = $2 AND phone = $3`, time.Now(), *e.CampaignID, e.Phone)
		} else if e.Disposition != domain.CallDisposition("DIALING") {
			_, _ = r.pool.Exec(ctx, `UPDATE leads SET status = 'QUEUED', last_dialed_at = $1 WHERE campaign_id = $2 AND phone = $3 AND status != 'COMPLETED'`, time.Now(), *e.CampaignID, e.Phone)
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

// ListCDRs lista registros de CDR com paginação, filtros avançados de Contact Center e busca inteligente.
//
// @pattern Repository (Query Specification / Discriminator Pattern)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
//
// @preExecution
// - Validação de pool não-nulo
// - Sanitização de page/limit
// - Discriminação de busca: numérica (B-Tree phone/lead_cpf) vs textual (GIN FTS transcription)
//
// @postExecution
// - COUNT(*) condicional (somente se IncludeTotal == true)
// - Ordenação dinâmica segura via whitelist estrita (sem interpolação de input)
// - HasMore calculado por len(cdrs) == limit
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

	if filter.AgentID != nil && *filter.AgentID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("agent_id = $%d", argIdx))
		args = append(args, *filter.AgentID)
		argIdx++
	}

	if filter.CallType != nil && *filter.CallType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("call_type = $%d", argIdx))
		args = append(args, string(*filter.CallType))
		argIdx++
	}

	if filter.TrunkUsed != nil && *filter.TrunkUsed != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("trunk_used = $%d", argIdx))
		args = append(args, *filter.TrunkUsed)
		argIdx++
	}

	if filter.LeadCPF != nil && *filter.LeadCPF != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("lead_cpf = $%d", argIdx))
		args = append(args, *filter.LeadCPF)
		argIdx++
	}

	if filter.LeadID != nil && *filter.LeadID > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("lead_id = $%d", argIdx))
		args = append(args, *filter.LeadID)
		argIdx++
	}

	if filter.Phone != nil && *filter.Phone != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("phone = $%d", argIdx))
		args = append(args, *filter.Phone)
		argIdx++
	}

	// Disposições: prioriza slice múltiplo; fallback para singular
	if len(filter.Dispositions) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("disposition = ANY($%d)", argIdx))
		dispStrings := make([]string, len(filter.Dispositions))
		for i, d := range filter.Dispositions {
			dispStrings[i] = string(d)
		}
		args = append(args, dispStrings)
		argIdx++
	} else if filter.Disposition != nil && *filter.Disposition != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("disposition = $%d", argIdx))
		args = append(args, string(*filter.Disposition))
		argIdx++
	}

	if filter.AMDStatus != nil && *filter.AMDStatus != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("amd_status = $%d", argIdx))
		args = append(args, *filter.AMDStatus)
		argIdx++
	}

	if filter.HasRecording != nil {
		if *filter.HasRecording {
			whereClauses = append(whereClauses, "recording_url IS NOT NULL AND recording_url != ''")
		} else {
			whereClauses = append(whereClauses, "(recording_url IS NULL OR recording_url = '')")
		}
	}

	if filter.MinBillsec != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("billsec_seconds >= $%d", argIdx))
		args = append(args, *filter.MinBillsec)
		argIdx++
	}
	if filter.MaxBillsec != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("billsec_seconds <= $%d", argIdx))
		args = append(args, *filter.MaxBillsec)
		argIdx++
	}
	if filter.MinDuration != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("duration_seconds >= $%d", argIdx))
		args = append(args, *filter.MinDuration)
		argIdx++
	}
	if filter.MaxDuration != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("duration_seconds <= $%d", argIdx))
		args = append(args, *filter.MaxDuration)
		argIdx++
	}

	// Busca Inteligente (Discriminator Pattern): numérica via B-Tree/varchar_pattern_ops, textual via GIN FTS
	if filter.Search != nil && *filter.Search != "" {
		if searchRes, nextArgIdx := buildSmartSearchClause(*filter.Search, argIdx); searchRes != nil {
			whereClauses = append(whereClauses, searchRes.SQLClause)
			args = append(args, searchRes.Args...)
			argIdx = nextArgIdx
		}
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

	// COUNT(*) condicional: pula se IncludeTotal == false
	var totalPtr *int64
	var totalPagesPtr *int
	if filter.IncludeTotal {
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM cdrs WHERE %s", whereSQL)
		var total int64
		if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, fmt.Errorf("falha ao contar cdrs: %w", err)
		}
		totalPtr = &total
		tp := int(math.Ceil(float64(total) / float64(limit)))
		if tp == 0 {
			tp = 1
		}
		totalPagesPtr = &tp
	}

	// Ordenação dinâmica segura via whitelist estrita
	orderColumn := resolveSortColumn(filter.SortBy)
	orderDir := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		orderDir = "ASC"
	}

	// Overfetch pattern: busca limit + 1 para determinar HasMore com precisão matemática absoluta
	fetchLimit := limit + 1
	selectSQL := fmt.Sprintf(`
		SELECT cdrs.id, cdrs.tenant_id, cdrs.campaign_id, cdrs.phone, cdrs.agent_id,
		       COALESCE(ta.agent_name, 'Não Informado') AS agent_name,
		       cdrs.call_type, cdrs.disposition,
		       cdrs.sip_status, cdrs.hangup_cause, cdrs.duration_seconds, cdrs.billsec_seconds, cdrs.ring_seconds,
		       cdrs.trunk_used, cdrs.recording_file, cdrs.recording_url, cdrs.transcription,
		       cdrs.lead_id, cdrs.lead_name, cdrs.lead_cpf, cdrs.amd_status, cdrs.amd_cause,
		       cdrs.created_at, cdrs.initiated_at, cdrs.answered_at, cdrs.ended_at
		FROM (
			SELECT id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
			       sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
			       trunk_used, recording_file, recording_url, transcription,
			       lead_id, lead_name, lead_cpf, amd_status, amd_cause,
			       created_at, initiated_at, answered_at, ended_at
			FROM cdrs
			WHERE %s
			ORDER BY %s %s NULLS LAST
			LIMIT $%d OFFSET $%d
		) cdrs
		LEFT JOIN tenants_agents ta ON ta.agent_id = cdrs.agent_id AND ta.tenant_id = cdrs.tenant_id
		ORDER BY cdrs.%s %s NULLS LAST
	`, whereSQL, orderColumn, orderDir, argIdx, argIdx+1, orderColumn, orderDir)

	selectArgs := append(args, fetchLimit, offset)
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
			&c.ID, &c.TenantID, &c.CampaignID, &c.Phone, &c.AgentID, &c.AgentName, &callTypeStr, &dispositionStr,
			&c.SIPStatus, &c.HangupCause, &c.DurationSeconds, &c.BillsecSeconds, &c.RingSeconds,
			&c.TrunkUsed, &c.RecordingFile, &c.RecordingURL, &c.Transcription,
			&c.LeadID, &c.LeadName, &c.LeadCPF, &c.AMDStatus, &c.AMDCause,
			&c.CreatedAt, &c.InitiatedAt, &c.AnsweredAt, &c.EndedAt,
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

	hasMore := len(cdrs) > limit
	if hasMore {
		cdrs = cdrs[:limit]
	} else if totalPtr != nil {
		hasMore = (int64(offset) + int64(len(cdrs))) < *totalPtr
	}

	return &domain.CDRListResponse{
		Total:      totalPtr,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPagesPtr,
		HasMore:    hasMore,
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
		SELECT cdrs.id, cdrs.tenant_id, cdrs.campaign_id, cdrs.phone, cdrs.agent_id,
		       COALESCE(ta.agent_name, 'Não Informado') AS agent_name,
		       cdrs.call_type, cdrs.disposition,
		       cdrs.sip_status, cdrs.hangup_cause, cdrs.duration_seconds, cdrs.billsec_seconds, cdrs.ring_seconds,
		       cdrs.trunk_used, cdrs.recording_file, cdrs.recording_url, cdrs.transcription,
		       cdrs.lead_id, cdrs.lead_name, cdrs.lead_cpf, cdrs.amd_status, cdrs.amd_cause,
		       cdrs.created_at, cdrs.initiated_at, cdrs.answered_at, cdrs.ended_at
		FROM cdrs
		LEFT JOIN tenants_agents ta ON ta.agent_id = cdrs.agent_id AND ta.tenant_id = cdrs.tenant_id
		WHERE cdrs.tenant_id = $1 AND cdrs.id = $2
		LIMIT 1
	`
	var c domain.CDR
	var callTypeStr, dispositionStr string
	err := r.pool.QueryRow(ctx, query, tenantID, cdrID).Scan(
		&c.ID, &c.TenantID, &c.CampaignID, &c.Phone, &c.AgentID, &c.AgentName, &callTypeStr, &dispositionStr,
		&c.SIPStatus, &c.HangupCause, &c.DurationSeconds, &c.BillsecSeconds, &c.RingSeconds,
		&c.TrunkUsed, &c.RecordingFile, &c.RecordingURL, &c.Transcription,
		&c.LeadID, &c.LeadName, &c.LeadCPF, &c.AMDStatus, &c.AMDCause,
		&c.CreatedAt, &c.InitiatedAt, &c.AnsweredAt, &c.EndedAt,
	)
	if err != nil {
		return nil, err
	}
	c.CallType = domain.CallType(callTypeStr)
	c.Disposition = domain.CallDisposition(dispositionStr)
	return &c, nil
}
