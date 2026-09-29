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
	reports             ports.ReportRepository
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

func (pe *PredictiveEngine) SetReportRepository(reports ports.ReportRepository) {
	pe.reports = reports
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

	// 3. Monta o Pool de Troncos de Saída habilitados
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

	// 4. Sincroniza operadores na fila Asterisk (app_queue) e armazena em cache
	queueName := domain.FormatQueueName(req.TenantID, req.CampaignID)
	for _, agent := range req.AvailableAgents {
		roomName := formatAgentRoom(agent.AgentID, "", agent.SIPRoute)
		iface := formatAgentQueueInterface(agent.AgentID, "", agent.SIPRoute)
		_ = pe.ami.QueueAdd(ctx, fmt.Sprintf("qadd-%s-%s", req.CampaignID, agent.AgentID), queueName, iface, agent.AgentID, 0, false)
		if pe.cache != nil {
			_ = pe.cache.PushIdleAgent(ctx, &domain.AgentRedisData{
				AgentID:     agent.AgentID,
				LiveKitRoom: roomName,
			})
		}
	}
	_ = pe.cache.StoreAvailableAgents(ctx, req.CampaignID, req.AvailableAgents, 30*time.Second)

	// 5. Parâmetros de Pacing
	contactProbability := 0.28 // 28% taxa média de contato
	hasInflated, _ := pe.cache.HasInflatedSuccessRate(ctx)
	if hasInflated {
		contactProbability = 1.0
	}

	ringTime := 18.0 // TMR 18s
	talkTime := 180.0 // TMA 180s
	if req.Aggressiveness != nil && *req.Aggressiveness > 0 {
		aggressiveness = *req.Aggressiveness
	}

	// Fórmula canônica de overdialing
	rawChannels := (float64(numAgents) / contactProbability) * (1.0 + (ringTime / talkTime)) * aggressiveness
	calculatedDemand := int(math.Ceil(rawChannels))

	minRatio := pe.GetMinChannelsPerAgent()
	if req.MinChannelsPerAgent != nil && *req.MinChannelsPerAgent > 0 {
		minRatio = *req.MinChannelsPerAgent
	}
	minDemand := numAgents * minRatio
	if calculatedDemand < minDemand {
		calculatedDemand = minDemand
	}

	// 6. Consulta métricas de 10 minutos e erros consecutivos para Pacing Reativo & Circuit Breaker
	var abandonRate float64 = 0.0
	if pe.reports != nil {
		abandoned, answered, repErr := pe.reports.Get10MinAbandonStats(ctx, req.TenantID, req.CampaignID)
		if repErr == nil && answered > 0 {
			abandonRate = (float64(abandoned) / float64(answered)) * 100.0
		}
	}
	consecutiveErrors, _ := pe.cache.GetConsecutiveErrors(ctx, req.CampaignID)

	// 7. Avaliação de Circuit Breaker & Amortecimento Progressivo
	isCircuitBreaker := abandonRate >= 100.0 || consecutiveErrors >= 5
	if isCircuitBreaker {
		log.Printf("[CIRCUIT-BREAKER] Triggered for Campaign: %s (AbandonRate10m: %.2f%%, ConsecutiveErrors: %d)", req.CampaignID, abandonRate, consecutiveErrors)
		calculatedDemand = 2 // Limita estritamente a 2 canais de sondagem

		if pe.webhookClient != nil && pe.tenants != nil {
			tenant, _ := pe.tenants.GetByID(ctx, req.TenantID)
			if tenant != nil && tenant.Webhook != "" {
				alertPayload := &domain.SystemAlertWebhookPayload{
					Event:            "telephony.system_alert",
					TenantID:         req.TenantID,
					CampaignID:       req.CampaignID,
					AlertType:        "CIRCUIT_BREAKER_TRUNK_FAILURES",
					AbandonRate10m:   abandonRate,
					ConsecutiveFails: consecutiveErrors,
					Message:          fmt.Sprintf("Circuit Breaker ativado: taxa de abandono em 10m atingiu %.2f%% com %d falhas consecutivas.", abandonRate, consecutiveErrors),
					ActionRequired:   "Verifique a integridade do tronco telefônico e execute POST /api/v1/trunks/reload",
					Timestamp:        time.Now(),
				}
				_ = pe.webhookClient.NotifySystemAlert(ctx, tenant.Webhook, alertPayload)
			}
		}
	} else {
		// Escala Progressiva de Amortecimento:
		if abandonRate >= 20.0 {
			// >= 20%: Overdialing desativado (0x), discagem estritamente 1:1
			calculatedDemand = numAgents
		} else if abandonRate >= 10.0 {
			// 10% <= R < 20%: Redução de 60% no overdialing e piso forçado a 1:1
			overdialingExtra := float64(calculatedDemand - numAgents)
			if overdialingExtra > 0 {
				calculatedDemand = numAgents + int(math.Ceil(overdialingExtra*0.40))
			}
			if calculatedDemand < numAgents {
				calculatedDemand = numAgents
			}
		} else if abandonRate >= 5.0 {
			// 5% <= R < 10%: Redução de 30% no overdialing e teto de no máximo 2 canais por operador livre
			overdialingExtra := float64(calculatedDemand - numAgents)
			if overdialingExtra > 0 {
				calculatedDemand = numAgents + int(math.Ceil(overdialingExtra*0.70))
			}
			maxCeiling := numAgents * 2
			if calculatedDemand > maxCeiling {
				calculatedDemand = maxCeiling
			}
		}
	}

	// 8. Abatimento de Chamadas em Toque (In-Flight Subtraction)
	ringingCalls := pe.channels.GetCampaignRingingCalls(req.CampaignID)
	effectiveDemand := calculatedDemand - ringingCalls
	if effectiveDemand < 0 {
		effectiveDemand = 0
	}
	log.Printf("[DEMAND] Campaign: %s, Agents: %d, Calculated: %d, Ringing: %d, EffectiveDemand: %d (Abandon10m: %.2f%%)",
		req.CampaignID, numAgents, calculatedDemand, ringingCalls, effectiveDemand, abandonRate)

	// 9. Originate direto no contexto de fila Asterisk from-dialer-queue
	dispatched := 0
	for i := 0; i < effectiveDemand; i++ {
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
			break
		}

		rawLead, _ := pe.cache.PopLead(ctx, req.CampaignID)
		var leadItem domain.LeadQueueItem
		if rawLead != "" {
			if strings.HasPrefix(rawLead, "{") {
				_ = json.Unmarshal([]byte(rawLead), &leadItem)
			} else {
				leadItem.Phone = rawLead
			}
		}

		if leadItem.Phone == "" && pe.leads != nil {
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
						Att1:      l.Att1,
						Att2:      l.Att2,
						Att3:      l.Att3,
						Custom:    l.Custom,
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
			break
		}

		var leadIDPtr *int64
		if leadItem.LeadID > 0 {
			lid := leadItem.LeadID
			leadIDPtr = &lid
		}

		leadIDStr := fmt.Sprintf("%d", leadItem.LeadID)
		if leadIDStr == "0" || leadIDStr == "" {
			leadIDStr = phone
		}

		callID := fmt.Sprintf("pred-%s", uuid.New().String())
		activeChan := &domain.ActiveChannel{
			ChannelID:  callID,
			TrunkID:    selectedTrunk.ID,
			TenantID:   req.TenantID,
			CampaignID: &req.CampaignID,
			Phone:      phone,
			LeadID:     leadIDPtr,
			CPF:        leadItem.CPF,
			Name:       leadItem.Name,
			Att1:       leadItem.Att1,
			Att2:       leadItem.Att2,
			Att3:       leadItem.Att3,
			Custom:     leadItem.Custom,
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

		audioName := domain.Slugify(leadItem.FirstName)
		if audioName == "" {
			audioName = domain.Slugify(leadItem.Name)
		}
		workWord := domain.Slugify(leadItem.WorkWord)

		trunkAMD := "0"
		if selectedTrunk.AMDEnabled {
			trunkAMD = "1"
		}

		vars := map[string]string{
			"CALL_ID":           callID,
			"__CALL_ID":         callID,
			"TENANT_ID":         req.TenantID,
			"__TENANT_ID":       req.TenantID,
			"CAMPAIGN_ID":       req.CampaignID,
			"__CAMPAIGN_ID":     req.CampaignID,
			"CALL_TYPE":         "PREDICTIVE",
			"__CALL_TYPE":       "PREDICTIVE",
			"TRUNK_ID":          selectedTrunk.ID,
			"__TRUNK_ID":        selectedTrunk.ID,
			"PHONE":             destPhone,
			"__PHONE":           destPhone,
			"LEAD_ID":           leadIDStr,
			"__LEAD_ID":         leadIDStr,
			"LEAD_CPF":          leadItem.CPF,
			"__LEAD_CPF":        leadItem.CPF,
			"LEAD_NAME":         leadItem.Name,
			"__LEAD_NAME":       leadItem.Name,
			"AUDIO_NAME":        audioName,
			"__AUDIO_NAME":      audioName,
			"WORK_WORD":         workWord,
			"__WORK_WORD":       workWord,
			"QUEUE_NAME":        queueName,
			"__QUEUE_NAME":      queueName,
			"TRUNK_AMD_ENABLED":   trunkAMD,
			"__TRUNK_AMD_ENABLED": trunkAMD,
		}
		if selectedTrunk.UserAgent != nil && *selectedTrunk.UserAgent != "" {
			vars["TRUNK_USER_AGENT"] = *selectedTrunk.UserAgent
		}

		now := time.Now()
		if pe.cache != nil {
			initEvt := &domain.CDREvent{
				CallID:      callID,
				TenantID:    req.TenantID,
				CampaignID:  &req.CampaignID,
				TrunkUsed:   selectedTrunk.ID,
				Phone:       destPhone,
				LeadID:      leadIDPtr,
				LeadName:    &leadItem.Name,
				LeadCPF:     &leadItem.CPF,
				CallType:    domain.CallTypePredictive,
				Type:        domain.CDREventInitiated,
				StartedAt:   now,
				InitiatedAt: &now,
				Timestamp:   now.Unix(),
			}
			_ = pe.cache.PublishCDREvent(ctx, initEvt)
		}

		err = pe.ami.Originate(ctx, actionID, dialChannel, "from-dialer-amd", "s", 1, 25, callerID, req.TenantID, vars)
		if err != nil {
			if pe.cache != nil {
				failEvt := &domain.CDREvent{
					CallID:          callID,
					TenantID:        req.TenantID,
					CampaignID:      &req.CampaignID,
					TrunkUsed:       selectedTrunk.ID,
					Phone:           destPhone,
					LeadID:          leadIDPtr,
					LeadName:        &leadItem.Name,
					LeadCPF:         &leadItem.CPF,
					CallType:        domain.CallTypePredictive,
					Disposition:     domain.DispositionFailed,
					Type:            domain.CDREventOriginateFailed,
					StartedAt:       now,
					InitiatedAt:     &now,
					EndedAt:         &now,
					Timestamp:       now.Unix(),
					DurationSeconds: 0,
					BillsecSeconds:  0,
					RingSeconds:     0,
				}
				_ = pe.cache.PublishCDREvent(ctx, failEvt)
			}
			_, _ = pe.cache.IncrementConsecutiveErrors(ctx, req.CampaignID)
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

