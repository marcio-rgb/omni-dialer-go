package core

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/google/uuid"
)

type ManualEngine struct {
	ami           ports.AMIPort
	channels      *ChannelManager
	trunks        ports.TrunkRepository
	cache         ports.CachePort
	roundRobinIdx uint64
}

func NewManualEngine(ami ports.AMIPort, channels *ChannelManager, trunks ports.TrunkRepository, cache ports.CachePort) *ManualEngine {
	return &ManualEngine{
		ami:      ami,
		channels: channels,
		trunks:   trunks,
		cache:    cache,
	}
}

// DialManual dispara chamada manual prioritária.
func (me *ManualEngine) DialManual(ctx context.Context, req *domain.ManualCallRequest) (*domain.ManualCallResponse, error) {
	// 1. Identifica tronco disponível para a chamada manual
	trunkID := req.TrunkID
	var trunk *domain.Trunk
	var err error

	if trunkID != "" && trunkID != "auto" && trunkID != "default" {
		trunk, err = me.trunks.GetByID(ctx, req.TenantID, trunkID)
		if err != nil || trunk == nil || !trunk.IsEnabled {
			return nil, domain.NewErrNotFound("TRUNK_NOT_FOUND", fmt.Sprintf("Tronco '%s' indisponível", trunkID))
		}
	} else {
		// Busca o melhor tronco habilitado do tenant no pool respeitando rigorosamente max_channels e telemetria de saúde
		list, err := me.trunks.ListByTenant(ctx, req.TenantID)
		if err != nil || len(list) == 0 {
			return nil, domain.NewErrBadRequest("NO_AVAILABLE_TRUNKS", "Nenhum tronco habilitado encontrado para o tenant")
		}

		// Filtra troncos do pool ignorando os que estão REJECTED ou UNREACHABLE
		var healthyList []*domain.Trunk
		for _, t := range list {
			if t.IsEnabled && t.ID != "livekit" && t.ID != "livekit-sip" && t.Direction != domain.DirectionInbound {
				health, _ := me.cache.GetTrunkHealth(ctx, t.ID)
				if health == nil || (health.Status != "REJECTED" && health.Status != "UNREACHABLE") {
					healthyList = append(healthyList, t)
				}
			}
		}

		poolCandidates := healthyList
		if len(poolCandidates) == 0 {
			for _, t := range list {
				if t.IsEnabled && t.ID != "livekit" && t.ID != "livekit-sip" && t.Direction != domain.DirectionInbound {
					poolCandidates = append(poolCandidates, t)
				}
			}
		}

		nPool := len(poolCandidates)
		if nPool == 0 {
			return nil, domain.NewErrTooManyRequests("ALL_TRUNKS_EXHAUSTED", "Todos os troncos telefônicos atingiram sua capacidade máxima simultânea", nil)
		}
		startIdx := atomic.AddUint64(&me.roundRobinIdx, 1)
		for j := 0; j < nPool; j++ {
			candidate := poolCandidates[(int(startIdx)+j)%nPool]
			me.channels.RegisterTrunkLimit(candidate.ID, candidate.MaxChannels)
			can, _ := me.channels.CanAcquireSlot(candidate.ID, true)
			if can {
				trunk = candidate
				trunkID = candidate.ID
				break
			}
		}
		if trunk == nil {
			return nil, domain.NewErrTooManyRequests("ALL_TRUNKS_EXHAUSTED", "Todos os troncos telefônicos atingiram sua capacidade máxima simultânea", nil)
		}
	}

	// 2. Validação hierárquica de capacidade
	can, reason := me.channels.CanAcquireSlot(trunkID, true)
	if !can {
		if reason == "TRUNK_CHANNELS_EXHAUSTED" {
			meta := map[string]interface{}{
				"trunk_id":        trunkID,
				"active_channels": me.channels.GetTrunkActiveCount(trunkID),
				"max_channels":    trunk.MaxChannels,
			}
			return nil, domain.NewErrTooManyRequests("TRUNK_CHANNELS_EXHAUSTED",
				fmt.Sprintf("O tronco '%s' atingiu o limite máximo de %d canais.", trunkID, trunk.MaxChannels), meta)
		}
		return nil, domain.NewErrTooManyRequests("CAPACITY_EXHAUSTED", "Central PBX com capacidade máxima atingida", nil)
	}

	destPhone := req.Phone
	digitsOnly := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, destPhone)
	if len(digitsOnly) > 0 {
		destPhone = digitsOnly
	}

	callID := fmt.Sprintf("manual-%s", uuid.New().String())
	dialChannel := trunk.DialString(destPhone)

	activeChan := &domain.ActiveChannel{
		ChannelID:  callID,
		TrunkID:    trunkID,
		TenantID:   req.TenantID,
		Phone:      destPhone,
		CallType:   domain.CallTypeManual,
		AgentID:    &req.AgentID,
		SIPRoute:   &req.SIPRoute,
		StartedAt:  time.Now(),
		IsAnswered: false,
	}

	// 3. Aloca slot imediatamente (preempção)
	if err := me.channels.AcquireSlot(ctx, activeChan, true); err != nil {
		return nil, domain.NewErrTooManyRequests("SLOT_ACQUISITION_FAILED", err.Error(), nil)
	}

	// 4. Emite Originate conectando a perna ao sip_route do operador
	actionID := fmt.Sprintf("act-%s", callID)
	callerID := trunkID
	if trunk.FromUser != nil && *trunk.FromUser != "" {
		callerID = *trunk.FromUser
	}

	// Resolve a rota SIP do operador dinamicamente a partir do tronco livekit cadastrado no banco se necessário
	sipRoute := req.SIPRoute
	if !strings.HasPrefix(sipRoute, "PJSIP/") {
		lkTrunk, err := me.trunks.GetByID(ctx, req.TenantID, "livekit-sip")
		if err == nil && lkTrunk != nil && lkTrunk.IsEnabled {
			if lkTrunk.Host != "" {
				sipRoute = fmt.Sprintf("PJSIP/%s/sip:%s@%s:%d", lkTrunk.ID, req.SIPRoute, lkTrunk.Host, lkTrunk.Port)
			} else {
				sipRoute = fmt.Sprintf("PJSIP/%s@%s", req.SIPRoute, lkTrunk.ID)
			}
		} else {
			sipRoute = fmt.Sprintf("PJSIP/%s@livekit-sip", req.SIPRoute)
		}
	}

	leadName := req.LeadName
	if leadName == "" {
		leadName = req.Name
	}
	leadCPF := req.LeadCPF
	if leadCPF == "" {
		leadCPF = req.CPF
	}

	vars := map[string]string{
		"CALL_ID":      callID,
		"__CALL_ID":    callID,
		"TENANT_ID":    req.TenantID,
		"__TENANT_ID":  req.TenantID,
		"AGENT_ID":     req.AgentID,
		"__AGENT_ID":   req.AgentID,
		"CALL_TYPE":    "MANUAL",
		"__CALL_TYPE":  "MANUAL",
		"SIP_ROUTE":    sipRoute,
		"__SIP_ROUTE":  sipRoute,
		"PHONE":        destPhone,
		"__PHONE":      destPhone,
		"LEAD_NAME":    leadName,
		"__LEAD_NAME":  leadName,
		"LEAD_CPF":     leadCPF,
		"__LEAD_CPF":   leadCPF,
		"TRUNK_ID":     trunkID,
		"__TRUNK_ID":   trunkID,
	}
	if trunk != nil && trunk.UserAgent != nil && *trunk.UserAgent != "" {
		vars["TRUNK_USER_AGENT"] = *trunk.UserAgent
	}

	// Dispara para o contexto de entrega direta ao operador usando Exten 's'
	err = me.ami.Originate(ctx, actionID, dialChannel, "from-dialer-manual", "s", 1, 30, callerID, req.TenantID, vars)
	if err != nil {
		me.channels.ReleaseSlot(ctx, callID)
		return nil, domain.NewErrInternal(fmt.Sprintf("Falha ao originar chamada manual no PBX: %s", err.Error()))
	}

	return &domain.ManualCallResponse{
		CallID:    callID,
		Status:    "dialing",
		TrunkUsed: trunkID,
	}, nil
}
