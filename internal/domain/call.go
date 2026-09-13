package domain

import "time"

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
	Phone       string           `json:"phone"`
	CallType    CallType         `json:"call_type"`
	AgentID     *string          `json:"agent_id,omitempty"`
	SIPRoute    *string          `json:"sip_route,omitempty"`
	WebhookURL  *string          `json:"webhook_url,omitempty"`
	StartedAt   time.Time        `json:"started_at"`
	IsAnswered  bool             `json:"is_answered"`
	Disposition *CallDisposition `json:"disposition,omitempty"`
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
	Phone  string `json:"phone"`
	CPF    string `json:"cpf,omitempty"`
	Name   string `json:"name,omitempty"`
	LeadID int64  `json:"lead_id,omitempty"`
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
	Timestamp       int64           `json:"timestamp"`
}

type AgentDemandDTO struct {
	AgentID         string `json:"agent_id"`
	PriorityOrder   int    `json:"priority_order"`
	SIPRoute        string `json:"sip_route"`
	IdleTimeSeconds int    `json:"idle_time_seconds"`
}

type PredictiveDemandRequest struct {
	TenantID        string           `json:"tenant_id"`
	CampaignID      string           `json:"campaign_id"`
	AvailableAgents []AgentDemandDTO `json:"available_agents"`
}

type PredictiveDemandResponse struct {
	CampaignID      string `json:"campaign_id"`
	DialingChannels int    `json:"dialing_channels"`
	Status          string `json:"status"`
}
