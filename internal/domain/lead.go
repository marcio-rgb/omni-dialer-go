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
