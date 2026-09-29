package domain

import "time"

// AgentRedisData representa as informações do operador aguardando chamada na fila dialer:idle_agents
type AgentRedisData struct {
	AgentID     string `json:"agent_id"`
	LiveKitRoom string `json:"livekit_room"`
}

// CustomerMetadata representa os dados completos do cliente/lead transmitidos na perna telefônica SIP
type CustomerMetadata struct {
	CustomerID string            `json:"customer_id"`
	Name       string            `json:"name"`
	Phone      string            `json:"phone"`
	Att1       string            `json:"att1,omitempty"`
	Att2       string            `json:"att2,omitempty"`
	Att3       string            `json:"att3,omitempty"`
	Custom     map[string]string `json:"custom,omitempty"`
}


type CallType string
const (
	CallTypePredictive CallType = "PREDICTIVE"
	CallTypeManual     CallType = "MANUAL"
	CallTypeInbound    CallType = "INBOUND"
	CallTypeReceptive  CallType = "RECEPTIVE"
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
	AgentName       *string         `json:"agent_name,omitempty"`
	CallType        CallType        `json:"call_type"`
	Disposition     CallDisposition `json:"disposition"`
	SIPStatus       *int            `json:"sip_status,omitempty"`
	HangupCause     *int            `json:"hangup_cause,omitempty"`
	DurationSeconds int             `json:"duration_seconds"`
	BillsecSeconds  int             `json:"billsec_seconds"`
	RingSeconds     int             `json:"ring_seconds"`
	TrunkUsed       string          `json:"trunk_used"`
	LeadID          *int64          `json:"lead_id,omitempty"`
	LeadName        *string         `json:"lead_name,omitempty"`
	LeadCPF         *string         `json:"lead_cpf,omitempty"`
	AMDStatus       *string         `json:"amd_status,omitempty"`
	AMDCause        *string         `json:"amd_cause,omitempty"`
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
	ChannelID     string            `json:"channel_id"`
	TrunkID       string            `json:"trunk_id"`
	TenantID      string            `json:"tenant_id"`
	CampaignID    *string           `json:"campaign_id,omitempty"`
	LeadID        *int64            `json:"lead_id,omitempty"`
	Phone         string            `json:"phone"`
	CPF           string            `json:"cpf,omitempty"`
	Name          string            `json:"name,omitempty"`
	Att1          string            `json:"att1,omitempty"`
	Att2          string            `json:"att2,omitempty"`
	Att3          string            `json:"att3,omitempty"`
	Custom        map[string]string `json:"custom,omitempty"`
	CallType      CallType          `json:"call_type"`
	AgentID       *string           `json:"agent_id,omitempty"`
	SIPRoute      *string           `json:"sip_route,omitempty"`
	WebhookURL    *string           `json:"webhook_url,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	AnsweredAt    *time.Time        `json:"answered_at,omitempty"`
	IsAnswered    bool              `json:"is_answered"`
	Disposition   *CallDisposition  `json:"disposition,omitempty"`
	AMDStatus     *string           `json:"amd_status,omitempty"`
	AMDCause      *string           `json:"amd_cause,omitempty"`
	RecordingFile string            `json:"recording_file,omitempty"`
	Transcription string            `json:"transcription,omitempty"`
}

// CDREventType define a tipagem estrita de eventos de ciclo de vida de CDR enfileirados via Redis Streams.
type CDREventType string

const (
	CDREventInitiated       CDREventType = "CALL_INITIATED"
	CDREventOriginateFailed CDREventType = "CALL_ORIGINATE_FAILED"
	CDREventAnswered        CDREventType = "CALL_ANSWERED"
	CDREventAgentConnected  CDREventType = "CALL_AGENT_CONNECTED"
	CDREventAMDClassified   CDREventType = "CALL_AMD_CLASSIFIED"
	CDREventHangup          CDREventType = "CALL_HANGUP"
)

// CDREvent encapsula o envelope completo de ciclo de vida para persistência via Redis Stream.
type CDREvent struct {
	Type            CDREventType    `json:"type"`
	CallID          string          `json:"call_id"`
	TenantID        string          `json:"tenant_id"`
	CampaignID      *string         `json:"campaign_id,omitempty"`
	LeadID          *int64          `json:"lead_id,omitempty"`
	Phone           string          `json:"phone"`
	LeadName        *string         `json:"lead_name,omitempty"`
	LeadCPF         *string         `json:"lead_cpf,omitempty"`
	AgentID         *string         `json:"agent_id,omitempty"`
	CallType        CallType        `json:"call_type"`
	Disposition     CallDisposition `json:"disposition"`
	SIPStatus       *int            `json:"sip_status,omitempty"`
	HangupCause     *int            `json:"hangup_cause,omitempty"`
	DurationSeconds int             `json:"duration_seconds"`
	BillsecSeconds  int             `json:"billsec_seconds"`
	RingSeconds     int             `json:"ring_seconds"`
	TrunkUsed       string          `json:"trunk_used"`
	AMDStatus       *string         `json:"amd_status,omitempty"`
	AMDCause        *string         `json:"amd_cause,omitempty"`
	RecordingFile   *string         `json:"recording_file,omitempty"`
	RecordingURL    *string         `json:"recording_url,omitempty"`
	Transcription   *string         `json:"transcription,omitempty"`
	StartedAt       time.Time       `json:"started_at"`
	InitiatedAt     *time.Time      `json:"initiated_at,omitempty"`
	AnsweredAt      *time.Time      `json:"answered_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	Timestamp       int64           `json:"timestamp"`
}

