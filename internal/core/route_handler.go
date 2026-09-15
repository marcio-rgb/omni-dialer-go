package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// RouteHandler orquestra o roteamento de alta performance para chamadas atendidas por humano.
//
// @pattern Strategy (Route Handler)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#1-algoritmo-de-pacing-e-equações-de-overdialing
//
// @preExecution
// - Recepção do evento AMD Human em tempo real (< 300ms SLA)
// - Extração de metadados do cliente a partir do canal ativo
// - Retirada atômica do operador da fila `dialer:idle_agents` no Redis
//
// @postExecution
// - Injeção de variáveis de canal no Asterisk (`AGENT_ROOM`, `CUSTOMER_*`)
// - Redirecionamento AMI para o tronco LiveKit SIP (`cos-all-custom` / `9999`)
// - Notificação assíncrona para o webhook do sistema consumidor
type RouteHandler struct {
	ami           ports.AMIPort
	cache         ports.CachePort
	channels      *ChannelManager
	webhookClient ports.WebhookPort
	tenants       ports.TenantRepository
}

func NewRouteHandler(ami ports.AMIPort, cache ports.CachePort, channels *ChannelManager, webhookClient ports.WebhookPort, tenants ports.TenantRepository) *RouteHandler {
	return &RouteHandler{
		ami:           ami,
		cache:         cache,
		channels:      channels,
		webhookClient: webhookClient,
		tenants:       tenants,
	}
}

// HandleHumanDetected processa o evento de atendimento humano e transfere a chamada para o operador no LiveKit.
func (rh *RouteHandler) HandleHumanDetected(ctx context.Context, channel, uniqueID, phone, campaignID, leadID string) error {
	callID := rh.channels.GetCallIDByAsterisk(channel, uniqueID)

	// 1. Constrói os metadados do cliente a partir do canal ativo
	var customer *domain.CustomerMetadata
	if callID != "" {
		activeChan := rh.channels.GetActiveChannel(callID)
		if activeChan != nil {
			customer = &domain.CustomerMetadata{
				CustomerID: activeChan.CPF,
				Name:       activeChan.Name,
				Phone:      activeChan.Phone,
				Att1:       activeChan.Att1,
				Att2:       activeChan.Att2,
				Att3:       activeChan.Att3,
			}
			if customer.CustomerID == "" {
				customer.CustomerID = leadID
			}
		}
	}
	if customer == nil {
		customer = &domain.CustomerMetadata{
			CustomerID: leadID,
			Phone:      phone,
		}
	}

	// 2. Busca o operador livre na fila `dialer:idle_agents` (com timeout curto de 50ms)
	agent, err := rh.cache.PopIdleAgent(ctx, 50*time.Millisecond)
	roomName := fmt.Sprintf("sala_campanha_%s", campaignID)
	userID := roomName

	var agentData *domain.AgentRedisData
	if err == nil && agent != nil && agent.AgentID != "" {
		agentData = agent
		userID = agent.AgentID
		roomName = agent.LiveKitRoom
		if roomName == "" {
			roomName = fmt.Sprintf("sala_agente_%s", agent.AgentID)
		}
		if callID != "" {
			rh.channels.AssignAgent(callID, agent.AgentID)
		}
	} else {
		// Fallback: tenta recuperar operador da fila volátil por campanha
		nextAgent, err := rh.cache.GetNextAvailableAgent(ctx, campaignID)
		if err == nil && nextAgent != nil && nextAgent.AgentID != "" {
			userID = nextAgent.AgentID
			roomName = fmt.Sprintf("sala_agente_%s", nextAgent.AgentID)
			agentData = &domain.AgentRedisData{
				AgentID:     nextAgent.AgentID,
				LiveKitRoom: roomName,
			}
			if callID != "" {
				rh.channels.AssignAgent(callID, nextAgent.AgentID)
			}
		} else {
			log.Printf("[ROUTE-HANDLER] Nenhum operador ocioso disponível no momento. Utilizando sala fallback '%s'", roomName)
			agentData = &domain.AgentRedisData{
				AgentID:     userID,
				LiveKitRoom: roomName,
			}
		}
	}

	// 3. Notificação HTTP em background para o webhook do sistema (se configurado)
	if rh.webhookClient != nil {
		go rh.dispatchInjectLead(callID, userID, phone, campaignID, customer)
	}

	// 4. Efetua a transferência no Asterisk para o LiveKit SIP
	return rh.ami.TransferToLiveKit(ctx, channel, agentData, customer)
}

func (rh *RouteHandler) dispatchInjectLead(callID, userID, phone, campaignID string, customer *domain.CustomerMetadata) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tenantID := "default"
	if callID != "" {
		activeChan := rh.channels.GetActiveChannel(callID)
		if activeChan != nil && activeChan.TenantID != "" {
			tenantID = activeChan.TenantID
		}
	}

	var webhookURL string
	if rh.tenants != nil {
		tenant, err := rh.tenants.GetByID(ctx, tenantID)
		if err == nil && tenant != nil {
			webhookURL = tenant.Webhook
		}
	}

	params := &domain.InjectLeadParams{
		UserID: userID,
		CPF:    customer.CustomerID,
		Name:   customer.Name,
		Phone:  customer.Phone,
		Att1:   customer.Att1,
		Att2:   customer.Att2,
		Att3:   customer.Att3,
	}

	if rh.webhookClient != nil {
		_ = rh.webhookClient.NotifyInjectLead(ctx, webhookURL, params)
	}
}
