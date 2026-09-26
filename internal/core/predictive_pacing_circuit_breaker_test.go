package core

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type mockReportRepoForPacing struct {
	ports.ReportRepository
	abandoned int64
	answered  int64
}

func (m *mockReportRepoForPacing) Get10MinAbandonStats(ctx context.Context, tenantID, campaignID string) (abandoned, answered int64, err error) {
	return m.abandoned, m.answered, nil
}

type mockWebhookForPacing struct {
	ports.WebhookPort
	lastAlert *domain.SystemAlertWebhookPayload
}

func (m *mockWebhookForPacing) NotifySystemAlert(ctx context.Context, webhookURL string, payload *domain.SystemAlertWebhookPayload) error {
	m.lastAlert = payload
	return nil
}

type mockTenantRepoForPacing struct {
	ports.TenantRepository
	tenant *domain.Tenant
}

func (m *mockTenantRepoForPacing) GetByID(ctx context.Context, tenantID string) (*domain.Tenant, error) {
	return m.tenant, nil
}

func createPacingTestEnv(abandoned, answered int64, consecutiveErrors int64) (*PredictiveEngine, *mockAMI, *ChannelManager, *mockCachePredictive, *mockWebhookForPacing) {
	ami := &mockAMI{}
	cache := &mockCachePredictive{}
	cm := NewChannelManager(60, 10, cache)
	cm.RegisterTrunkLimit("trunk-1", 50)

	trunk := &domain.Trunk{
		ID:          "trunk-1",
		TenantID:    "tenant-1",
		MaxChannels: 50,
		IsEnabled:   true,
		Direction:   domain.DirectionOutbound,
	}
	trunks := &mockTrunkRepo{trunk: trunk}
	campaign := &domain.Campaign{
		ID:             "camp-pacing",
		TenantID:       "tenant-1",
		Status:         domain.CampaignStatusActive,
		Aggressiveness: 1.0,
		TrunkName:      "trunk-1",
	}
	campaigns := &mockCampaignRepo{camp: campaign}
	leads := &mockLeadRepo{total: 100, available: 100}

	var leadBatch []string
	for i := 1; i <= 50; i++ {
		leadJSON, _ := json.Marshal(domain.LeadQueueItem{
			Phone:  fmt.Sprintf("5511999900%02d", i),
			Name:   fmt.Sprintf("Lead %d", i),
			LeadID: int64(i),
		})
		leadBatch = append(leadBatch, string(leadJSON))
	}
	cache.queue = leadBatch

	engine := NewPredictiveEngine(ami, cm, cache, campaigns, trunks, leads)
	engine.SetMinChannelsPerAgent(4) // 4 canais por agente padrão

	repRepo := &mockReportRepoForPacing{abandoned: abandoned, answered: answered}
	engine.SetReportRepository(repRepo)

	tenantRepo := &mockTenantRepoForPacing{tenant: &domain.Tenant{ID: "tenant-1", Webhook: "http://webhook.tenant.com"}}
	engine.SetTenantRepository(tenantRepo)

	wh := &mockWebhookForPacing{}
	engine.SetWebhookClient(wh)

	return engine, ami, cm, cache, wh
}

func TestPredictiveEngine_InFlightSubtraction(t *testing.T) {
	ctx := context.Background()
	engine, _, cm, _, _ := createPacingTestEnv(0, 100, 0)

	// Simula 2 chamadas tocando (ringing) para a campanha
	cm.AcquireSlot(ctx, &domain.ActiveChannel{
		ChannelID:  "chan-ring-1",
		TrunkID:    "trunk-1",
		TenantID:   "tenant-1",
		CampaignID: strPtr("camp-pacing"),
		Phone:      "1199990001",
		IsAnswered: false,
	}, false)
	cm.AcquireSlot(ctx, &domain.ActiveChannel{
		ChannelID:  "chan-ring-2",
		TrunkID:    "trunk-1",
		TenantID:   "tenant-1",
		CampaignID: strPtr("camp-pacing"),
		Phone:      "1199990002",
		IsAnswered: false,
	}, false)

	// 1 agente com ratio 4 -> Demanda calculada 4. Com 2 em ringing, demanda efetiva deve ser 4 - 2 = 2
	req := &domain.PredictiveDemandRequest{
		TenantID:   "tenant-1",
		CampaignID: "camp-pacing",
		AvailableAgents: []domain.AgentDemandDTO{
			{AgentID: "agent-1", SIPRoute: "PJSIP/agent-1@livekit-sip"},
		},
	}
	resp, err := engine.ProcessDemand(ctx, req)
	if err != nil {
		t.Fatalf("falha no ProcessDemand: %v", err)
	}
	if resp.DialingChannels != 2 {
		t.Fatalf("esperava 2 chamadas originadas (4 calculada - 2 ringing), obteve %d", resp.DialingChannels)
	}
}

func TestPredictiveEngine_PacingDamping_20Percent(t *testing.T) {
	ctx := context.Background()
	// Abandono 25 / 100 = 25% (>= 20%) -> Força overdialing 0x (1:1 estrito)
	engine, _, _, _, _ := createPacingTestEnv(25, 100, 0)

	req := &domain.PredictiveDemandRequest{
		TenantID:   "tenant-1",
		CampaignID: "camp-pacing",
		AvailableAgents: []domain.AgentDemandDTO{
			{AgentID: "agent-1", SIPRoute: "PJSIP/agent-1@livekit-sip"},
			{AgentID: "agent-2", SIPRoute: "PJSIP/agent-2@livekit-sip"},
		},
	}
	resp, err := engine.ProcessDemand(ctx, req)
	if err != nil {
		t.Fatalf("falha no ProcessDemand: %v", err)
	}
	// 2 agentes -> estritamente 2 chamadas (1:1)
	if resp.DialingChannels != 2 {
		t.Fatalf("esperava discagem 1:1 (2 canais para 2 agentes) com abandono >= 20%%, obteve %d", resp.DialingChannels)
	}
}

func TestPredictiveEngine_CircuitBreaker_TriggersAlertAndLimitsTo2(t *testing.T) {
	ctx := context.Background()
	// Abandono 100 / 100 = 100% -> Circuit Breaker
	engine, _, _, _, wh := createPacingTestEnv(100, 100, 0)

	req := &domain.PredictiveDemandRequest{
		TenantID:   "tenant-1",
		CampaignID: "camp-pacing",
		AvailableAgents: []domain.AgentDemandDTO{
			{AgentID: "agent-1"},
			{AgentID: "agent-2"},
			{AgentID: "agent-3"},
			{AgentID: "agent-4"},
		},
	}
	resp, err := engine.ProcessDemand(ctx, req)
	if err != nil {
		t.Fatalf("falha no ProcessDemand: %v", err)
	}

	// Teto de sondagem estrito de 2 canais
	if resp.DialingChannels != 2 {
		t.Fatalf("Circuit Breaker deve limitar a no maximo 2 canais de sondagem, obteve %d", resp.DialingChannels)
	}

	if wh.lastAlert == nil {
		t.Fatalf("Webhook de alerta deveria ter sido disparado pelo Circuit Breaker")
	}
	if wh.lastAlert.AlertType != "CIRCUIT_BREAKER_TRUNK_FAILURES" {
		t.Errorf("tipo de alerta incorreto: %s", wh.lastAlert.AlertType)
	}
}

