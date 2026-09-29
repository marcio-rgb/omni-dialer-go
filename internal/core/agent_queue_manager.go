package core

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// AgentQueueManager gerencia membros dinâmicos nas filas Asterisk (app_queue) e concilia com Redis.
//
// @pattern Strategy / Facade (Agent Queue Control)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type AgentQueueManager struct {
	ami       ports.AMIPort
	cache     ports.CachePort
	agentRepo ports.AgentRepository
}

func NewAgentQueueManager(ami ports.AMIPort, cache ports.CachePort) *AgentQueueManager {
	return &AgentQueueManager{
		ami:   ami,
		cache: cache,
	}
}

// SetAgentRepository injeta o repositório de agentes para gravação da timeline em tenant_agent_history.
func (m *AgentQueueManager) SetAgentRepository(repo ports.AgentRepository) {
	m.agentRepo = repo
}

func (m *AgentQueueManager) recordHistory(ctx context.Context, tenantID, agentID, campaignID, status, action, reason, room, callID string) {
	if m.agentRepo == nil || agentID == "" {
		return
	}
	if tenantID == "" {
		tenantID = "default"
	}
	var campPtr, reasonPtr, roomPtr, callPtr *string
	if campaignID != "" {
		campPtr = &campaignID
	}
	if reason != "" {
		reasonPtr = &reason
	}
	if room != "" {
		roomPtr = &room
	}
	if callID != "" {
		callPtr = &callID
	}
	h := &domain.TenantAgentHistory{
		TenantID:    tenantID,
		AgentID:     agentID,
		CampaignID:  campPtr,
		Status:      status,
		Action:      action,
		Reason:      reasonPtr,
		LiveKitRoom: roomPtr,
		CallID:      callPtr,
		StartedAt:   time.Now(),
	}
	if err := m.agentRepo.RecordHistory(ctx, h); err != nil {
		log.Printf("[WARN] [QUEUE-MGR] Falha ao registrar timeline de agente %s: %v", agentID, err)
	}
}

// AddMember adiciona um atendente à fila Asterisk e registra presença no Redis.
//
// @preExecution Validação de campaign_id e agent_id obrigatórios.
// @postExecution Disparo de AMI QueueAdd e sincronização de AgentRedisData no cache.
func (m *AgentQueueManager) AddMember(ctx context.Context, req *domain.QueueMemberRequest) error {
	if req.CampaignID == "" || req.AgentID == "" {
		return domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "campaign_id e agent_id são obrigatórios")
	}

	queueName := domain.FormatQueueName(req.TenantID, req.CampaignID)
	roomName := formatAgentRoom(req.AgentID, req.LiveKitRoom, req.SIPRoute)
	iface := formatAgentQueueInterface(req.AgentID, req.LiveKitRoom, req.SIPRoute)
	actionID := fmt.Sprintf("qadd-%s-%s-%d", req.CampaignID, req.AgentID, time.Now().UnixMilli())

	if m.ami != nil && m.ami.IsConnected() {
		if err := m.ami.QueueAdd(ctx, actionID, queueName, iface, req.AgentID, req.Penalty, req.Paused); err != nil {
			log.Printf("[WARN] [QUEUE-MGR] Falha ao adicionar membro %s na fila %s via AMI: %v", req.AgentID, queueName, err)
		} else {
			log.Printf("[INFO] [QUEUE-MGR] Membro %s (%s) adicionado à fila %s com sucesso no Asterisk.", req.AgentID, iface, queueName)
		}
	}

	if m.cache != nil && !req.Paused {
		agentData := &domain.AgentRedisData{
			AgentID:     req.AgentID,
			LiveKitRoom: roomName,
		}
		_ = m.cache.RemoveAgentFromQueues(ctx, req.AgentID)
		_ = m.cache.PushIdleAgent(ctx, agentData)
	}

	if req.Paused {
		m.recordHistory(ctx, req.TenantID, req.AgentID, req.CampaignID, "PAUSED", "add_paused", "", roomName, "")
	} else {
		m.recordHistory(ctx, req.TenantID, req.AgentID, req.CampaignID, "AVAILABLE", "add", "", roomName, "")
	}

	return nil
}

