package core

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

type mockLeadRepo struct {
	total     int64
	available int64
	dialed    int64
}

func (m *mockLeadRepo) BatchInsert(ctx context.Context, leads []*domain.Lead) (int64, error) {
	return int64(len(leads)), nil
}
func (m *mockLeadRepo) FetchNextFILOBatch(ctx context.Context, campaignID string, limit int, cooldownHours int) ([]*domain.Lead, error) {
	return nil, nil
}
func (m *mockLeadRepo) MarkDialing(ctx context.Context, leadID int64) error { return nil }
func (m *mockLeadRepo) MarkCompleted(ctx context.Context, leadID int64, status domain.LeadStatus) error {
	return nil
}
func (m *mockLeadRepo) CountCampaignLeads(ctx context.Context, campaignID string) (total, available, dialed int64, err error) {
	return m.total, m.available, m.dialed, nil
}
func (m *mockLeadRepo) ResetCampaignCycle(ctx context.Context, campaignID string) (int, error) {
	return 0, nil
}
func (m *mockLeadRepo) RecoverStaleDialing(ctx context.Context, campaignID string) error {
	return nil
}

type mockCampaignRepo struct {
	camp *domain.Campaign
}

func (m *mockCampaignRepo) GetByID(ctx context.Context, tenantID, campaignID string) (*domain.Campaign, error) {
	return m.camp, nil
}
func (m *mockCampaignRepo) ListActive(ctx context.Context, tenantID string) ([]*domain.Campaign, error) {
	return []*domain.Campaign{m.camp}, nil
}
func (m *mockCampaignRepo) SetStatus(ctx context.Context, tenantID, campaignID string, status domain.CampaignStatus) (*domain.Campaign, error) {
	m.camp.Status = status
	return m.camp, nil
}
func (m *mockCampaignRepo) IncrementCycle(ctx context.Context, campaignID string) error {
	m.camp.CycleCount++
	return nil
}

func TestSaturationService_MetricsAndBurnRate(t *testing.T) {
	ctx := context.Background()
	campName := "Campanha Antiga"
	camp := &domain.Campaign{
		ID:         "camp_123",
		TenantID:   "org_alpha",
		Name:       &campName,
		CycleCount: 1, // Ciclo 1 = RECICLADA
		CreatedAt:  time.Now(),
	}

	leadsMock := &mockLeadRepo{
		total:     1000,
		available: 100,
		dialed:    900,
	}

	service := NewSaturationService(leadsMock, &mockCampaignRepo{camp: camp})
	data, err := service.GetCampaignSaturation(ctx, "org_alpha", "camp_123")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if data.BurnRatePercentage != 90.0 {
		t.Errorf("BurnRatePercentage esperado 90.0, obtido %f", data.BurnRatePercentage)
	}

	if data.SaturationLevel != domain.SaturationCritica {
		t.Errorf("SaturationLevel esperado CRITICA (>=90%%), obtido %s", data.SaturationLevel)
	}

	if data.RefillUrgency != "CRITICAL" {
		t.Errorf("RefillUrgency esperado CRITICAL, obtido %s", data.RefillUrgency)
	}

	if data.CampaignID != "camp_123" {
		t.Errorf("CampaignID esperado camp_123, obtido %s", data.CampaignID)
	}
}
