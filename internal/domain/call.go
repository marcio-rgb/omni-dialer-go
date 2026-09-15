package domain

import "time"

// AgentRedisData representa as informações do operador aguardando chamada na fila dialer:idle_agents
type AgentRedisData struct {
	AgentID     string `json:"agent_id"`
	LiveKitRoom string `json:"livekit_room"`
}

// CustomerMetadata representa os dados completos do cliente/lead transmitidos na perna telefônica SIP
type CustomerMetadata struct {
	CustomerID string `json:"customer_id"`
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Att1       string `json:"att1,omitempty"`
	Att2       string `json:"att2,omitempty"`
	Att3       string `json:"att3,omitempty"`
}


type CallType string
const (
	CallTypePredictive CallType = "PREDICTIVE"
	CallTypeManual     CallType = "MANUAL"
	CallTypeInbound    CallType = "INBOUND"
)

type CallDisposition string
const (
	DispositionDelivered     CallDisposition = "DELIVERED"
	DispositionAnswered      CallDisposition = "ANSWERED"
	DispositionVoicemail     CallDisposition = "VOICEMAIL"
	DispositionAMDMachine    CallDisposition = "AMD_MACHINE"
	DispositionInvalidNumber CallDisposition = "INVALID_NUMBER"
	DispositionFailed        CallDisposition = "FAILED"
	DispositionCongestion    CallDisposition = "CONGESTION"
	DispositionBusy          CallDisposition = "BUSY"
	DispositionNoAnswer      CallDisposition = "NO_ANSWER"
	DispositionAbandoned     CallDisposition = "ABANDONED"
	DispositionCancelled     CallDisposition = "CANCELLED"
	DispositionTrunkCapacity CallDisposition = "TRUNK_CAPACITY"
)

// CDR (Call Detail Record) representa o histórico persistido de uma perna de chamada.
type CDR struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	CampaignID      *string         `json:"campaign_id,omitempty"`
	Phone           string          `json:"phone"`
	AgentID         *string         `json:"agent_id,omitempty"`
	CallType        CallType        `json:"call_type"`
	Disposition     CallDisposition `json:"disposition"`
	SIPStatus       *int            `json:"sip_status,omitempty"`
	HangupCause     *int            `json:"hangup_cause,omitempty"`
	DurationSeconds int             `json:"duration_seconds"`
	BillsecSeconds  int             `json:"billsec_seconds"`
	RingSeconds     int             `json:"ring_seconds"`
	TrunkUsed       string          `json:"trunk_used"`
	RecordingFile   *string         `json:"recording_file,omitempty"`
	RecordingURL    *string         `json:"recording_url,omitempty"`
	Transcription   *string         `json:"transcription,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	InitiatedAt     *time.Time      `json:"initiated_at,omitempty"`
	AnsweredAt      *time.Time      `json:"answered_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
}

