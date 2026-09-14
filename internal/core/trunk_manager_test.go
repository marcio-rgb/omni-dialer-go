package core

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

type mockReportRepoForTM struct {
	lastCDR *domain.CDR
}

func (m *mockReportRepoForTM) SaveCDR(ctx context.Context, cdr *domain.CDR) error {
	m.lastCDR = cdr
	return nil
}

func (m *mockReportRepoForTM) GetCallsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, campaignID *string) (*domain.CallsSummaryResponse, error) {
	return nil, nil
}

func (m *mockReportRepoForTM) ListCDRs(ctx context.Context, filter domain.CDRFilter) (*domain.CDRListResponse, error) {
	return nil, nil
}

func (m *mockReportRepoForTM) GetCDRByID(ctx context.Context, tenantID, cdrID string) (*domain.CDR, error) {
	return nil, nil
}

func TestTrunkManager_PredictiveMachine_SetsVoicemail(t *testing.T) {
	ctx := context.Background()
	cm := NewChannelManager(10, 0, nil)
	reportRepo := &mockReportRepoForTM{}

	tm := NewTrunkManager(nil, nil, nil, cm, reportRepo, nil)

	callID := "pred-test-machine-1"
	activeChan := &domain.ActiveChannel{
		ChannelID: callID,
		TrunkID:   "trunk-vivo",
		TenantID:  "tenant-1",
		Phone:     "11987654321",
		CallType:  domain.CallTypePredictive,
		StartedAt: time.Now(),
	}

	if err := cm.AcquireSlot(ctx, activeChan, false); err != nil {
		t.Fatalf("falha ao alocar slot: %v", err)
	}

	astChannel := "PJSIP/trunk-vivo-00000001"
	uniqueID := "1726000001.1"
	cm.LinkAsteriskChannel(callID, astChannel, uniqueID)

	// Simula detecção de silêncio pelo Vosk EAGI e emissão de PredictiveMachine
	tm.handleUserEvent(ctx, map[string]string{
		"UserEvent":  "PredictiveMachine",
		"Channel":    astChannel,
		"Uniqueid":   uniqueID,
		"Phone":      "11987654321",
		"CampaignId": "camp-1",
		"Cause":      "VOICEMAIL_SILENCE",
	})

	// Verifica se a disposição em memória foi atualizada para VOICEMAIL
	cm.chanMu.RLock()
	ch := cm.activeChannels[callID]
	if ch == nil || ch.Disposition == nil || *ch.Disposition != domain.DispositionVoicemail {
		t.Errorf("esperava disposição VOICEMAIL em memória, obteve %v", ch)
	}
	cm.chanMu.RUnlock()

	// Simula Hangup (Cause 16) pelo Asterisk após caixa postal / silêncio
	tm.handleHangup(ctx, map[string]string{
		"Channel":  astChannel,
		"Uniqueid": uniqueID,
		"Cause":    "16",
	})

	// Assegura que o CDR foi salvo com disposição VOICEMAIL
	if reportRepo.lastCDR == nil {
		t.Fatalf("esperava que o CDR tivesse sido persistido")
	}
	if reportRepo.lastCDR.Disposition != domain.DispositionVoicemail {
		t.Errorf("esperava disposição VOICEMAIL no CDR persistido, obteve %s", reportRepo.lastCDR.Disposition)
	}
}
