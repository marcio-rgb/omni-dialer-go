package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type mockAMIForQueue struct {
	lastQueueAction string
	lastQueueName   string
	lastIface       string
	lastPaused      bool
	lastReason      string
}

func (m *mockAMIForQueue) Connect(ctx context.Context) error { return nil }
func (m *mockAMIForQueue) Close() error                      { return nil }
func (m *mockAMIForQueue) IsConnected() bool                 { return true }
func (m *mockAMIForQueue) Originate(ctx context.Context, actionID, channel, context, exten string, priority int, timeout int, callerID, account string, variables map[string]string) error {
	return nil
}
func (m *mockAMIForQueue) Redirect(ctx context.Context, actionID, channel, extraChannel, context, exten string, priority int) error {
	return nil
}
func (m *mockAMIForQueue) TransferToLiveKit(ctx context.Context, channel string, agent *domain.AgentRedisData, customer *domain.CustomerMetadata) error {
	return nil
}
func (m *mockAMIForQueue) QueueAdd(ctx context.Context, actionID, queue, iface, memberName string, penalty int, paused bool) error {
	m.lastQueueAction = "add"
	m.lastQueueName = queue
	m.lastIface = iface
	m.lastPaused = paused
	return nil
}
func (m *mockAMIForQueue) QueueRemove(ctx context.Context, actionID, queue, iface string) error {
	m.lastQueueAction = "remove"
	m.lastQueueName = queue
	m.lastIface = iface
	return nil
}
func (m *mockAMIForQueue) QueuePause(ctx context.Context, actionID, queue, iface string, paused bool, reason string) error {
	m.lastQueueAction = "pause"
	m.lastQueueName = queue
	m.lastIface = iface
	m.lastPaused = paused
	m.lastReason = reason
	return nil
}
func (m *mockAMIForQueue) Hangup(ctx context.Context, actionID, channel string, cause int) error {
	return nil
}
func (m *mockAMIForQueue) SetVar(ctx context.Context, actionID, channel, variable, value string) error {
	return nil
}
func (m *mockAMIForQueue) Command(ctx context.Context, actionID, command string) (string, error) {
	return "", nil
}
func (m *mockAMIForQueue) SubscribeEvents() <-chan ports.AMIEvent {
	ch := make(chan ports.AMIEvent)
	close(ch)
	return ch
}

type mockCacheForQueue struct{}

func (m *mockCacheForQueue) GetWhitelistIPs(ctx context.Context) ([]string, error)    { return nil, nil }
func (m *mockCacheForQueue) AddWhitelistIP(ctx context.Context, ip string) error     { return nil }
func (m *mockCacheForQueue) SetTrunkHealth(ctx context.Context, trunkID string, health domain.TrunkHealth, ttl time.Duration) error {
	return nil
}
func (m *mockCacheForQueue) GetTrunkHealth(ctx context.Context, trunkID string) (*domain.TrunkHealth, error) {
	return nil, nil
}
func (m *mockCacheForQueue) PushLeads(ctx context.Context, campaignID string, phoneList []string) error {
	return nil
}
func (m *mockCacheForQueue) PopLead(ctx context.Context, campaignID string) (string, error) {
	return "", nil
}
func (m *mockCacheForQueue) GetQueueLength(ctx context.Context, campaignID string) (int64, error) {
	return 0, nil
}
func (m *mockCacheForQueue) SetCampaignPaused(ctx context.Context, campaignID string, paused bool) error {
	return nil
}
func (m *mockCacheForQueue) IsCampaignPaused(ctx context.Context, campaignID string) (bool, error) {
	return false, nil
}
func (m *mockCacheForQueue) SetInflatedSuccessRate(ctx context.Context, ttl time.Duration) error {
	return nil
}
func (m *mockCacheForQueue) HasInflatedSuccessRate(ctx context.Context) (bool, error) {
	return false, nil
}
func (m *mockCacheForQueue) IncrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	return nil
}
func (m *mockCacheForQueue) DecrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	return nil
}
func (m *mockCacheForQueue) GetTrunkActiveChannelsCount(ctx context.Context, trunkID string) (int, error) {
	return 0, nil
}
func (m *mockCacheForQueue) GetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string) (*domain.CallsSummaryResponse, time.Duration, error) {
	return nil, 0, nil
}
func (m *mockCacheForQueue) SetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string, data *domain.CallsSummaryResponse, ttl time.Duration) error {
	return nil
}
func (m *mockCacheForQueue) StoreAvailableAgents(ctx context.Context, campaignID string, agents []domain.AgentDemandDTO, ttl time.Duration) error {
	return nil
}
func (m *mockCacheForQueue) GetNextAvailableAgent(ctx context.Context, campaignID string) (*domain.AgentDemandDTO, error) {
	return nil, nil
}
func (m *mockCacheForQueue) PopIdleAgent(ctx context.Context, timeout time.Duration) (*domain.AgentRedisData, error) {
	return nil, nil
}
func (m *mockCacheForQueue) PushIdleAgent(ctx context.Context, agent *domain.AgentRedisData) error {
	return nil
}
func (m *mockCacheForQueue) RemoveAgentFromQueues(ctx context.Context, agentID string) error {
	return nil
}
func (m *mockCacheForQueue) AcquireRoomLock(ctx context.Context, roomName string, ttl time.Duration) (bool, error) {
	return true, nil
}
func (m *mockCacheForQueue) ReleaseRoomLock(ctx context.Context, roomName string) error {
	return nil
}
func (m *mockCacheForQueue) PushAnsweredLead(ctx context.Context, event *domain.AnsweredLeadEvent) error {
	return nil
}
func (m *mockCacheForQueue) ResetConsecutiveErrors(ctx context.Context, campaignID string) error {
	return nil
}
func (m *mockCacheForQueue) IncrementConsecutiveErrors(ctx context.Context, campaignID string) (int64, error) {
	return 0, nil
}
func (m *mockCacheForQueue) GetConsecutiveErrors(ctx context.Context, campaignID string) (int64, error) {
	return 0, nil
}

func TestQueueHandler_SetPresence(t *testing.T) {
	ami := &mockAMIForQueue{}
	cache := &mockCacheForQueue{}
	queueMgr := core.NewAgentQueueManager(ami, cache)
	handler := NewQueueHandler(queueMgr)

	// Cenário 1: Tabulação (Pausa com motivo)
	payloadPause := domain.QueuePresenceEvent{
		Action:      "pause",
		TenantID:    "default",
		CampaignID:  "12",
		AgentID:     "emerson",
		LiveKitRoom: "room_emerson",
		Reason:      "ACW_Tabulacao",
	}
	body, _ := json.Marshal(payloadPause)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/queues/presence", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.SetPresence(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 OK, obteve %d", rec.Code)
	}
	if !ami.lastPaused || ami.lastReason != "ACW_Tabulacao" {
		t.Errorf("esperava paused=true e motivo ACW_Tabulacao, obteve %v / %s", ami.lastPaused, ami.lastReason)
	}

	// Cenário 2: Retorno para Disponível (Unpause / Ready)
	payloadReady := domain.QueuePresenceEvent{
		Action:      "ready",
		TenantID:    "default",
		CampaignID:  "12",
		AgentID:     "emerson",
		LiveKitRoom: "room_emerson",
	}
	body, _ = json.Marshal(payloadReady)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/queues/presence", bytes.NewReader(body))
	rec = httptest.NewRecorder()

	handler.SetPresence(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 OK, obteve %d", rec.Code)
	}
	if ami.lastPaused {
		t.Errorf("esperava paused=false ao enviar action 'ready'")
	}
}
