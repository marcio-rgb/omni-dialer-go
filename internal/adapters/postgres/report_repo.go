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
			trunk_used, recording_file, recording_url, created_at, initiated_at, answered_at, ended_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`
	_, err := r.pool.Exec(ctx, query,
		c.ID, c.TenantID, c.CampaignID, c.Phone, c.AgentID, string(c.CallType), string(c.Disposition),
		c.SIPStatus, c.HangupCause, c.DurationSeconds, c.BillsecSeconds, c.RingSeconds,
		c.TrunkUsed, c.RecordingFile, c.RecordingURL, c.CreatedAt, c.InitiatedAt, c.AnsweredAt, c.EndedAt,
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

// GetCallsSummary executa a query agregada em passada única no PostgreSQL
func (r *ReportRepo) GetCallsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, campaignID *string) (*domain.CallsSummaryResponse, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}
	query := `
		SELECT 
			COUNT(*) AS total_dialed,
			COUNT(*) FILTER (WHERE disposition = 'DELIVERED') AS delivered_calls,
			COUNT(*) FILTER (WHERE disposition IN ('ANSWERED', 'DELIVERED')) AS answered_calls,
			COUNT(*) FILTER (WHERE disposition IN ('VOICEMAIL', 'AMD_MACHINE')) AS answering_machine_calls,
			COUNT(*) FILTER (WHERE disposition = 'INVALID_NUMBER' OR sip_status IN (404, 484) OR hangup_cause IN (1, 28)) AS invalid_number_calls,
			COUNT(*) FILTER (WHERE disposition IN ('FAILED', 'CONGESTION') OR sip_status >= 500 OR hangup_cause = 34) AS failed_calls,
			COUNT(*) FILTER (WHERE disposition = 'BUSY' OR sip_status = 486 OR hangup_cause = 17) AS busy_calls,
			COUNT(*) FILTER (WHERE disposition = 'NO_ANSWER' OR hangup_cause = 19) AS unanswered_calls,
			COUNT(*) FILTER (WHERE disposition = 'ABANDONED') AS abandoned_calls,
			COUNT(*) FILTER (WHERE disposition = 'CANCELLED') AS cancelled_calls,
			
			-- Sub-detalhamento de números inválidos
			COUNT(*) FILTER (WHERE sip_status = 404 OR hangup_cause = 1) AS unallocated_number_calls,
			COUNT(*) FILTER (WHERE sip_status = 484 OR hangup_cause = 28) AS incomplete_number_calls,
			COUNT(*) FILTER (WHERE sip_status = 403 OR hangup_cause = 21) AS unassigned_service_calls,
			
			-- Sub-detalhamento de falhas
			COUNT(*) FILTER (WHERE sip_status = 503 OR hangup_cause = 34) AS congestion_calls,
			COUNT(*) FILTER (WHERE sip_status = 408 OR hangup_cause = 18) AS timeout_calls,
			COUNT(*) FILTER (WHERE sip_status IN (500, 502)) AS carrier_error_calls,
			COUNT(*) FILTER (WHERE disposition = 'TRUNK_CAPACITY') AS trunk_limit_exhausted_calls,
			
			-- Tempos
			COALESCE(SUM(billsec_seconds), 0) AS total_talk_time_seconds,
			COALESCE(ROUND(AVG(billsec_seconds) FILTER (WHERE disposition IN ('ANSWERED', 'DELIVERED'))), 0) AS average_talk_time_seconds,
			COALESCE(SUM(ring_seconds), 0) AS total_ring_time_seconds,
			COALESCE(ROUND(AVG(ring_seconds)), 0) AS average_ring_time_seconds
		FROM cdrs
		WHERE tenant_id = $1 
		  AND created_at >= $2 
		  AND created_at <= $3
		  AND ($4::varchar IS NULL OR campaign_id = $4)
	`

	var totalDialed, delivered, answered, voicemail, invalidNum, failed, busy, noAnswer, abandoned, cancelled int64
	var unalloc, incomplete, unassigned int64
	var congestion, timeout, carrierErr, trunkLimit int64
	var totalTalk, avgTalk, totalRing, avgRing int64

	err := r.pool.QueryRow(ctx, query, tenantID, startDate, endDate, campaignID).Scan(
		&totalDialed, &delivered, &answered, &voicemail, &invalidNum, &failed, &busy, &noAnswer, &abandoned, &cancelled,
		&unalloc, &incomplete, &unassigned,
		&congestion, &timeout, &carrierErr, &trunkLimit,
		&totalTalk, &avgTalk, &totalRing, &avgRing,
	)
	if err != nil {
		return nil, err
	}

	calcPercentage := func(part, total int64) float64 {
		if total == 0 {
			return 0.0
		}
		return math.Round((float64(part)/float64(total))*10000) / 100
	}

	abandonmentRate := 0.0
	if answered > 0 {
		abandonmentRate = math.Round((float64(abandoned)/float64(answered))*10000) / 100
	}

	return &domain.CallsSummaryResponse{
		TenantID: tenantID,
		Period: domain.PeriodDTO{
			StartDate: startDate.Format(time.RFC3339),
			EndDate:   endDate.Format(time.RFC3339),
		},
		Metrics: domain.CallMetricsDTO{
			TotalDialed:           totalDialed,
			DeliveredCalls:        delivered,
			AnsweredCalls:         answered,
			AnsweringMachineCalls: voicemail,
			InvalidNumberCalls:    invalidNum,
			FailedCalls:           failed,
			BusyCalls:             busy,
			UnansweredCalls:       noAnswer,
			AbandonedCalls:        abandoned,
			CancelledCalls:        cancelled,
			InvalidNumberBreakdown: domain.InvalidNumberBreakdown{
				UnallocatedNumberCalls: unalloc,
				IncompleteNumberCalls:  incomplete,
				UnassignedServiceCalls: unassigned,
			},
			FailureBreakdown: domain.FailureBreakdown{
				CongestionCalls:          congestion,
				TimeoutCalls:             timeout,
				CarrierErrorCalls:        carrierErr,
				TrunkLimitExhaustedCalls: trunkLimit,
			},
			Percentages: domain.PercentagesDTO{
				DeliveryRatePercentage:      calcPercentage(delivered, totalDialed),
				AnswerRatePercentage:        calcPercentage(answered, totalDialed),
				AMDDiscardPercentage:        calcPercentage(voicemail, totalDialed),
				InvalidNumberRatePercentage: calcPercentage(invalidNum, totalDialed),
				FailureRatePercentage:       calcPercentage(failed, totalDialed),
				BusyRatePercentage:          calcPercentage(busy, totalDialed),
				NoAnswerRatePercentage:      calcPercentage(noAnswer, totalDialed),
				AbandonmentRatePercentage:   abandonmentRate,
			},
		},
		Durations: domain.DurationsDTO{
			TotalTalkTimeSeconds:   totalTalk,
			AverageTalkTimeSeconds: avgTalk,
			TotalRingTimeSeconds:   totalRing,
			AverageRingTimeSeconds: avgRing,
		},
	}, nil
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
		       trunk_used, recording_file, recording_url, created_at, initiated_at, answered_at, ended_at
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
			&c.TrunkUsed, &c.RecordingFile, &c.RecordingURL, &c.CreatedAt, &c.InitiatedAt, &c.AnsweredAt, &c.EndedAt,
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
		       trunk_used, recording_file, recording_url, created_at, initiated_at, answered_at, ended_at
		FROM cdrs
		WHERE tenant_id = $1 AND id = $2
		LIMIT 1
	`
	var c domain.CDR
	var callTypeStr, dispositionStr string
	err := r.pool.QueryRow(ctx, query, tenantID, cdrID).Scan(
		&c.ID, &c.TenantID, &c.CampaignID, &c.Phone, &c.AgentID, &callTypeStr, &dispositionStr,
		&c.SIPStatus, &c.HangupCause, &c.DurationSeconds, &c.BillsecSeconds, &c.RingSeconds,
		&c.TrunkUsed, &c.RecordingFile, &c.RecordingURL, &c.CreatedAt, &c.InitiatedAt, &c.AnsweredAt, &c.EndedAt,
	)
	if err != nil {
		return nil, err
	}
	c.CallType = domain.CallType(callTypeStr)
	c.Disposition = domain.CallDisposition(dispositionStr)
	return &c, nil
}
