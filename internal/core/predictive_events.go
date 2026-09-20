package core

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"dialer-go/internal/domain"
)

// HandlePredictiveHuman é acionado quando o Asterisk detecta humano no triagem-amd (UserEvent PredictiveHuman).
//
// @pattern Observer / Event Handler
// @governedBy docs/rules/TELEPHONY_POLICIES.md#1-algoritmo-de-pacing-e-equações-de-overdialing
//
// @preExecution
// - Extração de metadados do cliente a partir do canal ativo
// - Retirada do operador da fila `dialer:idle_agents` com trava distribuída
//
// @postExecution
// - Enfileiramento do lead atendido com dados dinâmicos na fila Redis `dialer:answered_leads`
// - Notificação HTTP assíncrona para o webhook do sistema (inject-lead)
// - Transferência SIP para o LiveKit do operador
func (pe *PredictiveEngine) HandlePredictiveHuman(ctx context.Context, channel, uniqueID, phone, campaignID, leadID string) error {
	callID := pe.channels.GetCallIDByAsterisk(channel, uniqueID)

	// 1. Extrai metadados do cliente a partir do canal ativo
	var customer *domain.CustomerMetadata
	tenantID := "default"
	var activeChanRef *domain.ActiveChannel

	if callID != "" {
		if activeChan := pe.channels.GetActiveChannel(callID); activeChan != nil {
			activeChanRef = activeChan
			if activeChan.TenantID != "" {
				tenantID = activeChan.TenantID
			}
			customer = &domain.CustomerMetadata{
				CustomerID: activeChan.CPF,
				Name:       activeChan.Name,
				Phone:      activeChan.Phone,
				Att1:       activeChan.Att1,
				Att2:       activeChan.Att2,
				Att3:       activeChan.Att3,
				Custom:     activeChan.Custom,
			}
			if customer.CustomerID == "" {
				customer.CustomerID = leadID
			}
		}
	}
	if customer == nil {
		customer = &domain.CustomerMetadata{CustomerID: leadID, Phone: phone}
	}

	// 2. Busca o próximo operador disponível na fila Redis `dialer:idle_agents` com trava distribuída (Race Condition Prevention)
	var agentData *domain.AgentRedisData
	var userID string

	for attempt := 0; attempt < 3; attempt++ {
		agentRedis, err := pe.cache.PopIdleAgent(ctx, 50*time.Millisecond)
		if err == nil && agentRedis != nil && agentRedis.AgentID != "" {
			targetRoom := agentRedis.LiveKitRoom
			if targetRoom == "" {
				targetRoom = fmt.Sprintf("sala_agente_%s", agentRedis.AgentID)
			}
			acquired, lockErr := pe.cache.AcquireRoomLock(ctx, targetRoom, 10*time.Second)
			if lockErr == nil && acquired {
				agentData = agentRedis
				userID = agentRedis.AgentID
				if callID != "" {
					pe.channels.AssignAgent(callID, agentRedis.AgentID)
				}
				log.Printf("[RACE-PREVENTION] Trava preditiva de sala '%s' adquirida para o agente %s", targetRoom, agentRedis.AgentID)
				break
			}
			log.Printf("[RACE-PREVENTION] Concorrência detectada! Trava para sala '%s' ocupada. Buscando próximo operador...", targetRoom)
			continue
		}

		agent, err := pe.cache.GetNextAvailableAgent(ctx, campaignID)
		if err == nil && agent != nil && agent.AgentID != "" {
			targetRoom := fmt.Sprintf("sala_agente_%s", agent.AgentID)
			acquired, lockErr := pe.cache.AcquireRoomLock(ctx, targetRoom, 10*time.Second)
			if lockErr == nil && acquired {
				userID = agent.AgentID
				agentData = &domain.AgentRedisData{AgentID: agent.AgentID, LiveKitRoom: targetRoom}
				if callID != "" {
					pe.channels.AssignAgent(callID, agent.AgentID)
				}
				break
			}
		}
	}

	if agentData == nil {
		log.Printf("[PREDICTIVE-HUMAN] Nenhum operador ocioso com trava livre na fila Redis. Desligando chamada (Cause 17) para evitar ligação muda no cliente %s", phone)
		if callID != "" {
			pe.channels.SetCallDisposition(callID, domain.DispositionAbandoned)
		}
		_ = pe.cache.SetInflatedSuccessRate(ctx, 30*time.Second)
		actionID := fmt.Sprintf("pred-no-agent-%d", time.Now().UnixNano())
		return pe.ami.Hangup(ctx, actionID, channel, 17)
	}

	// 3. Enfileira o lead atendido na fila Redis `dialer:answered_leads` com todos os campos dinâmicos
	var leadIDInt int64
	if leadID != "" {
		leadIDInt, _ = strconv.ParseInt(leadID, 10, 64)
	}
	var customMap map[string]string
	var cpfVal, nameVal string
	if activeChanRef != nil {
		customMap = activeChanRef.Custom
		cpfVal = activeChanRef.CPF
		nameVal = activeChanRef.Name
	}
	if customer != nil && len(customer.Custom) > 0 {
		customMap = customer.Custom
	}
	if cpfVal == "" && customer != nil {
		cpfVal = customer.CustomerID
	}
	if nameVal == "" && customer != nil {
		nameVal = customer.Name
	}

	answeredEvent := &domain.AnsweredLeadEvent{
		Event:      "call.answered",
		CallID:     callID,
		Channel:    channel,
		TenantID:   tenantID,
		CampaignID: campaignID,
		LeadID:     leadIDInt,
		Phone:      phone,
		CPF:        cpfVal,
		Name:       nameVal,
		AgentID:    userID,
		RoomName:   agentData.LiveKitRoom,
		Custom:     customMap,
		AnsweredAt: time.Now(),
		Timestamp:  time.Now().Unix(),
	}
	if err := pe.cache.PushAnsweredLead(ctx, answeredEvent); err != nil {
		log.Printf("[WARN] Falha ao enfileirar answered lead no Redis: %v", err)
	}

	// 4. Disparo assíncrono do webhook inject-lead com resiliência (sem travar a telefonia)
	if pe.webhookClient != nil {
		go pe.dispatchInjectLeadWebhook(callID, userID, phone, campaignID)
	}

	// 5. Executa a transferência no Asterisk para o LiveKit SIP com suporte aos cabeçalhos X-Customer-*
	return pe.ami.TransferToLiveKit(ctx, channel, agentData, customer)
}

