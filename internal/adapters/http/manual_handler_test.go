package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

func TestManualHandler_Hangup(t *testing.T) {
	ami := &mockAMIForQueue{}
	cache := &mockCacheForQueue{}
	cm := core.NewChannelManager(100, 10, cache)
	engine := core.NewManualEngine(ami, cm, nil, cache)
	handler := NewManualHandler(engine)

	// Registra canal em andamento
	ctx := context.Background()
	agentID := "emerson"
	callID := "call-test-999"
	ch := &domain.ActiveChannel{
		ChannelID: callID,
		TrunkID:   "trunk-1",
		CallType:  domain.CallTypeManual,
		Phone:     "11988887777",
		AgentID:   &agentID,
		TenantID:  "default",
	}
	_ = cm.AcquireSlot(ctx, ch, true)
	cm.LinkAsteriskChannel(callID, "PJSIP/vivo-000001", "1790000000.1")

	// Requisição de Hangup por AgentID
	payload := domain.HangupCallRequest{
		AgentID: "emerson",
		Cause:   16,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/calls/hangup", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Hangup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 OK, obteve %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("erro ao decodificar resposta JSON: %v", err)
	}

	if res["success"] != true {
		t.Errorf("esperava success=true, obteve %v", res["success"])
	}
}
