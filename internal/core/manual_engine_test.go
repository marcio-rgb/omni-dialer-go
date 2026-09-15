package core

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// mockAMI implementa ports.AMIPort capturando chamadas Originate para asserção
type mockAMI struct {
	lastActionID string
	lastChannel  string
	lastContext  string
	lastExten    string
	lastCallerID string
	lastVars     map[string]string
}

func (m *mockAMI) Connect(ctx context.Context) error { return nil }
func (m *mockAMI) Close() error                      { return nil }
func (m *mockAMI) IsConnected() bool                 { return true }
func (m *mockAMI) Originate(ctx context.Context, actionID, channel, context, exten string, priority int, timeout int, callerID, account string, variables map[string]string) error {
	m.lastActionID = actionID
	m.lastChannel = channel
	m.lastContext = context
	m.lastExten = exten
	m.lastCallerID = callerID
	m.lastVars = variables
	return nil
}
func (m *mockAMI) Redirect(ctx context.Context, actionID, channel, extraChannel, context, exten string, priority int) error {
	return nil
}
func (m *mockAMI) Hangup(ctx context.Context, actionID, channel string, cause int) error {
	return nil
}
func (m *mockAMI) SetVar(ctx context.Context, actionID, channel, variable, value string) error {
	return nil
}
func (m *mockAMI) Command(ctx context.Context, actionID, command string) (string, error) {
	return "", nil
}
func (m *mockAMI) SubscribeEvents() <-chan ports.AMIEvent {
	ch := make(chan ports.AMIEvent)
	close(ch)
	return ch
}

// mockTrunkRepo implementa ports.TrunkRepository retornando tronco ativo configurado
type mockTrunkRepo struct {
	trunk *domain.Trunk
}

func (m *mockTrunkRepo) Create(ctx context.Context, trunk *domain.Trunk) error { return nil }
func (m *mockTrunkRepo) GetByID(ctx context.Context, tenantID, trunkID string) (*domain.Trunk, error) {
	return m.trunk, nil
}
func (m *mockTrunkRepo) ListByTenant(ctx context.Context, tenantID string) ([]*domain.Trunk, error) {
	return []*domain.Trunk{m.trunk}, nil
}
func (m *mockTrunkRepo) Update(ctx context.Context, trunk *domain.Trunk) error       { return nil }
func (m *mockTrunkRepo) Delete(ctx context.Context, tenantID, trunkID string) error { return nil }
func (m *mockTrunkRepo) ListAllEnabled(ctx context.Context) ([]*domain.Trunk, error) {
	return []*domain.Trunk{m.trunk}, nil
}

// mockCache implementa ports.CachePort com retorno neutro para testes de discagem manual
type mockCache struct{}

func (m *mockCache) GetWhitelistIPs(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockCache) AddWhitelistIP(ctx context.Context, ip string) error   { return nil }
func (m *mockCache) SetTrunkHealth(ctx context.Context, trunkID string, health domain.TrunkHealth, ttl time.Duration) error {
	return nil
}
func (m *mockCache) GetTrunkHealth(ctx context.Context, trunkID string) (*domain.TrunkHealth, error) {
	return &domain.TrunkHealth{Status: "ONLINE", MaxChannels: 30}, nil
}
func (m *mockCache) PushLeads(ctx context.Context, campaignID string, phoneList []string) error {
	return nil
}
func (m *mockCache) PopLead(ctx context.Context, campaignID string) (string, error) { return "", nil }
func (m *mockCache) GetQueueLength(ctx context.Context, campaignID string) (int64, error) {
	return 0, nil
}
func (m *mockCache) SetCampaignPaused(ctx context.Context, campaignID string, paused bool) error {
	return nil
}
func (m *mockCache) IsCampaignPaused(ctx context.Context, campaignID string) (bool, error) {
	return false, nil
}
func (m *mockCache) SetInflatedSuccessRate(ctx context.Context, ttl time.Duration) error {
	return nil
}
func (m *mockCache) HasInflatedSuccessRate(ctx context.Context) (bool, error) { return false, nil }
func (m *mockCache) IncrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	return nil
}
func (m *mockCache) DecrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	return nil
}
func (m *mockCache) GetTrunkActiveChannelsCount(ctx context.Context, trunkID string) (int, error) {
	return 0, nil
}
func (m *mockCache) GetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string) (*domain.CallsSummaryResponse, time.Duration, error) {
	return nil, 0, nil
}
func (m *mockCache) SetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string, data *domain.CallsSummaryResponse, ttl time.Duration) error {
	return nil
}
func (m *mockCache) StoreAvailableAgents(ctx context.Context, campaignID string, agents []domain.AgentDemandDTO, ttl time.Duration) error {
	return nil
}
func (m *mockCache) GetNextAvailableAgent(ctx context.Context, campaignID string) (*domain.AgentDemandDTO, error) {
	return nil, nil
}

