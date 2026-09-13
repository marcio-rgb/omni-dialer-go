package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
			}
		}
	}
}

func (tm *TrunkManager) handleOriginateResponse(ctx context.Context, attrs map[string]string) {
	actionID := attrs["ActionID"]
	response := attrs["Response"]
	astChannel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]

	callID := actionID
	if strings.HasPrefix(callID, "act-") {
		callID = strings.TrimPrefix(callID, "act-")
	} else if strings.HasPrefix(callID, "orig-") {
		callID = strings.TrimPrefix(callID, "orig-")
	}

	if response != "Success" {
		if callID != "" {
			tm.channels.ReleaseSlot(ctx, callID)
		}
	} else {
		if callID != "" {
			tm.channels.LinkAsteriskChannel(callID, astChannel, uniqueID)
		}
	}
}

func (tm *TrunkManager) handleVarSet(ctx context.Context, attrs map[string]string) {
	if attrs["Variable"] == "CALL_ID" {
		callID := attrs["Value"]
		astChannel := attrs["Channel"]
		uniqueID := attrs["Uniqueid"]
		if callID != "" {
			tm.channels.LinkAsteriskChannel(callID, astChannel, uniqueID)
		}
	}
}

func (tm *TrunkManager) handleHangup(ctx context.Context, attrs map[string]string) {
	astChannel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]
	activeChan := tm.channels.ReleaseByAsterisk(ctx, astChannel, uniqueID)
	if activeChan == nil {
		return
	}

	tm.mu.RLock()
	repRepo := tm.reportRepo
	routRepo := tm.routing
	tm.mu.RUnlock()

	causeStr := attrs["Cause"]
	causeInt, _ := strconv.Atoi(causeStr)

	disposition := domain.DispositionFailed
	if activeChan.Disposition != nil {
		disposition = *activeChan.Disposition
	} else {
		switch causeInt {
		case 16: // Normal Clearing
			if activeChan.IsAnswered {
				if activeChan.AgentID != nil {
					disposition = domain.DispositionDelivered
				} else {
					disposition = domain.DispositionAnswered
				}
			} else {
				disposition = domain.DispositionAnswered
			}
		case 17:
			disposition = domain.DispositionBusy
		case 19:
			disposition = domain.DispositionNoAnswer
		case 34:
			disposition = domain.DispositionCongestion
		case 1, 28:
			disposition = domain.DispositionInvalidNumber
		default:
			if causeInt > 0 {
				disposition = domain.DispositionFailed
			}
		}
	}

	now := time.Now()
	duration := int(now.Sub(activeChan.StartedAt).Seconds())
	if duration < 0 {
		duration = 0
	}
	billsec := 0
	if activeChan.IsAnswered {
		billsec = duration
	}
	ringSeconds := duration - billsec
	if ringSeconds < 0 {
		ringSeconds = 0
	}

	if repRepo != nil {
		cdr := &domain.CDR{
			ID:              fmt.Sprintf("cdr-%s", activeChan.ChannelID),
			TenantID:        activeChan.TenantID,
			CampaignID:      activeChan.CampaignID,
			Phone:           activeChan.Phone,
			AgentID:         activeChan.AgentID,
			CallType:        activeChan.CallType,
			Disposition:     disposition,
			SIPStatus:       nil,
			HangupCause:     &causeInt,
			DurationSeconds: duration,
			BillsecSeconds:  billsec,
			RingSeconds:     ringSeconds,
			TrunkUsed:       activeChan.TrunkID,
			CreatedAt:       activeChan.StartedAt,
			InitiatedAt:     &activeChan.StartedAt,
			EndedAt:         &now,
		}
		_ = repRepo.SaveCDR(ctx, cdr)
	}

	if routRepo != nil && (activeChan.CallType == domain.CallTypePredictive || activeChan.CallType == domain.CallTypeManual) {
		_ = routRepo.SaveRouting(ctx, &domain.PhoneTrunkMapping{
			Phone:        activeChan.Phone,
			LastTrunk:    activeChan.TrunkID,
			LastProject:  activeChan.TenantID,
			LastSIPRoute: "",
			TenantID:     activeChan.TenantID,
			UpdatedAt:    time.Now(),
		})
	}

	tm.mu.RLock()
	notifier := tm.notifier
	tm.mu.RUnlock()

	if notifier != nil && activeChan.CallType == domain.CallTypeManual {
		notifier.DispatchManualHangup(activeChan, disposition, causeInt, duration, billsec, ringSeconds)
	}
}