func (pe *PredictiveEngine) dispatchInjectLeadWebhook(callID, userID, phone, campaignID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tenantID := "default"
	var cpf, name, att1, att2, att3 string
	if callID != "" {
		if activeChan := pe.channels.GetActiveChannel(callID); activeChan != nil {
			if activeChan.TenantID != "" {
				tenantID = activeChan.TenantID
			}
			cpf, name = activeChan.CPF, activeChan.Name
			att1, att2, att3 = activeChan.Att1, activeChan.Att2, activeChan.Att3
		}
	}

	var webhookURL string
	if pe.tenants != nil {
		if tenant, err := pe.tenants.GetByID(ctx, tenantID); err == nil && tenant != nil {
			webhookURL = tenant.Webhook
		}
	}

	params := &domain.InjectLeadParams{
		UserID: userID,
		CPF:    cpf,
		Name:   name,
		Phone:  phone,
		Att1:   att1,
		Att2:   att2,
		Att3:   att3,
	}

	if pe.webhookClient != nil {
		for attempt := 1; attempt <= 2; attempt++ {
			if err := pe.webhookClient.NotifyInjectLead(ctx, webhookURL, params); err == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// HandlePredictiveAi é acionado quando o Asterisk detecta atendimento de IA (UserEvent PredictiveAi)
func (pe *PredictiveEngine) HandlePredictiveAi(ctx context.Context, channel, uniqueID, phone, campaignID, leadID string) error {
	actionID := fmt.Sprintf("pred-ai-%d", time.Now().UnixNano())
	callID := pe.channels.GetCallIDByAsterisk(channel, uniqueID)
	if callID != "" {
		pe.channels.SetCallDisposition(callID, domain.DispositionAMDMachine)
	}

	cleanPhone := strings.ReplaceAll(strings.ReplaceAll(phone, "+", ""), " ", "")
	roomName := fmt.Sprintf("sala_agente_%s_%s", campaignID, cleanPhone)

	_ = pe.ami.SetVar(ctx, fmt.Sprintf("setvar-%s", actionID), channel, "AGENT_ROOM", roomName)

	return pe.ami.Redirect(ctx, actionID, channel, "", "cos-all-custom", "9999", 1)
}