func TestManualEngine_ZeroNormalization_PhoneIntegrity(t *testing.T) {
	testCases := []struct {
		name          string
		inputPhone    string
		expectedPhone string
	}{
		{
			name:          "10 dígitos fixo local (N10 sem 55)",
			inputPhone:    "2133445566",
			expectedPhone: "2133445566",
		},
		{
			name:          "11 dígitos celular (N11 sem 55)",
			inputPhone:    "21999914324",
			expectedPhone: "21999914324",
		},
		{
			name:          "12 dígitos com zero DDD",
			inputPhone:    "021999914324",
			expectedPhone: "021999914324",
		},
		{
			name:          "13 dígitos internacional DDI 55",
			inputPhone:    "5521999914324",
			expectedPhone: "5521999914324",
		},
		{
			name:          "14 dígitos com CSP operadora (015)",
			inputPhone:    "01521999914324",
			expectedPhone: "01521999914324",
		},
		{
			name:          "Formatado amigável com parênteses e traço (sanitização de caracteres sem normalização)",
			inputPhone:    "(21) 99991-4324",
			expectedPhone: "21999914324",
		},
		{
			name:          "Formatado internacional com sinal de mais",
			inputPhone:    "+55 (21) 99991-4324",
			expectedPhone: "5521999914324",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ami := &mockAMI{}
			cm := NewChannelManager(60, 10, nil)
			cm.RegisterTrunkLimit("trunk-vivo", 30)

			fromUser := "1127011340"
			trunk := &domain.Trunk{
				ID:               "trunk-vivo",
				TenantID:         "tenant-test",
				Name:             "Vivo SIP",
				Direction:        domain.DirectionOutbound,
				RegistrationMode: domain.RegistrationModeIPBased,
				Host:             "metapabx.vivo.net.br",
				Port:             5060,
				FromUser:         &fromUser,
				MaxChannels:      30,
				IsEnabled:        true,
			}
			trunks := &mockTrunkRepo{trunk: trunk}
			cache := &mockCache{}

			engine := NewManualEngine(ami, cm, trunks, cache)

			req := &domain.ManualCallRequest{
				TenantID: "tenant-test",
				AgentID:  "agent-1",
				Phone:    tc.inputPhone,
				SIPRoute: "sala_agente_1",
				TrunkID:  "trunk-vivo",
				LeadName: "João Silva",
				LeadCPF:  "12345678900",
			}

			resp, err := engine.DialManual(ctx, req)
			if err != nil {
				t.Fatalf("DialManual falhou: %v", err)
			}

			if resp.Status != "dialing" {
				t.Errorf("esperava status 'dialing', obteve '%s'", resp.Status)
			}

			// Validação 1: O canal Asterisk gerado deve conter o telefone sem normalização
			expectedDialPrefix := "PJSIP/trunk-vivo/sip:" + tc.expectedPhone + "@metapabx.vivo.net.br"
			if ami.lastChannel != expectedDialPrefix {
				t.Errorf("lastChannel incorreto: esperado '%s', obtido '%s'", expectedDialPrefix, ami.lastChannel)
			}

			// Validação 2: A variável PHONE do canal deve preservar a numeração exata
			if ami.lastVars["PHONE"] != tc.expectedPhone {
				t.Errorf("PHONE var incorreta: esperado '%s', obtido '%s'", tc.expectedPhone, ami.lastVars["PHONE"])
			}
			if ami.lastVars["__PHONE"] != tc.expectedPhone {
				t.Errorf("__PHONE var incorreta: esperado '%s', obtido '%s'", tc.expectedPhone, ami.lastVars["__PHONE"])
			}

			// Validação 3: CallerID para LiveKit leg e identificação
			if ami.lastVars["LEAD_NAME"] != "João Silva" {
				t.Errorf("LEAD_NAME var incorreta: obtido '%s'", ami.lastVars["LEAD_NAME"])
			}
		})
	}
}
