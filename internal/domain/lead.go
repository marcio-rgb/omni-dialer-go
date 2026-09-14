package domain

import "time"

type LeadStatus string
const (
	LeadStatusNew       LeadStatus = "NEW"
	LeadStatusQueued    LeadStatus = "QUEUED"
	LeadStatusDialing   LeadStatus = "DIALING"
	LeadStatusCompleted LeadStatus = "COMPLETED"
	LeadStatusCooldown  LeadStatus = "COOLDOWN"
)

type Lead struct {
	ID            int64      `json:"id"`
	CampaignID    string     `json:"campaign_id"`
	TenantID      string     `json:"tenant_id"`
	CPF           string     `json:"cpf"`
	Phone         string     `json:"phone"`
	Name          string     `json:"name,omitempty"`
	Status        LeadStatus `json:"status"`
	AttemptsCount int        `json:"attempts_count"`
	LastDialedAt  *time.Time `json:"last_dialed_at,omitempty"`
	DialedAt      *time.Time `json:"dialed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// DTOs para Refill de Mailing (POST /api/v1/campaigns/refill)
type RefillRequest struct {
	TenantID   string `json:"tenant_id"`
	CampaignID string `json:"campaign_id"`
	FileURI    string `json:"file_uri"` // URI MinIO s3://mailings/camp101/batch.zip
}

type RefillResponse struct {
	CampaignID   string `json:"campaign_id"`
	TenantID     string `json:"tenant_id"`
	TotalRows    int64  `json:"total_rows"`
	ValidRows    int64  `json:"valid_rows"`
	InvalidRows  int64  `json:"invalid_rows"`
	Status       string `json:"status"` // "INGESTED"
	QueueSize    int64  `json:"queue_size"`
}

// BatchLeadItem representa um lead individual na carga via JSON.
type BatchLeadItem struct {
	CPF      string `json:"cpf"`
	Phone    string `json:"phone"`
	Name     string `json:"name,omitempty"`
	AudioKey string `json:"audio_key,omitempty"`
}

// BatchLeadRequest contrato para carga de lote de leads em campanha preditiva.
type BatchLeadRequest struct {
	CampaignID string          `json:"campaign_id"`
	TenantID   string          `json:"tenant_id"`
	Leads      []BatchLeadItem `json:"leads"`
}

// BatchLeadResponse relatório do processamento de carga e pré-síntese de áudios.
type BatchLeadResponse struct {
	CampaignID           string `json:"campaign_id"`
	TotalReceived        int    `json:"total_received"`
	LeadsQueued          int    `json:"leads_queued"`
	NewAudiosSynthesized int    `json:"new_audios_synthesized"`
	CachedAudiosCount    int    `json:"cached_audios_count"`
	ElapsedMs            int64  `json:"elapsed_ms"`
}

