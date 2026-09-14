package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type TrunkManager struct {
	repo       ports.TrunkRepository
	cache      ports.CachePort
	ami        ports.AMIPort
	channels   *ChannelManager
	reportRepo ports.ReportRepository
	routing    ports.RoutingRepository
	predictive *PredictiveEngine
	inbound    *InboundEngine
	notifier   *CallNotifier
	stopCh     chan struct{}
	mu         sync.RWMutex
}

func NewTrunkManager(repo ports.TrunkRepository, cache ports.CachePort, ami ports.AMIPort, channels *ChannelManager, reportRepo ports.ReportRepository, routing ports.RoutingRepository) *TrunkManager {
	return &TrunkManager{
		repo:       repo,
		cache:      cache,
		ami:        ami,
		channels:   channels,
		reportRepo: reportRepo,
		routing:    routing,
		stopCh:     make(chan struct{}),
	}
}

// SetEngines injeta os motores preditivo e receptivo para despacho de eventos em tempo real.
func (tm *TrunkManager) SetEngines(predictive *PredictiveEngine, inbound *InboundEngine) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.predictive = predictive
	tm.inbound = inbound
}

// SetNotifier injeta o despachador de notificações de chamadas para sistemas upstream.
func (tm *TrunkManager) SetNotifier(notifier *CallNotifier) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.notifier = notifier
}

// StartDaemon inicializa a escuta de eventos AMI e o loop de qualify de troncos.
func (tm *TrunkManager) StartDaemon(ctx context.Context) {
	go tm.eventListenerLoop(ctx)
	go tm.qualifyLoop(ctx)
	go func() {
		time.Sleep(1 * time.Second)
		tm.PollAsteriskEndpoints(ctx)
	}()
}

func (tm *TrunkManager) Stop() {
	close(tm.stopCh)
}

func (tm *TrunkManager) eventListenerLoop(ctx context.Context) {
	eventCh := tm.ami.SubscribeEvents()
	for {
		select {
		case <-tm.stopCh:
			return
		case <-ctx.Done():
			return
		case evt := <-eventCh:
			switch evt.Name {
			case "ContactStatus":
				tm.handleContactStatus(ctx, evt.Attributes)
			case "Registry":
				tm.handleRegistry(ctx, evt.Attributes)
			case "OriginateResponse":
				tm.handleOriginateResponse(ctx, evt.Attributes)
			case "VarSet":
				tm.handleVarSet(ctx, evt.Attributes)
			case "Hangup":
				tm.handleHangup(ctx, evt.Attributes)
			case "UserEvent":
				tm.handleUserEvent(ctx, evt.Attributes)
			case "Newstate":
				tm.handleNewstate(ctx, evt.Attributes)
			}
		}
	}
}

func (tm *TrunkManager) qualifyLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			trunks, err := tm.repo.ListAllEnabled(ctx)
			if err != nil {
				continue
			}

			tm.PollAsteriskEndpoints(ctx)
			for _, t := range trunks {
				tm.channels.RegisterTrunkLimit(t.ID, t.MaxChannels)

				cached, _ := tm.cache.GetTrunkHealth(ctx, t.ID)
				if cached == nil {
					health := domain.TrunkHealth{
						Status:         "OFFLINE",
						LatencyMS:      0.0,
						ActiveChannels: tm.channels.GetTrunkActiveCount(t.ID),
						MaxChannels:    t.MaxChannels,
						LastQualifyAt:  time.Time{},
					}
					_ = tm.cache.SetTrunkHealth(ctx, t.ID, health, 24*time.Hour)
				}
			}
		}
	}
}

// ReloadPBXTrunks recarrega a configuração do Asterisk e reconstrói o agrupamento/limites de troncos.
//
// @pattern Pool Manager (Hot Reloader & Synchronizer)
// @governedBy docs/rules/TRUNKS_LIFECYCLE.md#3-hot-reload-a-quente-no-asterisk-pbx
//
// @preExecution
// - Reconciliação dos contadores atômicos em `ChannelManager`
// - Consulta dos troncos habilitados no PostgreSQL (`trunks`)
//
// @postExecution
// - Registro dos limites de capacidade granular por tronco
// - Disparo de comandos AMI `pjsip reload` e `dialplan reload`
func (tm *TrunkManager) ReloadPBXTrunks(ctx context.Context) error {
	// 1. Reconcilia canais ativos em memória
	tm.channels.ReconcileCounters(ctx)

	// 2. Busca todos os troncos habilitados no banco e reconstrói os limites granulares
	trunks, err := tm.repo.ListAllEnabled(ctx)
	if err == nil {
		for _, t := range trunks {
			tm.channels.RegisterTrunkLimit(t.ID, t.MaxChannels)
			existing, _ := tm.cache.GetTrunkHealth(ctx, t.ID)
			if existing == nil {
				health := domain.TrunkHealth{
					Status:         "OFFLINE",
					LatencyMS:      0.0,
					ActiveChannels: tm.channels.GetTrunkActiveCount(t.ID),
					MaxChannels:    t.MaxChannels,
					LastQualifyAt:  time.Time{},
				}
				_ = tm.cache.SetTrunkHealth(ctx, t.ID, health, 24*time.Hour)
			}
		}
	}

	// 3. Comanda o Asterisk via AMI para recarregar PJSIP e Dialplan
	actionID1 := fmt.Sprintf("reload-pjsip-%d", time.Now().UnixNano())
	_, _ = tm.ami.Command(ctx, actionID1, "pjsip reload")

	actionID2 := fmt.Sprintf("reload-dialplan-%d", time.Now().UnixNano())
	_, _ = tm.ami.Command(ctx, actionID2, "dialplan reload")

	return nil
}
