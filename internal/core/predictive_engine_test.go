package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

// mockCachePredictive implementa ports.CachePort para testes preditivos
type mockCachePredictive struct {
	queue []string
}

func (m *mockCachePredictive) GetWhitelistIPs(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockCachePredictive) AddWhitelistIP(ctx context.Context, ip string) error   { return nil }
func (m *mockCachePredictive) SetTrunkHealth(ctx context.Context, trunkID string, health domain.TrunkHealth, ttl time.Duration) error {
	return nil
}
func (m *mockCachePredictive) GetTrunkHealth(ctx context.Context, trunkID string) (*domain.TrunkHealth, error) {
	return &domain.TrunkHealth{Status: "ONLINE", MaxChannels: 50}, nil
}
func (m *mockCachePredictive) PushLeads(ctx context.Context, campaignID string, phoneList []string) error {
	m.queue = append(m.queue, phoneList...)
	return nil
}
func (m *mockCachePredictive) PopLead(ctx context.Context, campaignID string) (string, error) {
	if len(m.queue) == 0 {
		return "", nil
	}
	lead := m.queue[0]
	m.queue = m.queue[1:]
	return lead, nil
}
func (m *mockCachePredictive) GetQueueLength(ctx context.Context, campaignID string) (int64, error) {
	return int64(len(m.queue)), nil
}
func (m *mockCachePredictive) SetCampaignPaused(ctx context.Context, campaignID string, paused bool) error {
	return nil
}
func (m *mockCachePredictive) IsCampaignPaused(ctx context.Context, campaignID string) (bool, error) {
	return false, nil
}
func (m *mockCachePredictive) SetInflatedSuccessRate(ctx context.Context, ttl time.Duration) error {
	return nil
}
func (m *mockCachePredictive) HasInflatedSuccessRate(ctx context.Context) (bool, error) { return false, nil }
func (m *mockCachePredictive) IncrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	return nil
}
func (m *mockCachePredictive) DecrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	return nil
}
func (m *mockCachePredictive) GetTrunkActiveChannelsCount(ctx context.Context, trunkID string) (int, error) {
	return 0, nil
}
func (m *mockCachePredictive) GetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string) (*domain.CallsSummaryResponse, time.Duration, error) {
	return nil, 0, nil
}
func (m *mockCachePredictive) SetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string, data *domain.CallsSummaryResponse, ttl time.Duration) error {
	return nil
}
func (m *mockCachePredictive) StoreAvailableAgents(ctx context.Context, campaignID string, agents []domain.AgentDemandDTO, ttl time.Duration) error {
	return nil
}
func (m *mockCachePredictive) GetNextAvailableAgent(ctx context.Context, campaignID string) (*domain.AgentDemandDTO, error) {
	return &domain.AgentDemandDTO{AgentID: "agent-1", SIPRoute: "sala_agente_1"}, nil
}

func TestPredictiveEngine_RandomizeCallerID_ZeroNormalization(t *testing.T) {
	pe := &PredictiveEngine{}

	testCases := []struct {
		name         string
		destPhone    string
		expectLen    int
		expectPrefix string
	}{
		{
			name:         "10 dígitos fixo local (N10)",
			destPhone:    "2133445566",
			expectLen:    10,
			expectPrefix: "213344", // preserva os 6 primeiros dígitos
		},
		{
			name:         "11 dígitos celular (N11)",
			destPhone:    "21999914324",
			expectLen:    11,
			expectPrefix: "2199991", // preserva os 7 primeiros dígitos
		},
		{
			name:         "12 dígitos com zero DDD",
			destPhone:    "021999914324",
			expectLen:    12,
			expectPrefix: "02199991", // preserva com zero
		},
		{
			name:         "13 dígitos internacional DDI 55",
			destPhone:    "5521999914324",
			expectLen:    13,
			expectPrefix: "552199991", // preserva com 55
		},
		{
			name:         "14 dígitos com CSP operadora",
			destPhone:    "01521999914324",
			expectLen:    14,
			expectPrefix: "0152199991", // preserva CSP
		},
		{
			name:         "Menos de 10 dígitos (retorna inalterado)",
			destPhone:    "12345678",
			expectLen:    8,
			expectPrefix: "12345678",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := pe.randomizeCallerID(tc.destPhone)
			if len(result) != tc.expectLen {
				t.Errorf("tamanho incorreto: esperado %d, obtido %d (%s)", tc.expectLen, len(result), result)
			}
			if !strings.HasPrefix(result, tc.expectPrefix) {
				t.Errorf("prefixo alterado incorretamente: esperado começar com '%s', obtido '%s'", tc.expectPrefix, result)
			}
		})
	}
}

