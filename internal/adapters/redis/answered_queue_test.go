package redis

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

func TestRedisAdapter_PushAnsweredLead_NilSafe(t *testing.T) {
	ctx := context.Background()
	var adapter *RedisAdapter
	err := adapter.PushAnsweredLead(ctx, &domain.AnsweredLeadEvent{})
	if err != nil {
		t.Errorf("esperava erro nil para adapter nil, obteve %v", err)
	}

	adapter = &RedisAdapter{client: nil}
	err = adapter.PushAnsweredLead(ctx, nil)
	if err != nil {
		t.Errorf("esperava erro nil para evento nil, obteve %v", err)
	}
}

func TestRedisAdapter_AnsweredLeadEvent_Serialization(t *testing.T) {
	ev := &domain.AnsweredLeadEvent{
		Event:      "call.answered",
		CallID:     "pred-12345",
		Channel:    "PJSIP/trunk_01-000001",
		TenantID:   "tenant_alpha",
		CampaignID: "camp_vendas",
		LeadID:     998877,
		Phone:      "11988887777",
		CPF:        "12345678901",
		Name:       "Carlos Eduardo",
		FirstName:  "carlos",
		AgentID:    "agent_10",
		RoomName:   "sala_agente_agent_10",
		Custom: map[string]string{
			"renda":    "5000",
			"banco":    "033",
			"contrato": "CTR-9988",
		},
		AnsweredAt: time.Now(),
		Timestamp:  time.Now().Unix(),
	}

	if ev.Custom["renda"] != "5000" {
		t.Errorf("Custom[renda] incorreto: %s", ev.Custom["renda"])
	}
	if ev.LeadID != 998877 {
		t.Errorf("LeadID incorreto: %d", ev.LeadID)
	}
}
