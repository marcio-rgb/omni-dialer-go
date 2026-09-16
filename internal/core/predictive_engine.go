package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/google/uuid"
)

type PredictiveEngine struct {
	ami                 ports.AMIPort
	channels            *ChannelManager
	cache               ports.CachePort
	campaigns           ports.CampaignRepository
	trunks              ports.TrunkRepository
	leads               ports.LeadRepository
	tenants             ports.TenantRepository
	webhookClient       ports.WebhookPort
	roundRobinIdx       uint64
	minChannelsPerAgent atomic.Int32
}

func NewPredictiveEngine(ami ports.AMIPort, channels *ChannelManager, cache ports.CachePort, campaigns ports.CampaignRepository, trunks ports.TrunkRepository, leads ports.LeadRepository) *PredictiveEngine {
	pe := &PredictiveEngine{
		ami:       ami,
		channels:  channels,
		cache:     cache,
		campaigns: campaigns,
		trunks:    trunks,
		leads:     leads,
	}
	pe.minChannelsPerAgent.Store(2)
	return pe
}

func (pe *PredictiveEngine) SetTenantRepository(tenants ports.TenantRepository) {
	pe.tenants = tenants
}

func (pe *PredictiveEngine) SetWebhookClient(webhookClient ports.WebhookPort) {
	pe.webhookClient = webhookClient
}

// SetMinChannelsPerAgent configura o piso de canais simultâneos originados por usuário disponível de forma thread-safe.
func (pe *PredictiveEngine) SetMinChannelsPerAgent(minChannels int) {
	if minChannels > 0 {
		pe.minChannelsPerAgent.Store(int32(minChannels))
	}
}

// GetMinChannelsPerAgent retorna a taxa configurada de canais simultâneos por operador disponível.
func (pe *PredictiveEngine) GetMinChannelsPerAgent() int {
	val := int(pe.minChannelsPerAgent.Load())
	if val <= 0 {
		return 2
	}
	return val
}