// RemoveMember remove o atendente da fila Asterisk e purga do Redis.
func (m *AgentQueueManager) RemoveMember(ctx context.Context, req *domain.QueueMemberRemoveRequest) error {
	if req.AgentID == "" {
		return domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "agent_id é obrigatório")
	}

	queueName := ""
	if req.CampaignID != "" && req.CampaignID != "all" {
		queueName = domain.FormatQueueName(req.TenantID, req.CampaignID)
	}

	iface := formatAgentQueueInterface(req.AgentID, req.LiveKitRoom, "")
	actionID := fmt.Sprintf("qrem-%s-%s-%d", req.CampaignID, req.AgentID, time.Now().UnixMilli())

	if m.ami != nil && m.ami.IsConnected() {
		if err := m.ami.QueueRemove(ctx, actionID, queueName, iface); err != nil {
			log.Printf("[WARN] [QUEUE-MGR] Falha ao remover membro %s da fila %s via AMI: %v", req.AgentID, queueName, err)
		} else {
			log.Printf("[INFO] [QUEUE-MGR] Membro %s removido da fila %s no Asterisk.", req.AgentID, queueName)
		}
	}

	if m.cache != nil {
		_ = m.cache.RemoveAgentFromQueues(ctx, req.AgentID)
	}

	m.recordHistory(ctx, req.TenantID, req.AgentID, req.CampaignID, "OFFLINE", "remove", "", "", "")

	return nil
}

// PauseMember altera o status de pausa do atendente na fila Asterisk e sincroniza o cache Redis.
func (m *AgentQueueManager) PauseMember(ctx context.Context, req *domain.QueueMemberPauseRequest) error {
	if req.AgentID == "" {
		return domain.NewErrBadRequest("MISSING_REQUIRED_FIELDS", "agent_id é obrigatório")
	}

	queueName := ""
	if req.CampaignID != "" && req.CampaignID != "all" {
		queueName = domain.FormatQueueName(req.TenantID, req.CampaignID)
	}

	roomName := formatAgentRoom(req.AgentID, req.LiveKitRoom, "")
	iface := formatAgentQueueInterface(req.AgentID, req.LiveKitRoom, "")
	actionID := fmt.Sprintf("qpause-%s-%s-%d", req.CampaignID, req.AgentID, time.Now().UnixMilli())

	if m.ami != nil && m.ami.IsConnected() {
		if err := m.ami.QueuePause(ctx, actionID, queueName, iface, req.Paused, req.Reason); err != nil {
			log.Printf("[WARN] [QUEUE-MGR] Falha ao alterar pausa do membro %s na fila %s via AMI: %v", req.AgentID, queueName, err)
			// Auto-cura: se despausando e membro não existe na fila Asterisk, adiciona o membro imediatamente!
			if !req.Paused && queueName != "" {
				addActID := fmt.Sprintf("qadd-heal-%s-%s-%d", req.CampaignID, req.AgentID, time.Now().UnixMilli())
				if addErr := m.ami.QueueAdd(ctx, addActID, queueName, iface, req.AgentID, 0, false); addErr != nil {
					log.Printf("[WARN] [QUEUE-MGR] Falha na auto-adição de membro %s na fila %s: %v", req.AgentID, queueName, addErr)
				} else {
					log.Printf("[INFO] [QUEUE-MGR] Membro %s auto-adicionado à fila %s com sucesso no Asterisk.", req.AgentID, queueName)
				}
			}
		} else {
			log.Printf("[INFO] [QUEUE-MGR] Membro %s na fila %s pausa definida como %v (motivo: %s).", req.AgentID, queueName, req.Paused, req.Reason)
		}
	}

	if m.cache != nil {
		if req.Paused {
			// Se pausado (Tabulação, Pausa ou Negociação), remove do pool de ociosos do discador
			_ = m.cache.RemoveAgentFromQueues(ctx, req.AgentID)
		} else {
			// Se despausado (Disponível/Pronto), recoloca no pool de ociosos garantindo unicidade
			_ = m.cache.RemoveAgentFromQueues(ctx, req.AgentID)
			_ = m.cache.PushIdleAgent(ctx, &domain.AgentRedisData{
				AgentID:     req.AgentID,
				LiveKitRoom: roomName,
			})
		}
	}

	if req.Paused {
		m.recordHistory(ctx, req.TenantID, req.AgentID, req.CampaignID, "PAUSED", "pause", req.Reason, roomName, "")
	} else {
		m.recordHistory(ctx, req.TenantID, req.AgentID, req.CampaignID, "AVAILABLE", "unpause", "", roomName, "")
	}

	return nil
}

