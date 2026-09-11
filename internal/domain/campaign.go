package domain

import "time"

type CampaignMode string
const (
	CampaignModePredictive CampaignMode = "PREDICTIVE"
	CampaignModePower      CampaignMode = "POWER"
	CampaignModeManual     CampaignMode = "MANUAL"
)

type CampaignStatus string
const (
	CampaignStatusActive    CampaignStatus = "active"
	CampaignStatusPaused    CampaignStatus = "paused"
	CampaignStatusExhausted CampaignStatus = "exhausted"
)

type SaturationLevel string
const (
	SaturationNova      SaturationLevel = "NOVA"
	SaturationReciclada SaturationLevel = "RECICLADA"
	SaturationCritica   SaturationLevel = "CRITICA"
)

type Campaign struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	Name            *string         `json:"name,omitempty"`
	Mode            CampaignMode    `json:"mode"`
	Status          CampaignStatus  `json:"status"`
	Aggressiveness  float64         `json:"aggressiveness"`
	TrunkName       string          `json:"trunk_name"`
	CycleCount      int             `json:"cycle_count"`
	SaturationLevel SaturationLevel `json:"saturation_level"`
	LastCycleAt     *time.Time      `json:"last_cycle_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// DTOs para Toggle de Campanha (POST /api/v1/campaigns/toggle)
type ToggleCampaignRequest struct {
	CampaignID string `json:"campaign_id"`
	TenantID   string `json:"tenant_id"`
	Enable     bool   `json:"enable"`
}

type ToggleCampaignResponseData struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type ToggleCampaignResponse struct {
	Success bool                       `json:"success"`
	Data    ToggleCampaignResponseData `json:"data"`
}

// DTOs para Relatório de Saturação (sem campaign_name):
type CampaignSaturationResponse struct {
	Success bool                   `json:"success"`
	Data    CampaignSaturationData `json:"data"`
}

type CampaignSaturationData struct {
	CampaignID           string          `json:"campaign_id"`
	TenantID             string          `json:"tenant_id"`
	SaturationLevel      SaturationLevel `json:"saturation_level"`
	TotalLeads           int64           `json:"total_leads"`
	AvailableLeads       int64           `json:"available_leads"`
	DialedLeads          int64           `json:"dialed_leads"`
	CycleCount           int             `json:"cycle_count"`
	BurnRatePercentage   float64         `json:"burn_rate_percentage"`
	RefillUrgency        string          `json:"refill_urgency"` // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	InfiniteLoopingMode  string          `json:"infinite_looping_mode"` // "FILO_ACTIVE"
	LastRefillAt         *time.Time      `json:"last_refill_at,omitempty"`
	EstimatedExhaustionH float64         `json:"estimated_exhaustion_hours"`
}

type TenantSaturationConsolidatedResponse struct {
	Success bool                     `json:"success"`
	Data    TenantSaturationOverview `json:"data"`
}

type TenantSaturationOverview struct {
	TenantID          string                   `json:"tenant_id"`
	ActiveCampaigns   int                      `json:"active_campaigns"`
	CriticalCount     int                      `json:"critical_count"`
	RefillUrgentCount int                      `json:"refill_urgent_count"`
	Campaigns         []CampaignSaturationData `json:"campaigns"`
}
