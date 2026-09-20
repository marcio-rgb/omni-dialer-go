package core

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

type mockCacheForEvents struct {
	mockCachePredictive
	lastAnsweredLead *domain.AnsweredLeadEvent
}

func (m *mockCacheForEvents) PushAnsweredLead(ctx context.Context, event *domain.AnsweredLeadEvent) error {
	m.lastAnsweredLead = event
	return nil
}

func TestPredictiveEngine_HandlePredictiveHuman_PushesAnsweredLead(t *testing.T) {
	ctx := context.Background()
	ami := &mockAMI{}
	cache := &mockCacheForEvents{}
	cm := NewChannelManager(60, 10, nil)
	cm.RegisterTrunkLimit("trunk-1", 50)
	trunks := &mockTrunkRepo{
		trunk: &domain.Trunk{
			ID:          "trunk-1",
			TenantID:    "tenant-alpha",
			Host:        "10.0.0.1",
			MaxChannels: 50,
			IsEnabled:   true,
		},
	}
	campaigns := &mockCampaignRepo{
		camp: &domain.Campaign{
			ID:        "camp-101",
			TenantID:  "tenant-alpha",
			Status:    domain.CampaignStatusActive,
			TrunkName: "trunk-1",
		},
	}
	leads := &mockLeadRepo{}

	engine := NewPredictiveEngine(ami, cm, cache, campaigns, trunks, leads)

	// Simula canal ativo originado com dados dinâmicos Custom
	callID := "pred-test-uuid-123"
	activeChan := &domain.ActiveChannel{
		ChannelID:  callID,
		TrunkID:    "trunk-1",
		TenantID:   "tenant-alpha",
		CampaignID: strPtr("camp-101"),
		Phone:      "11988887777",
		CPF:        "12345678901",
		Name:       "Carlos Eduardo",
		Att1:       "att_val_1",
		Custom: map[string]string{
			"renda":    "5000",
			"banco":    "033",
			"contrato": "CTR-9988",
		},
		CallType:  domain.CallTypePredictive,
		StartedAt: time.Now(),
	}
	_ = cm.AcquireSlot(ctx, activeChan, false)
	cm.LinkAsteriskChannel(callID, "PJSIP/trunk-1-000001", "unique-123")

	err := engine.HandlePredictiveHuman(ctx, "PJSIP/trunk-1-000001", "unique-123", "11988887777", "camp-101", "12345678901")
	if err != nil {
		t.Fatalf("HandlePredictiveHuman falhou: %v", err)
	}

	if cache.lastAnsweredLead == nil {
		t.Fatalf("esperava evento de lead atendido no cache Redis, mas foi nil")
	}

	ev := cache.lastAnsweredLead
	if ev.Event != "call.answered" {
		t.Errorf("evento incorreto: %s (esperava call.answered)", ev.Event)
	}
	if ev.CallID != callID {
		t.Errorf("callID incorreto: %s (esperava %s)", ev.CallID, callID)
	}
	if ev.TenantID != "tenant-alpha" {
		t.Errorf("tenantID incorreto: %s (esperava tenant-alpha)", ev.TenantID)
	}
	if ev.CampaignID != "camp-101" {
		t.Errorf("campaignID incorreto: %s (esperava camp-101)", ev.CampaignID)
	}
	if ev.Phone != "11988887777" {
		t.Errorf("phone incorreto: %s", ev.Phone)
	}
	if ev.CPF != "12345678901" {
		t.Errorf("CPF incorreto: %s", ev.CPF)
	}
	if ev.Name != "Carlos Eduardo" {
		t.Errorf("Name incorreto: %s", ev.Name)
	}
	if ev.AgentID != "agent-1" {
		t.Errorf("AgentID incorreto: %s (esperava agent-1)", ev.AgentID)
	}
	if ev.Custom == nil {
		t.Fatalf("Custom é nil no evento de lead atendido")
	}
	if ev.Custom["renda"] != "5000" || ev.Custom["banco"] != "033" || ev.Custom["contrato"] != "CTR-9988" {
		t.Errorf("Custom incorreto no evento Redis: %v", ev.Custom)
	}
}

func strPtr(s string) *string {
	return &s
}
