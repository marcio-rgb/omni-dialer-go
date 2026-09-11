package core

import (
	"context"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type InboundEngine struct {
	ami        ports.AMIPort
	routing    ports.RoutingRepository
	channels   *ChannelManager
	defaultContext string
	defaultExten   string
}

func NewInboundEngine(ami ports.AMIPort, routing ports.RoutingRepository, channels *ChannelManager, defContext, defExten string) *InboundEngine {
	return &InboundEngine{
		ami:            ami,
		routing:        routing,
		channels:       channels,
		defaultContext: defContext,
		defaultExten:   defExten,
	}
}

// ProcessInboundCall recebe o evento UserEvent(InboundCall) e resolve o roteamento O(1).
func (ie *InboundEngine) ProcessInboundCall(ctx context.Context, channel, callerPhone, didNumber string) error {
	actionID := fmt.Sprintf("inbound-%d", time.Now().UnixNano())

	// 1. Consulta B-tree O(1) na tabela auxiliar phone_trunk_mappings
	mapping, err := ie.routing.GetLastRouting(ctx, callerPhone)
	if err == nil && mapping != nil && mapping.LastTrunk != "" {
		// Verifica se o tronco anterior possui capacidade para receber a chamada
		can, _ := ie.channels.CanAcquireSlot(mapping.LastTrunk, true)
		if can {
			// Roteia para o contexto/extensão associado ao mesmo projeto/tronco de origem
			targetContext := mapping.LastProject
			if targetContext == "" {
				targetContext = ie.defaultContext
			}
			targetExten := mapping.LastSIPRoute
			if targetExten == "" {
				targetExten = ie.defaultExten
			}

			return ie.ami.Redirect(ctx, actionID, channel, "", targetContext, targetExten, 1)
		}
	}

	// 2. Fallback: Roteia para o tronco/fila receptiva padrão
	return ie.ami.Redirect(ctx, actionID, channel, "", ie.defaultContext, ie.defaultExten, 1)
}

// RecordOutboundDestination grava o mapeamento de retorno O(1) pós-atendimento
func (ie *InboundEngine) RecordOutboundDestination(ctx context.Context, phone, trunkID, project, sipRoute, tenantID string) error {
	mapping := &domain.PhoneTrunkMapping{
		Phone:        phone,
		LastTrunk:    trunkID,
		LastProject:  project,
		LastSIPRoute: sipRoute,
		TenantID:     tenantID,
		UpdatedAt:    time.Now(),
	}
	return ie.routing.SaveRouting(ctx, mapping)
}