// SetPresence é o ponto de entrada unificado para atualizar o estado de presença do operador.
func (m *AgentQueueManager) SetPresence(ctx context.Context, event *domain.QueuePresenceEvent) error {
	return m.ProcessPresenceEvent(ctx, event)
}

// ProcessPresenceEvent processa eventos de presença recebidos via HTTP REST, Redis Pub/Sub ou Webhooks.
func (m *AgentQueueManager) ProcessPresenceEvent(ctx context.Context, event *domain.QueuePresenceEvent) error {
	if event == nil {
		return nil
	}

	action := strings.ToLower(strings.TrimSpace(event.Action))
	switch action {
	case "add", "join":
		return m.AddMember(ctx, &domain.QueueMemberRequest{
			TenantID:    event.TenantID,
			CampaignID:  event.CampaignID,
			AgentID:     event.AgentID,
			LiveKitRoom: event.LiveKitRoom,
			SIPRoute:    event.SIPRoute,
			Penalty:     event.Penalty,
			Paused:      event.Paused,
		})

	case "online", "available", "ready", "unpause", "resume", "retomar", "despausar":
		// Primeiro despausa na fila
		err := m.PauseMember(ctx, &domain.QueueMemberPauseRequest{
			TenantID:    event.TenantID,
			CampaignID:  event.CampaignID,
			AgentID:     event.AgentID,
			LiveKitRoom: event.LiveKitRoom,
			Paused:      false,
			Reason:      event.Reason,
		})
		// Se não encontrou ou falhou, garante adição como membro ativo
		if err != nil && event.CampaignID != "" {
			return m.AddMember(ctx, &domain.QueueMemberRequest{
				TenantID:    event.TenantID,
				CampaignID:  event.CampaignID,
				AgentID:     event.AgentID,
				LiveKitRoom: event.LiveKitRoom,
				SIPRoute:    event.SIPRoute,
				Penalty:     event.Penalty,
				Paused:      false,
			})
		}
		return err

	case "pause", "paused", "tabulando", "acw", "wrapup", "negociacao", "pausa":
		reason := event.Reason
		if reason == "" {
			reason = strings.ToUpper(action)
		}
		return m.PauseMember(ctx, &domain.QueueMemberPauseRequest{
			TenantID:    event.TenantID,
			CampaignID:  event.CampaignID,
			AgentID:     event.AgentID,
			LiveKitRoom: event.LiveKitRoom,
			Paused:      true,
			Reason:      reason,
		})

	case "remove", "leave", "offline", "disconnect", "logout":
		return m.RemoveMember(ctx, &domain.QueueMemberRemoveRequest{
			TenantID:    event.TenantID,
			CampaignID:  event.CampaignID,
			AgentID:     event.AgentID,
			LiveKitRoom: event.LiveKitRoom,
		})

	default:
		log.Printf("[WARN] [QUEUE-MGR] Ação de presença desconhecida: %s (agent_id: %s, campaign: %s)", event.Action, event.AgentID, event.CampaignID)
		return domain.NewErrBadRequest("UNKNOWN_PRESENCE_ACTION", fmt.Sprintf("Ação de presença desconhecida: '%s'", event.Action))
	}
}
