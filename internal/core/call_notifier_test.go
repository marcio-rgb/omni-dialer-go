package core

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

type mockWebhookPort struct {
	lastPayload *domain.CallEndedWebhookPayload
	lastURL     string
	called      chan struct{}
}

func (m *mockWebhookPort) NotifyCallEnded(ctx context.Context, webhookURL string, payload *domain.CallEndedWebhookPayload) error {
	m.lastURL = webhookURL
	m.lastPayload = payload
	if m.called != nil {
		m.called <- struct{}{}
	}
	return nil
}

func (m *mockWebhookPort) NotifyInjectLead(ctx context.Context, webhookURL string, params *domain.InjectLeadParams) error {
	return nil
}

func TestResolveHangupReason(t *testing.T) {
	tests := []struct {
		disposition domain.CallDisposition
		cause       int
		answered    bool
		expected    string
	}{
		{domain.DispositionAnswered, 16, true, "Chamada Atendida e Encerrada"},
		{domain.DispositionBusy, 17, false, "Número Ocupado"},
		{domain.DispositionNoAnswer, 19, false, "Não Atende / Tempo Esgotado"},
		{domain.DispositionCongestion, 34, false, "Circuito / Tronco Congestionado"},
		{domain.DispositionInvalidNumber, 1, false, "Número Inexistente ou Inválido"},
		{domain.DispositionCancelled, 0, false, "Cancelada pelo Operador"},
		{domain.DispositionFailed, 16, false, "Falha na Ligação / Desconectado pela Operadora"},
	}

	for _, tt := range tests {
		got := ResolveHangupReason(tt.disposition, tt.cause, tt.answered)
		if got != tt.expected {
			t.Errorf("ResolveHangupReason(%s, %d, %v) = %s; esperava %s", tt.disposition, tt.cause, tt.answered, got, tt.expected)
		}
	}
}

func TestCallNotifier_DispatchManualHangup(t *testing.T) {
	mockPort := &mockWebhookPort{called: make(chan struct{}, 1)}
	notifier := NewCallNotifier(mockPort, "http://default-webhook.local")

	agentID := "usr_99"
	customWebhook := "http://custom-webhook.local"
	activeChan := &domain.ActiveChannel{
		ChannelID:  "manual-call-777",
		TrunkID:    "trunk_vivo",
		TenantID:   "tenant_xyz",
		Phone:      "11988887777",
		CallType:   domain.CallTypeManual,
		AgentID:    &agentID,
		WebhookURL: &customWebhook,
		StartedAt:  time.Now().Add(-5 * time.Second),
		IsAnswered: false,
	}

	notifier.DispatchManualHangup(activeChan, domain.DispositionBusy, 17, 5, 0, 5)

	select {
	case <-mockPort.called:
		if mockPort.lastURL != customWebhook {
			t.Errorf("esperava URL customizada %s, obteve %s", customWebhook, mockPort.lastURL)
		}
		if mockPort.lastPayload == nil {
			t.Fatal("payload não deveria ser nil")
		}
		if mockPort.lastPayload.Event != "telephony.manual_call_failed" {
			t.Errorf("esperava evento telephony.manual_call_failed, obteve %s", mockPort.lastPayload.Event)
		}
		if mockPort.lastPayload.HangupReason != "Número Ocupado" {
			t.Errorf("esperava razão 'Número Ocupado', obteve %s", mockPort.lastPayload.HangupReason)
		}
		if mockPort.lastPayload.IsAnswered != false {
			t.Errorf("esperava IsAnswered false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando dispatch do webhook")
	}
}
