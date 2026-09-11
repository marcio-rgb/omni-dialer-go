package core

import (
	"context"
	"math"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type SaturationService struct {
	leads     ports.LeadRepository
	campaigns ports.CampaignRepository
}

func NewSaturationService(leads ports.LeadRepository, campaigns ports.CampaignRepository) *SaturationService {
	return &SaturationService{
		leads:     leads,
		campaigns: campaigns,
	}
}

// GetCampaignSaturation calcula o status individual de saturação sem expor campaign_name.
func (ss *SaturationService) GetCampaignSaturation(ctx context.Context, tenantID, campaignID string) (*domain.CampaignSaturationData, error) {
	camp, err := ss.campaigns.GetByID(ctx, tenantID, campaignID)
	if err != nil || camp == nil {
		return nil, domain.NewErrNotFound("CAMPAIGN_NOT_FOUND", "Campanha não encontrada")
	}

	total, available, dialed, err := ss.leads.CountCampaignLeads(ctx, campaignID)
	if err != nil {
		return nil, domain.NewErrInternal("Falha ao contabilizar métricas de leads da campanha")
	}

	burnRate := 0.0
	if total > 0 {
		burnRate = (float64(dialed) / float64(total)) * 100.0
		burnRate = math.Round(burnRate*100) / 100
	}

	satLevel := domain.SaturationNova
	refillUrgency := "LOW"

	if camp.CycleCount > 0 {
		satLevel = domain.SaturationReciclada
		refillUrgency = "MEDIUM"
	}

	if available == 0 || burnRate >= 90.0 {
		satLevel = domain.SaturationCritica
		refillUrgency = "CRITICAL"
	} else if available < int64(float64(total)*0.2) {
		refillUrgency = "HIGH"
	}

	// Estimativa de horas de esgotamento baseada em consumo médio de 300 leads/hora
	estHours := 0.0
	if available > 0 {
		estHours = math.Round((float64(available)/300.0)*10) / 10
	}

	return &domain.CampaignSaturationData{
		CampaignID:           camp.ID,
		TenantID:             camp.TenantID,
		SaturationLevel:      satLevel,
		TotalLeads:           total,
		AvailableLeads:       available,
		DialedLeads:          dialed,
		CycleCount:           camp.CycleCount,
		BurnRatePercentage:   burnRate,
		RefillUrgency:        refillUrgency,
		InfiniteLoopingMode:  "FILO_ACTIVE",
		EstimatedExhaustionH: estHours,
	}, nil
}

// GetConsolidatedSaturation compila a visão de saturação de todas as campanhas ativas do tenant.
func (ss *SaturationService) GetConsolidatedSaturation(ctx context.Context, tenantID string) (*domain.TenantSaturationOverview, error) {
	activeCamps, err := ss.campaigns.ListActive(ctx, tenantID)
	if err != nil {
		return nil, domain.NewErrInternal("Falha ao listar campanhas do tenant")
	}

	var list []domain.CampaignSaturationData
	criticalCount := 0
	refillUrgentCount := 0

	for _, c := range activeCamps {
		data, err := ss.GetCampaignSaturation(ctx, tenantID, c.ID)
		if err == nil && data != nil {
			list = append(list, *data)
			if data.SaturationLevel == domain.SaturationCritica {
				criticalCount++
			}
			if data.RefillUrgency == "HIGH" || data.RefillUrgency == "CRITICAL" {
				refillUrgentCount++
			}
		}
	}

	return &domain.TenantSaturationOverview{
		TenantID:          tenantID,
		ActiveCampaigns:   len(list),
		CriticalCount:     criticalCount,
		RefillUrgentCount: refillUrgentCount,
		Campaigns:         list,
	}, nil
}