// ProcessDemand processa a requisição síncrona POST /api/v1/predictive/demand e calcula os disparos.
//
// @pattern Strategy (Predictive Engine)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#1-algoritmo-de-pacing-e-equações-de-overdialing
//
// @preExecution
// - Checagem de pausa da campanha em cache Redis (`IsCampaignPaused`)
// - Filtragem do pool de troncos PJSIP elegíveis e saudáveis
// - Cálculo de overdialing dinâmico baseado em agentes livres e agressividade
// - Aplicação do piso mínimo de discagem por usuário disponível (mínimo de 7:1)
//
// @postExecution
// - Alocação atômica de slots no `ChannelManager` respeitando `HumanReserveQuota`
// - Consumo de leads da fila/banco via `leadRepo.PopLead`
// - Disparo de requisições `Originate` via socket AMI do Asterisk
func (pe *PredictiveEngine) ProcessDemand(ctx context.Context, req *domain.PredictiveDemandRequest) (*domain.PredictiveDemandResponse, error) {
	// 1. Verifica se a campanha está pausada via flag rápida Redis
	paused, _ := pe.cache.IsCampaignPaused(ctx, req.CampaignID)
	if paused {
		return &domain.PredictiveDemandResponse{
			CampaignID:      req.CampaignID,
			DialingChannels: 0,
			Status:          "paused",
		}, nil
	}

	// 2. Identifica campanha e parâmetros
	var aggressiveness float64 = 1.2
	var specificTrunkName string
	campaign, err := pe.campaigns.GetByID(ctx, req.TenantID, req.CampaignID)
	if err == nil && campaign != nil {
		if campaign.Aggressiveness > 0 {
			aggressiveness = campaign.Aggressiveness
		}
		specificTrunkName = campaign.TrunkName
	}

	// 3. Monta o Pool de Troncos de Saída habilitados (ignora gateways internos como LiveKit e rotas Inbound)
	allTrunks, err := pe.trunks.ListByTenant(ctx, req.TenantID)
	if err != nil || len(allTrunks) == 0 {
		return nil, domain.NewErrBadRequest("NO_AVAILABLE_TRUNKS", "Nenhum tronco habilitado encontrado para o tenant")
	}

	var availablePool []*domain.Trunk
	for _, t := range allTrunks {
		if t.IsEnabled && t.ID != "livekit" && t.ID != "livekit-sip" && t.Direction != domain.DirectionInbound {
			if specificTrunkName != "" && specificTrunkName != "auto" && specificTrunkName != "default" && t.ID != specificTrunkName && t.Name != specificTrunkName {
				continue
			}
			pe.channels.RegisterTrunkLimit(t.ID, t.MaxChannels)
			availablePool = append(availablePool, t)
		}
	}

	// Prioriza troncos com status saudável (ONLINE ou REGISTERED) na telemetria
	var healthyPool []*domain.Trunk
	for _, t := range availablePool {
		health, _ := pe.cache.GetTrunkHealth(ctx, t.ID)
		if health == nil || (health.Status != "REJECTED" && health.Status != "UNREACHABLE") {
			healthyPool = append(healthyPool, t)
		}
	}
	if len(healthyPool) > 0 {
		availablePool = healthyPool
	}

	if len(availablePool) == 0 {
		return nil, domain.NewErrBadRequest("TRUNKS_UNAVAILABLE", "Nenhum tronco de saída disponível no pool")
	}

	numAgents := len(req.AvailableAgents)
	if numAgents == 0 {
		return &domain.PredictiveDemandResponse{
			CampaignID:      req.CampaignID,
			DialingChannels: 0,
			Status:          "idle_no_agents",
		}, nil
	}

	// Armazena operadores disponíveis para entrega da chamada em tempo real
	_ = pe.cache.StoreAvailableAgents(ctx, req.CampaignID, req.AvailableAgents, 30*time.Second)

	// 2. Parâmetros de Pacing
	contactProbability := 0.28 // 28% taxa média de contato
	hasInflated, _ := pe.cache.HasInflatedSuccessRate(ctx)
	if hasInflated {
		// Pós-abandono regulatório (< 2s): desacelera o ritmo forçando probabilidade 1.0 por 30s
		contactProbability = 1.0
	}

	ringTime := 18.0 // TMR 18s
	talkTime := 180.0 // TMA 180s
	if campaign != nil && campaign.Aggressiveness > 0 {
		aggressiveness = campaign.Aggressiveness
	}
	if req.Aggressiveness != nil && *req.Aggressiveness > 0 {
		aggressiveness = *req.Aggressiveness
	}

	// 3. Fórmula canônica de overdialing com piso mínimo por usuário disponível:
	rawChannels := (float64(numAgents) / contactProbability) * (1.0 + (ringTime / talkTime)) * aggressiveness
	calculatedDemand := int(math.Ceil(rawChannels))

	// Piso mínimo: discar taxa configurada de canais/chamadas por usuário disponível (default = 2:1)
	minRatio := pe.GetMinChannelsPerAgent()
	if req.MinChannelsPerAgent != nil && *req.MinChannelsPerAgent > 0 {
		minRatio = *req.MinChannelsPerAgent
	}
	minDemand := numAgents * minRatio
	if calculatedDemand < minDemand {
		calculatedDemand = minDemand
	}
	log.Printf("[DEMAND] Campaign: %s, Agents: %d, Demand: %d (min floor: %d, ratio: %d:1), Pool: %d", req.CampaignID, numAgents, calculatedDemand, minDemand, minRatio, len(availablePool))

	// 4. Limita à capacidade disponível no ChannelManager distribuindo pelo Pool de Troncos
	dispatched := 0
	for i := 0; i < calculatedDemand; i++ {
		// Seleciona tronco via Round-Robin atômico no pool disponível
		var selectedTrunk *domain.Trunk
		nTrunks := len(availablePool)
		if nTrunks == 0 {
			break
		}
		startIdx := atomic.AddUint64(&pe.roundRobinIdx, 1)
		for j := 0; j < nTrunks; j++ {
			candidate := availablePool[(int(startIdx)+j)%nTrunks]
			pe.channels.RegisterTrunkLimit(candidate.ID, candidate.MaxChannels)
			can, _ := pe.channels.CanAcquireSlot(candidate.ID, false)
			if can {
				selectedTrunk = candidate
				break
			}
		}

		if selectedTrunk == nil {
			log.Printf("[DEMAND] No trunk could acquire slot among %d available", len(availablePool))
			break // Todos os troncos do pool atingiram rigorosamente o seu max_channels
		}

		rawLead, err := pe.cache.PopLead(ctx, req.CampaignID)
		var leadItem domain.LeadQueueItem
		if rawLead != "" {
			if strings.HasPrefix(rawLead, "{") {
				_ = json.Unmarshal([]byte(rawLead), &leadItem)
			} else {
				leadItem.Phone = rawLead
			}
		}

		if leadItem.Phone == "" && pe.leads != nil {
			// Reabastecimento autônomo da fila a partir do PostgreSQL (FILO)
			dbLeads, fetchErr := pe.leads.FetchNextFILOBatch(ctx, req.CampaignID, 50, 2)
			if fetchErr != nil || len(dbLeads) == 0 {
				_ = pe.leads.RecoverStaleDialing(ctx, req.CampaignID)
				dbLeads, _ = pe.leads.FetchNextFILOBatch(ctx, req.CampaignID, 50, 2)
			}
			if len(dbLeads) == 0 {
				total, available, _, _ := pe.leads.CountCampaignLeads(ctx, req.CampaignID)
				if total > 0 && available == 0 {
					_, _ = pe.leads.ResetCampaignCycle(ctx, req.CampaignID)
				}
				dbLeads, _ = pe.leads.FetchNextFILOBatch(ctx, req.CampaignID, 50, 0)
			}
			if len(dbLeads) > 0 {
				var leadBatch []string
				for _, l := range dbLeads {
					item := domain.LeadQueueItem{
						Phone:     l.Phone,
						CPF:       l.CPF,
						Name:      l.Name,
						FirstName: l.FirstName,
						WorkWord:  l.WorkWord,
						LeadID:    l.ID,
					}
					b, _ := json.Marshal(item)
					leadBatch = append(leadBatch, string(b))
					_ = pe.leads.MarkDialing(ctx, l.ID)
				}
				if len(leadBatch) > 0 {
					_ = pe.cache.PushLeads(ctx, req.CampaignID, leadBatch[1:])
					_ = json.Unmarshal([]byte(leadBatch[0]), &leadItem)
				}
			}
		}
		phone := leadItem.Phone
		if phone == "" {
			log.Printf("[DEMAND] Phone is empty string - queue exhausted!")
			break // Fila esgotada
		}
		log.Printf("[DEMAND] Selected trunk %s for phone %s (Name: %s, CPF: %s)", selectedTrunk.ID, phone, leadItem.Name, leadItem.CPF)

		callID := fmt.Sprintf("pred-%s", uuid.New().String())
		activeChan := &domain.ActiveChannel{
			ChannelID:  callID,
			TrunkID:    selectedTrunk.ID,
			TenantID:   req.TenantID,
			CampaignID: &req.CampaignID,
			Phone:      phone,
			CPF:        leadItem.CPF,
			Name:       leadItem.Name,
			Att1:       leadItem.Att1,
			Att2:       leadItem.Att2,
			Att3:       leadItem.Att3,
			CallType:   domain.CallTypePredictive,
			StartedAt:  time.Now(),
			IsAnswered: false,
		}

		if err := pe.channels.AcquireSlot(ctx, activeChan, false); err != nil {
			leadBytes, _ := json.Marshal(leadItem)
			_ = pe.cache.PushLeads(ctx, req.CampaignID, []string{string(leadBytes)})
			break
		}

		destPhone := phone
		digitsOnly := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, destPhone)
		if len(digitsOnly) > 0 {
			destPhone = digitsOnly
		}

		callerID := destPhone
		dialChannel := selectedTrunk.DialString(destPhone)
		actionID := fmt.Sprintf("orig-%s", callID)

		leadIDStr := fmt.Sprintf("%d", leadItem.LeadID)
		if leadIDStr == "0" || leadIDStr == "" {
			leadIDStr = phone
		}

		audioName := domain.Slugify(leadItem.FirstName)
		if audioName == "" {
			audioName = domain.Slugify(leadItem.Name)
		}
		workWord := domain.Slugify(leadItem.WorkWord)

		vars := map[string]string{
			"CALL_ID":       callID,
			"__CALL_ID":     callID,
			"TENANT_ID":     req.TenantID,
			"__TENANT_ID":   req.TenantID,
			"CAMPAIGN_ID":   req.CampaignID,
			"__CAMPAIGN_ID": req.CampaignID,
			"CALL_TYPE":     "PREDICTIVE",
			"__CALL_TYPE":   "PREDICTIVE",
			"TRUNK_ID":      selectedTrunk.ID,
			"__TRUNK_ID":    selectedTrunk.ID,
			"PHONE":         destPhone,
			"__PHONE":       destPhone,
			"LEAD_ID":       leadIDStr,
			"__LEAD_ID":     leadIDStr,
			"LEAD_CPF":      leadItem.CPF,
			"__LEAD_CPF":    leadItem.CPF,
			"LEAD_NAME":     leadItem.Name,
			"__LEAD_NAME":   leadItem.Name,
			"AUDIO_NAME":    audioName,
			"__AUDIO_NAME":  audioName,
			"WORK_WORD":     workWord,
			"__WORK_WORD":   workWord,
		}
		if selectedTrunk.UserAgent != nil && *selectedTrunk.UserAgent != "" {
			vars["TRUNK_USER_AGENT"] = *selectedTrunk.UserAgent
		}

		err = pe.ami.Originate(ctx, actionID, dialChannel, "from-dialer-amd", "s", 1, 25, callerID, req.TenantID, vars)
		if err != nil {
			pe.channels.ReleaseSlot(ctx, callID)
			continue
		}

		dispatched++
	}

	return &domain.PredictiveDemandResponse{
		CampaignID:      req.CampaignID,
		DialingChannels: dispatched,
		Status:          "active",
	}, nil
}

// HandlePredictiveHuman é acionado quando o Asterisk detecta humano no triagem-amd (UserEvent PredictiveHuman)
func (pe *PredictiveEngine) HandlePredictiveHuman(ctx context.Context, channel, uniqueID, phone, campaignID, leadID string) error {
	callID := pe.channels.GetCallIDByAsterisk(channel, uniqueID)

	// 1. Extrai metadados do cliente a partir do canal ativo
	var customer *domain.CustomerMetadata
	if callID != "" {
		if activeChan := pe.channels.GetActiveChannel(callID); activeChan != nil {
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

	// 3. Disparo assíncrono do webhook inject-lead com resiliência (sem travar a telefonia)
	if pe.webhookClient != nil {
		go pe.dispatchInjectLeadWebhook(callID, userID, phone, campaignID)
	}

	// 4. Executa a transferência no Asterisk para o LiveKit SIP com suporte aos cabeçalhos X-Customer-*
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

