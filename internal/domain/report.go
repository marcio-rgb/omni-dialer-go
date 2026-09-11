package domain

import "time"

// CallsSummaryRequest encapsula os filtros validados
type CallsSummaryRequest struct {
	TenantID   string    `json:"tenant_id"`
	StartDate  time.Time `json:"start_date"`
	EndDate    time.Time `json:"end_date"`
	CampaignID *string   `json:"campaign_id,omitempty"`
}

// CallsSummaryResponse payload de saída do relatório analítico
type CallsSummaryResponse struct {
	TenantID  string         `json:"tenant_id"`
	Period    PeriodDTO      `json:"period"`
	Metrics   CallMetricsDTO `json:"metrics"`
	Durations DurationsDTO   `json:"durations"`
	CacheMeta CacheMetaDTO   `json:"cache_meta"`
}

type PeriodDTO struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type CallMetricsDTO struct {
	TotalDialed            int64                  `json:"total_dialed"`
	DeliveredCalls         int64                  `json:"delivered_calls"`
	AnsweredCalls          int64                  `json:"answered_calls"`
	AnsweringMachineCalls  int64                  `json:"answering_machine_calls"`
	InvalidNumberCalls     int64                  `json:"invalid_number_calls"`
	FailedCalls            int64                  `json:"failed_calls"`
	BusyCalls              int64                  `json:"busy_calls"`
	UnansweredCalls        int64                  `json:"unanswered_calls"`
	AbandonedCalls         int64                  `json:"abandoned_calls"`
	CancelledCalls         int64                  `json:"cancelled_calls"`
	InvalidNumberBreakdown InvalidNumberBreakdown `json:"invalid_number_breakdown"`
	FailureBreakdown       FailureBreakdown       `json:"failure_breakdown"`
	Percentages            PercentagesDTO         `json:"percentages"`
}

type InvalidNumberBreakdown struct {
	UnallocatedNumberCalls int64 `json:"unallocated_number_calls"`
	IncompleteNumberCalls  int64 `json:"incomplete_number_calls"`
	UnassignedServiceCalls int64 `json:"unassigned_service_calls"`
}

type FailureBreakdown struct {
	CongestionCalls          int64 `json:"congestion_calls"`
	TimeoutCalls             int64 `json:"timeout_calls"`
	CarrierErrorCalls        int64 `json:"carrier_error_calls"`
	TrunkLimitExhaustedCalls int64 `json:"trunk_limit_exhausted_calls"`
}

type PercentagesDTO struct {
	DeliveryRatePercentage      float64 `json:"delivery_rate_percentage"`
	AnswerRatePercentage        float64 `json:"answer_rate_percentage"`
	AMDDiscardPercentage        float64 `json:"amd_discard_percentage"`
	InvalidNumberRatePercentage float64 `json:"invalid_number_rate_percentage"`
	FailureRatePercentage       float64 `json:"failure_rate_percentage"`
	BusyRatePercentage          float64 `json:"busy_rate_percentage"`
	NoAnswerRatePercentage      float64 `json:"no_answer_rate_percentage"`
	AbandonmentRatePercentage   float64 `json:"abandonment_rate_percentage"`
}

type DurationsDTO struct {
	TotalTalkTimeSeconds   int64 `json:"total_talk_time_seconds"`
	AverageTalkTimeSeconds int64 `json:"average_talk_time_seconds"`
	TotalRingTimeSeconds   int64 `json:"total_ring_time_seconds"`
	AverageRingTimeSeconds int64 `json:"average_ring_time_seconds"`
}

type CacheMetaDTO struct {
	Cached                 bool   `json:"cached"`
	CachedAt               string `json:"cached_at"`
	CacheExpiresAt         string `json:"cache_expires_at"`
	TTLRemainingSeconds    int64  `json:"ttl_remaining_seconds"`
	BufferDurationSeconds  int    `json:"buffer_duration_seconds"`
	RefreshIntervalMinutes int    `json:"refresh_interval_minutes"`
	LastRefreshReason      string `json:"last_refresh_reason"`
}