func TestPredictiveEngine_ProcessDemand_ZeroNormalization(t *testing.T) {
	ctx := context.Background()
	ami := &mockAMI{}
	cm := NewChannelManager(60, 10, nil)
	cm.RegisterTrunkLimit("trunk-vivo", 50)

	trunk := &domain.Trunk{
		ID:               "trunk-vivo",
		TenantID:         "tenant-test",
		Name:             "Vivo SIP",
		Direction:        domain.DirectionOutbound,
		RegistrationMode: domain.RegistrationModeIPBased,
		Host:             "metapabx.vivo.net.br",
		Port:             5060,
		MaxChannels:      50,
		IsEnabled:        true,
	}
	trunks := &mockTrunkRepo{trunk: trunk}

	campaign := &domain.Campaign{
		ID:            "camp-1",
		TenantID:      "tenant-test",
		Mode:          domain.CampaignModePredictive,
		Status:        domain.CampaignStatusActive,
		Aggressiveness: 1.0,
		TrunkName:     "trunk-vivo",
	}
	campaigns := &mockCampaignRepo{camp: campaign}
	leads := &mockLeadRepo{total: 100, available: 50, dialed: 50}

	// Enfileira leads com 13 dígitos
	lead13, _ := json.Marshal(domain.LeadQueueItem{
		Phone:  "5521999914324",
		Name:   "Maria DDI",
		CPF:    "11122233344",
		LeadID: 101,
	})
	cache := &mockCachePredictive{
		queue: []string{string(lead13)},
	}

	engine := NewPredictiveEngine(ami, cm, cache, campaigns, trunks, leads)

	req := &domain.PredictiveDemandRequest{
		TenantID:   "tenant-test",
		CampaignID: "camp-1",
		AvailableAgents: []domain.AgentDemandDTO{
			{AgentID: "agent-1", SIPRoute: "sala_agente_1"},
		},
	}

	resp, err := engine.ProcessDemand(ctx, req)
	if err != nil {
		t.Fatalf("ProcessDemand falhou: %v", err)
	}

	if resp.DialingChannels < 1 {
		t.Fatalf("esperava ao menos 1 canal disparado, obteve %d", resp.DialingChannels)
	}

	// Verifica se o canal gerado preserva rigorosamente o número com 55 (sem corte forçado)
	expectedPhone := "5521999914324"
	expectedDialPrefix := fmt.Sprintf("PJSIP/trunk-vivo/sip:%s@metapabx.vivo.net.br", expectedPhone)
	if ami.lastChannel != expectedDialPrefix {
		t.Errorf("lastChannel incorreto: esperado '%s', obtido '%s'", expectedDialPrefix, ami.lastChannel)
	}

	if ami.lastVars["PHONE"] != expectedPhone {
		t.Errorf("PHONE var incorreta: esperado '%s', obtido '%s'", expectedPhone, ami.lastVars["PHONE"])
	}
	if ami.lastVars["__PHONE"] != expectedPhone {
		t.Errorf("__PHONE var incorreta: esperado '%s', obtido '%s'", expectedPhone, ami.lastVars["__PHONE"])
	}
}