// ActiveChannel representa o registro em memória rápida de um canal em conversação ou discagem.
type ActiveChannel struct {
	ChannelID   string           `json:"channel_id"`
	TrunkID     string           `json:"trunk_id"`
	TenantID    string           `json:"tenant_id"`
	CampaignID  *string          `json:"campaign_id,omitempty"`
	Phone         string           `json:"phone"`
	CPF           string           `json:"cpf,omitempty"`
	Name          string           `json:"name,omitempty"`
	Att1          string           `json:"att1,omitempty"`
	Att2          string           `json:"att2,omitempty"`
	Att3          string           `json:"att3,omitempty"`
	CallType      CallType         `json:"call_type"`
	AgentID     *string          `json:"agent_id,omitempty"`
	SIPRoute    *string          `json:"sip_route,omitempty"`
	WebhookURL  *string          `json:"webhook_url,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	AnsweredAt    *time.Time       `json:"answered_at,omitempty"`
	IsAnswered    bool             `json:"is_answered"`
	Disposition   *CallDisposition `json:"disposition,omitempty"`
	RecordingFile string           `json:"recording_file,omitempty"`
	Transcription string           `json:"transcription,omitempty"`
}

// PhoneTrunkMapping representa o vínculo O(1) de último tronco para chamadas receptivas.
type PhoneTrunkMapping struct {
	Phone        string    `json:"phone"`
	LastTrunk    string    `json:"last_trunk"`
	LastProject  string    `json:"last_project"`
	LastSIPRoute string    `json:"last_sip_route"`
	TenantID     string    `json:"tenant_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ManualCallRequest struct {
	TenantID   string `json:"tenant_id"`
	AgentID    string `json:"agent_id"`
	Phone      string `json:"phone"`
	SIPRoute   string `json:"sip_route"`
	TrunkID    string `json:"trunk_id,omitempty"`
	LeadName   string `json:"lead_name,omitempty"`
	LeadCPF    string `json:"lead_cpf,omitempty"`
	Name       string `json:"name,omitempty"`
	CPF        string `json:"cpf,omitempty"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type LeadQueueItem struct {
	Phone     string `json:"phone"`
	CPF       string `json:"cpf,omitempty"`
	Name      string `json:"name,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	WorkWord  string `json:"work_word,omitempty"`
	Att1      string `json:"att1,omitempty"`
	Att2      string `json:"att2,omitempty"`
	Att3      string `json:"att3,omitempty"`
	LeadID    int64  `json:"lead_id,omitempty"`
}

type ManualCallResponse struct {
	CallID    string `json:"call_id"`
	Status    string `json:"status"` // "dialing"
	TrunkUsed string `json:"trunk_used"`
}

// CallEndedWebhookPayload representa o payload canônico disparado via webhook ao término da chamada.
type CallEndedWebhookPayload struct {
	Event           string          `json:"event"` // "telephony.call_ended"
	CallID          string          `json:"call_id"`
	CallType        CallType        `json:"call_type"`
	TenantID        string          `json:"tenant_id"`
	AgentID         *string         `json:"agent_id,omitempty"`
	Phone           string          `json:"phone"`
	TrunkUsed       string          `json:"trunk_used"`
	Disposition     CallDisposition `json:"disposition"`
	HangupCause     int             `json:"hangup_cause"`
	HangupReason    string          `json:"hangup_reason"`
	IsAnswered      bool            `json:"is_answered"`
	DurationSeconds int             `json:"duration_seconds"`
	BillsecSeconds  int             `json:"billsec_seconds"`
	RingSeconds     int             `json:"ring_seconds"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         time.Time       `json:"ended_at"`
	RecordingURL    string          `json:"recording_url,omitempty"`
	Transcription   string          `json:"transcription,omitempty"`
	Timestamp       int64           `json:"timestamp"`
}

type AgentDemandDTO struct {
	AgentID         string `json:"agent_id"`
	PriorityOrder   int    `json:"priority_order"`
	SIPRoute        string `json:"sip_route"`
	IdleTimeSeconds int    `json:"idle_time_seconds"`
}

type PredictiveDemandRequest struct {
	TenantID            string           `json:"tenant_id"`
	CampaignID          string           `json:"campaign_id"`
	Aggressiveness      *float64         `json:"aggressiveness,omitempty"`
	MinChannelsPerAgent *int             `json:"min_channels_per_agent,omitempty"`
	AvailableAgents     []AgentDemandDTO `json:"available_agents"`
}

type PredictiveDemandResponse struct {
	CampaignID      string `json:"campaign_id"`
	DialingChannels int    `json:"dialing_channels"`
	Status          string `json:"status"`
}

// CDRFilter define os parâmetros de busca e paginação para consulta de CDRs.
type CDRFilter struct {
	TenantID    string
	CampaignID  *string
	Phone       *string
	Disposition *CallDisposition
	Search      *string
	StartDate   *time.Time
	EndDate     *time.Time
	Page        int
	Limit       int
}

// CDRListResponse representa a lista paginada de CDRs retornada pela API.
type CDRListResponse struct {
	Total      int64  `json:"total"`
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	TotalPages int    `json:"total_pages"`
	CDRs       []*CDR `json:"cdrs"`
}
