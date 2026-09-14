package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
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

		callerID := pe.randomizeCallerID(destPhone)
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

// randomizeCallerID preserva o DDD e randomiza os últimos 4 dígitos.
func (pe *PredictiveEngine) randomizeCallerID(destPhone string) string {
	if len(destPhone) >= 10 {
		prefix := destPhone[:len(destPhone)-4]
		randomSuffix := rand.Intn(9000) + 1000
		return fmt.Sprintf("%s%d", prefix, randomSuffix)
	}
	return destPhone
}

// HandlePredictiveHuman é acionado quando o Asterisk detecta humano no triagem-amd (UserEvent PredictiveHuman)
func (pe *PredictiveEngine) HandlePredictiveHuman(ctx context.Context, channel, uniqueID, phone, campaignID, leadID string) error {
	actionID := fmt.Sprintf("pred-human-%d", time.Now().UnixNano())
	callID := pe.channels.GetCallIDByAsterisk(channel, uniqueID)

	// 1. Busca o próximo operador disponível na fila da campanha
	agent, err := pe.cache.GetNextAvailableAgent(ctx, campaignID)
	if err != nil || agent == nil {
		// Sem operador imediatamente disponível: pós-abandono regulatório (< 2s)
		if callID != "" {
			pe.channels.SetCallDisposition(callID, domain.DispositionAbandoned)
		}
		_ = pe.cache.SetInflatedSuccessRate(ctx, 30*time.Second)
		_ = pe.ami.Hangup(ctx, actionID, channel, 16)
		return fmt.Errorf("nenhum operador disponivel para campanha %s: chamada abandonada", campaignID)
	}

	// 2. Promove a chamada para cota humana e vincula o operador
	if callID != "" {
		pe.channels.AssignAgent(callID, agent.AgentID)
	}

	// 3. Define o nome da sala persistente do operador no LiveKit
	roomName := fmt.Sprintf("sala_agente_%s", agent.AgentID)

	// 4. Define a variável AGENT_ROOM no canal do Asterisk
	setVarCmd := fmt.Sprintf("dialplan set chanvar %s AGENT_ROOM %s", channel, roomName)
	_, _ = pe.ami.Command(ctx, fmt.Sprintf("setvar-%s", actionID), setVarCmd)

	// 5. Redireciona o canal do cliente para o contexto cos-all-custom exten 9999 (Dial LiveKit SIP)
	return pe.ami.Redirect(ctx, actionID, channel, "", "cos-all-custom", "9999", 1)
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

	setVarCmd := fmt.Sprintf("dialplan set chanvar %s AGENT_ROOM %s", channel, roomName)
	_, _ = pe.ami.Command(ctx, fmt.Sprintf("setvar-%s", actionID), setVarCmd)

	return pe.ami.Redirect(ctx, actionID, channel, "", "cos-all-custom", "9999", 1)
}

