package ports

import (
	"context"
	"dialer-go/internal/domain"
	"time"
)

type TrunkRepository interface {
	Create(ctx context.Context, trunk *domain.Trunk) error
	GetByID(ctx context.Context, tenantID, trunkID string) (*domain.Trunk, error)
	ListByTenant(ctx context.Context, tenantID string) ([]*domain.Trunk, error)
	Update(ctx context.Context, trunk *domain.Trunk) error
	Delete(ctx context.Context, tenantID, trunkID string) error
	ListAllEnabled(ctx context.Context) ([]*domain.Trunk, error)
}

type RoutingRepository interface {
	GetLastRouting(ctx context.Context, phone string) (*domain.PhoneTrunkMapping, error)
	SaveRouting(ctx context.Context, mapping *domain.PhoneTrunkMapping) error
}

type LeadRepository interface {
	BatchInsert(ctx context.Context, leads []*domain.Lead) (int64, error)
	FetchNextFILOBatch(ctx context.Context, campaignID string, limit int, cooldownHours int) ([]*domain.Lead, error)
	MarkDialing(ctx context.Context, leadID int64) error
	MarkCompleted(ctx context.Context, leadID int64, status domain.LeadStatus) error
	CountCampaignLeads(ctx context.Context, campaignID string) (total, available, dialed int64, err error)
	ResetCampaignCycle(ctx context.Context, campaignID string) (int, error)
	RecoverStaleDialing(ctx context.Context, campaignID string) error
}

type CampaignRepository interface {
	GetByID(ctx context.Context, tenantID, campaignID string) (*domain.Campaign, error)
	ListActive(ctx context.Context, tenantID string) ([]*domain.Campaign, error)
	ListByTenant(ctx context.Context, tenantID string, status *domain.CampaignStatus) ([]*domain.Campaign, error)
	Create(ctx context.Context, campaign *domain.Campaign) error
	Update(ctx context.Context, campaign *domain.Campaign) error
	Delete(ctx context.Context, tenantID, campaignID string) error
	SetStatus(ctx context.Context, tenantID, campaignID string, status domain.CampaignStatus) (*domain.Campaign, error)
	IncrementCycle(ctx context.Context, campaignID string) error
}

type ReportRepository interface {
	SaveCDR(ctx context.Context, cdr *domain.CDR) error
	GetCallsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, campaignID *string) (*domain.CallsSummaryResponse, error)
	ListCDRs(ctx context.Context, filter domain.CDRFilter) (*domain.CDRListResponse, error)
	GetCDRByID(ctx context.Context, tenantID, cdrID string) (*domain.CDR, error)
}
