package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"dialer-go/internal/domain"
)

// GetCallsSummary executa a query agregada em passada única no PostgreSQL
//
// @pattern Repository & Unit of Work
// @governedBy docs/rules/CAMPAIGN_SATURATION.md#4-política-de-cache-de-15-minutos-para-métricas-operacionais
//
// @preExecution
// - Validação de conexão ativa no pool `pgxpool.Pool`
//
// @postExecution
// - Agregação em passada única com FILTER (WHERE ...) na tabela `cdrs`
// - Mapeamento e cálculo de taxas e métricas operacionais consolidadas
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