// CDREventMessage representa a mensagem lida de um Redis Stream com seu StreamID único.
type CDREventMessage struct {
	ID    string    `json:"id"`
	Event *CDREvent `json:"event"`
}

// TranscriptionJob define um job de transcrição assíncrona pós-chamada para o Faster-Whisper.
type TranscriptionJob struct {
	CallID        string    `json:"call_id"`
	TenantID      string    `json:"tenant_id"`
	RecordingFile string    `json:"recording_file"`
	Duration      int       `json:"duration"`
	Language      string    `json:"language,omitempty"`
	EnqueuedAt    time.Time `json:"enqueued_at"`
}

// TranscriptionJobMessage representa o job de transcrição lido do Redis Stream.
type TranscriptionJobMessage struct {
	ID  string            `json:"id"`
	Job *TranscriptionJob `json:"job"`
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
	LeadID     string `json:"lead_id,omitempty"`
	LeadName   string `json:"lead_name,omitempty"`
	LeadCPF    string `json:"lead_cpf,omitempty"`
	Name       string `json:"name,omitempty"`
	CPF        string `json:"cpf,omitempty"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type LeadQueueItem struct {
	Phone     string            `json:"phone"`
	CPF       string            `json:"cpf,omitempty"`
	Name      string            `json:"name,omitempty"`
	FirstName string            `json:"first_name,omitempty"`
	WorkWord  string            `json:"work_word,omitempty"`
	Att1      string            `json:"att1,omitempty"`
	Att2      string            `json:"att2,omitempty"`
	Att3      string            `json:"att3,omitempty"`
	Custom    map[string]string `json:"custom,omitempty"`
	LeadID    int64             `json:"lead_id,omitempty"`
}

// AnsweredLeadEvent representa o evento enfileirado no Redis quando a chamada é atendida por humano.
type AnsweredLeadEvent struct {
	Event      string            `json:"event"` // "call.answered"
	CallID     string            `json:"call_id"`
	Channel    string            `json:"channel,omitempty"`
	TenantID   string            `json:"tenant_id"`
	CampaignID string            `json:"campaign_id,omitempty"`
	LeadID     int64             `json:"lead_id,omitempty"`
	Phone      string            `json:"phone"`
	CPF        string            `json:"cpf,omitempty"`
	Name       string            `json:"name,omitempty"`
	FirstName  string            `json:"first_name,omitempty"`
	AgentID    string            `json:"agent_id,omitempty"`
	RoomName   string            `json:"room_name,omitempty"`
	Custom     map[string]string `json:"custom,omitempty"`
	AnsweredAt time.Time         `json:"answered_at"`
	Timestamp  int64             `json:"timestamp"`
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
//
// @pattern Value Object (Query Specification)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
type CDRFilter struct {
	TenantID     string
	CampaignID   *string
	AgentID      *string
	CallType     *CallType
	TrunkUsed    *string
	LeadCPF      *string
	LeadID       *int64
	Phone        *string
	Disposition  *CallDisposition   // Filtro singular (retrocompatibilidade: ?disposition=ANSWERED)
	Dispositions []CallDisposition  // Filtro múltiplo via CSV (?disposition=ANSWERED,DELIVERED)
	AMDStatus    *string
	HasRecording *bool
	MinBillsec   *int
	MaxBillsec   *int
	MinDuration  *int
	MaxDuration  *int
	Search       *string
	StartDate    *time.Time
	EndDate      *time.Time
	SortBy       string // Whitelist canônica: id, tenant_id, campaign_id, agent_id, trunk_used, phone, lead_name, lead_cpf, lead_id, call_type, disposition, sip_status, hangup_cause, amd_status, amd_cause, duration_seconds, billsec_seconds, ring_seconds, created_at, initiated_at, answered_at, ended_at, recording_file, recording_url, transcription
	SortOrder    string // "asc" ou "desc"
	Page         int
	Limit        int
	IncludeTotal bool // true por padrão; false pula COUNT(*) e retorna Total=nil
}

// CDRListResponse representa a lista paginada de CDRs retornada pela API.
type CDRListResponse struct {
	Total      *int64 `json:"total"`       // nil quando include_total=false
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	TotalPages *int   `json:"total_pages"` // nil quando include_total=false
	HasMore    bool   `json:"has_more"`    // true se len(CDRs) == limit
	CDRs       []*CDR `json:"cdrs"`
}


// SystemAlertWebhookPayload representa a notificação de alerta operacional emitida para o Tenant em caso de Circuit Breaker.
type SystemAlertWebhookPayload struct {
	Event            string    `json:"event"` // "telephony.system_alert"
	TenantID         string    `json:"tenant_id"`
	CampaignID       string    `json:"campaign_id"`
	AlertType        string    `json:"alert_type"`
	AbandonRate10m   float64   `json:"abandon_rate_10m"`
	ConsecutiveFails int64     `json:"consecutive_failures"`
	Message          string    `json:"message"`
	ActionRequired   string    `json:"action_required"`
	Timestamp        time.Time `json:"timestamp"`
}

