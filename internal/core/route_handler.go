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

	// 2. Busca o operador livre na fila `dialer:idle_agents` garantindo trava distribuída SETNX (Race Condition Prevention)
	var agentData *domain.AgentRedisData
	var roomName string
	var userID string

	// Tenta até 3 vezes obter um operador ocioso cuja trava de sala seja adquirida com sucesso
	for attempt := 0; attempt < 3; attempt++ {
		agent, err := rh.cache.PopIdleAgent(ctx, 50*time.Millisecond)
		if err == nil && agent != nil && agent.AgentID != "" {
			targetRoom := agent.LiveKitRoom
			if targetRoom == "" {
				targetRoom = fmt.Sprintf("sala_agente_%s", agent.AgentID)
			}

			// Tenta adquirir a trava distribuída SETNX lock:room:<room_name> 1 EX 10
			acquired, lockErr := rh.cache.AcquireRoomLock(ctx, targetRoom, 10*time.Second)
			if lockErr == nil && acquired {
				agentData = agent
				userID = agent.AgentID
				roomName = targetRoom
				if callID != "" {
					rh.channels.AssignAgent(callID, agent.AgentID)
				}
				log.Printf("[RACE-PREVENTION] Trava de sala '%s' adquirida com sucesso para o agente %s", targetRoom, agent.AgentID)
				break
			}
			log.Printf("[RACE-PREVENTION] Concorrência detectada! Trava para sala '%s' pertence a outra chamada. Buscando próximo operador...", targetRoom)
			continue
		}

		// Fallback: tenta recuperar operador da fila volátil por campanha
		nextAgent, err := rh.cache.GetNextAvailableAgent(ctx, campaignID)
		if err == nil && nextAgent != nil && nextAgent.AgentID != "" {
			targetRoom := fmt.Sprintf("sala_agente_%s", nextAgent.AgentID)
			acquired, lockErr := rh.cache.AcquireRoomLock(ctx, targetRoom, 10*time.Second)
			if lockErr == nil && acquired {
				userID = nextAgent.AgentID
				roomName = targetRoom
				agentData = &domain.AgentRedisData{
					AgentID:     nextAgent.AgentID,
					LiveKitRoom: targetRoom,
				}
				if callID != "" {
					rh.channels.AssignAgent(callID, nextAgent.AgentID)
				}
				break
			}
		}
	}

	// Se nenhum operador tiver trava concedida, utiliza sala de transbordo da campanha
	if agentData == nil {
		roomName = fmt.Sprintf("sala_campanha_%s", campaignID)
		userID = roomName
		agentData = &domain.AgentRedisData{
			AgentID:     userID,
			LiveKitRoom: roomName,
		}
		log.Printf("[ROUTE-HANDLER] Nenhum operador livre com trava disponível. Transferindo para sala fallback de campanha '%s'", roomName)
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