func cleanTrunkID(raw string) string {
	trunkID := raw
	if strings.HasPrefix(trunkID, "sip:") {
		trunkID = strings.TrimPrefix(trunkID, "sip:")
	}
	if idx := strings.Index(trunkID, "/"); idx != -1 {
		trunkID = trunkID[:idx]
	}
	if idx := strings.Index(trunkID, "@"); idx != -1 {
		trunkID = trunkID[:idx]
	}
	if idx := strings.Index(trunkID, ":"); idx != -1 {
		trunkID = trunkID[:idx]
	}
	trunkID = strings.TrimSuffix(trunkID, "-aor")
	trunkID = strings.TrimSuffix(trunkID, "-reg")
	return trunkID
}

func (tm *TrunkManager) handleContactStatus(ctx context.Context, attrs map[string]string) {
	rawID := attrs["EndpointName"]
	if rawID == "" {
		rawID = attrs["AOR"]
	}
	trunkID := cleanTrunkID(rawID)
	if trunkID == "" {
		return
	}

	status := attrs["ContactStatus"] // Reachable, Unreachable, NonQualified, Unknown, Removed
	rttUS, _ := strconv.ParseFloat(attrs["RoundtripUsec"], 64)
	rttMS := rttUS / 1000.0

	healthStatus := "ONLINE"
	if status == "Unreachable" || status == "Removed" {
		healthStatus = "UNREACHABLE"
	} else if status == "Unknown" {
		healthStatus = "OFFLINE"
	}

	activeChans := tm.channels.GetTrunkActiveCount(trunkID)
	health := domain.TrunkHealth{
		Status:         healthStatus,
		LatencyMS:      rttMS,
		ActiveChannels: activeChans,
		LastQualifyAt:  time.Now(),
	}

	_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
}

func (tm *TrunkManager) handleRegistry(ctx context.Context, attrs map[string]string) {
	trunkID := cleanTrunkID(attrs["Username"])
	if trunkID == "" {
		return
	}
	state := attrs["Status"] // Registered, Rejected, Request Sent, Failed, Unregistered

	healthStatus := "REGISTERED"
	if state == "Rejected" {
		healthStatus = "REJECTED"
	} else if state == "Unregistered" || state == "Failed" {
		healthStatus = "OFFLINE"
	} else if state != "Registered" {
		healthStatus = state
	}

	activeChans := tm.channels.GetTrunkActiveCount(trunkID)
	health := domain.TrunkHealth{
		Status:         healthStatus,
		ActiveChannels: activeChans,
		LastQualifyAt:  time.Now(),
	}

	_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
}

func (tm *TrunkManager) handleUserEvent(ctx context.Context, attrs map[string]string) {
	userEvent := attrs["UserEvent"]
	channel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]
	phone := attrs["Phone"]
	campaignID := attrs["CampaignId"]
	leadID := attrs["LeadId"]

	tm.mu.RLock()
	pred := tm.predictive
	inb := tm.inbound
	tm.mu.RUnlock()

	switch userEvent {
	case "PredictiveHuman":
		if pred != nil {
			_ = pred.HandlePredictiveHuman(ctx, channel, uniqueID, phone, campaignID, leadID)
		}
	case "PredictiveAi":
		if pred != nil {
			_ = pred.HandlePredictiveAi(ctx, channel, uniqueID, phone, campaignID, leadID)
		}
	case "InboundCall":
		if inb != nil {
			callerPhone := attrs["CallerIDNum"]
			didNumber := attrs["Exten"]
			_ = inb.ProcessInboundCall(ctx, channel, callerPhone, didNumber)
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
