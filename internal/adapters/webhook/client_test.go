package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

func TestWebhookClient_NotifyCallEnded_Success(t *testing.T) {
	var receivedPayload domain.CallEndedWebhookPayload
	var receivedTenant string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTenant = r.Header.Get("X-Tenant-Id")
		receivedContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	agentID := "agent_123"

	payload := &domain.CallEndedWebhookPayload{
		Event:           "telephony.manual_call_failed",
		CallID:          "manual-test-1",
		CallType:        domain.CallTypeManual,
		TenantID:        "tenant_abc",
		AgentID:         &agentID,
		Phone:           "5511999998888",
		TrunkUsed:       "trunk_01",
		Disposition:     domain.DispositionBusy,
		HangupCause:     17,
		HangupReason:    "Número Ocupado",
		IsAnswered:      false,
		DurationSeconds: 10,
		BillsecSeconds:  0,
		RingSeconds:     10,
		StartedAt:       time.Now().Add(-10 * time.Second),
		EndedAt:         time.Now(),
		Timestamp:       time.Now().Unix(),
	}

	err := client.NotifyCallEnded(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("esperava sucesso no envio de webhook, obteve: %v", err)
	}

	if receivedTenant != "tenant_abc" {
		t.Errorf("esperava X-Tenant-Id tenant_abc, obteve %s", receivedTenant)
	}
	if receivedContentType != "application/json" {
		t.Errorf("esperava Content-Type application/json, obteve %s", receivedContentType)
	}
	if receivedPayload.CallID != "manual-test-1" {
		t.Errorf("esperava CallID manual-test-1, obteve %s", receivedPayload.CallID)
	}
	if receivedPayload.Disposition != domain.DispositionBusy {
		t.Errorf("esperava Disposition BUSY, obteve %s", receivedPayload.Disposition)
	}
	if receivedPayload.HangupReason != "Número Ocupado" {
		t.Errorf("esperava HangupReason 'Número Ocupado', obteve %s", receivedPayload.HangupReason)
	}
}

func TestWebhookClient_NotifyCallEnded_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	payload := &domain.CallEndedWebhookPayload{
		CallID:   "manual-fail",
		TenantID: "default",
	}

	err := client.NotifyCallEnded(context.Background(), "", payload)
	if err == nil {
		t.Fatal("esperava erro quando o webhook retorna 500, obteve nil")
	}
}
