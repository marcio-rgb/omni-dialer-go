package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// ChannelManager arbitra concorrentemente o uso da infraestrutura de telecomunicação.
type ChannelManager struct {
	maxGlobalChannels int
	humanReserveQuota int
	activeGlobalCalls atomic.Int32
	activeHumanCalls  atomic.Int32

	trunkLimits map[string]int
	trunkActive map[string]*atomic.Int32
	trunkMu     sync.RWMutex

	activeChannels map[string]*domain.ActiveChannel
	astToCall      map[string]string
	chanMu         sync.RWMutex

	cache ports.CachePort
}

func NewChannelManager(maxGlobal, humanReserve int, cache ports.CachePort) *ChannelManager {
	return &ChannelManager{
		maxGlobalChannels: maxGlobal,
		humanReserveQuota: humanReserve,
		trunkLimits:       make(map[string]int),
		trunkActive:       make(map[string]*atomic.Int32),
		activeChannels:    make(map[string]*domain.ActiveChannel),
		astToCall:         make(map[string]string),
		cache:             cache,
	}
}

// RegisterTrunkLimit registra ou atualiza o limite de canais simultâneos de um tronco.
func (cm *ChannelManager) RegisterTrunkLimit(trunkID string, maxChannels int) {
	cm.trunkMu.Lock()
	defer cm.trunkMu.Unlock()
	cm.trunkLimits[trunkID] = maxChannels
	if _, ok := cm.trunkActive[trunkID]; !ok {
		cm.trunkActive[trunkID] = &atomic.Int32{}
	}
}

// UnregisterTrunk remove o tronco do controle em memória.
func (cm *ChannelManager) UnregisterTrunk(trunkID string) {
	cm.trunkMu.Lock()
	defer cm.trunkMu.Unlock()
	delete(cm.trunkLimits, trunkID)
	delete(cm.trunkActive, trunkID)
}

// CanAcquireSlot avalia se a chamada pode ser originada respeitando a hierarquia de dois níveis.
func (cm *ChannelManager) CanAcquireSlot(trunkID string, isHuman bool) (bool, string) {
	// Nível 1: Teto Global do PBX
	currGlobal := int(cm.activeGlobalCalls.Load())
	if currGlobal >= cm.maxGlobalChannels {
		return false, "GLOBAL_CAPACITY_EXHAUSTED"
	}

	// Reserva para Humanos: se a chamada for virtual/IA, ela não pode invadir a cota humana reservada
	if !isHuman {
		availableForVirtual := cm.maxGlobalChannels - cm.humanReserveQuota
		if currGlobal >= availableForVirtual {
			return false, "HUMAN_RESERVED_QUOTA_PROTECTION"
		}
	}

	// Nível 2: Limite Granular do Tronco
	cm.trunkMu.RLock()
	limit, hasLimit := cm.trunkLimits[trunkID]
	activeCounter, hasCounter := cm.trunkActive[trunkID]
	cm.trunkMu.RUnlock()

	if hasLimit && hasCounter {
		currTrunk := int(activeCounter.Load())
		if currTrunk >= limit {
			return false, "TRUNK_CHANNELS_EXHAUSTED"
		}
	}

	return true, ""
}

// AcquireSlot aloca atomicamente os slots nos contadores globais e por tronco.
func (cm *ChannelManager) AcquireSlot(ctx context.Context, channel *domain.ActiveChannel, isHuman bool) error {
	can, reason := cm.CanAcquireSlot(channel.TrunkID, isHuman)
	if !can {
		return fmt.Errorf("rejeição de capacidade: %s", reason)
	}

	cm.trunkMu.RLock()
	activeCounter := cm.trunkActive[channel.TrunkID]
	cm.trunkMu.RUnlock()

	cm.activeGlobalCalls.Add(1)
	if isHuman {
		cm.activeHumanCalls.Add(1)
	}
	if activeCounter != nil {
		activeCounter.Add(1)
	}

	cm.chanMu.Lock()
	cm.activeChannels[channel.ChannelID] = channel
	cm.chanMu.Unlock()

	if cm.cache != nil {
		_ = cm.cache.IncrementTrunkChannel(ctx, channel.TrunkID, channel.ChannelID)
	}

	return nil
}

// LinkAsteriskChannel associa o canal Asterisk e o UniqueID ao callID em andamento.
func (cm *ChannelManager) LinkAsteriskChannel(callID, astChannel, uniqueID string) {
	if callID == "" {
		return
	}
	cm.chanMu.Lock()
	defer cm.chanMu.Unlock()
	if astChannel != "" {
		cm.astToCall[astChannel] = callID
	}
	if uniqueID != "" {
		cm.astToCall[uniqueID] = callID
	}
}

// GetCallIDByAsterisk busca o callID associado a um canal Asterisk ou UniqueID.
func (cm *ChannelManager) GetCallIDByAsterisk(astChannel, uniqueID string) string {
	cm.chanMu.RLock()
	defer cm.chanMu.RUnlock()
	if uniqueID != "" {
		if id, ok := cm.astToCall[uniqueID]; ok {
			return id
		}
	}
	if astChannel != "" {
		if id, ok := cm.astToCall[astChannel]; ok {
			return id
		}
	}
	return ""
}

// AssignAgent associa um operador humano a uma chamada ativa em andamento e promove para cota humana.
func (cm *ChannelManager) AssignAgent(callID, agentID string) {
	if callID == "" || agentID == "" {
		return
	}
	cm.chanMu.Lock()
	ch, exists := cm.activeChannels[callID]
	if exists {
		wasHuman := ch.CallType == domain.CallTypeManual || ch.AgentID != nil
		ch.AgentID = &agentID
		ch.IsAnswered = true
		if !wasHuman {
			cm.activeHumanCalls.Add(1)
		}
	}
	cm.chanMu.Unlock()
}

// SetCallDisposition define a disposição explícita da chamada antes do término.
func (cm *ChannelManager) SetCallDisposition(callID string, disp domain.CallDisposition) {
	if callID == "" {
		return
	}
	cm.chanMu.Lock()
	if ch, exists := cm.activeChannels[callID]; exists {
		ch.Disposition = &disp
	}
	cm.chanMu.Unlock()
}

// ReleaseByAsterisk desaloca o slot associado a um canal Asterisk ou UniqueID e retorna o canal encerrado.
func (cm *ChannelManager) ReleaseByAsterisk(ctx context.Context, astChannel, uniqueID string) *domain.ActiveChannel {
	callID := cm.GetCallIDByAsterisk(astChannel, uniqueID)
	if callID != "" {
		return cm.ReleaseSlot(ctx, callID)
	}
	return nil
}

// ReleaseSlot desaloca atomicamente o canal quando o Hangup for recebido e retorna o canal encerrado.
func (cm *ChannelManager) ReleaseSlot(ctx context.Context, channelID string) *domain.ActiveChannel {
	cm.chanMu.Lock()
	channel, exists := cm.activeChannels[channelID]
	if exists {
		delete(cm.activeChannels, channelID)
		for k, v := range cm.astToCall {
			if v == channelID {
				delete(cm.astToCall, k)
			}
		}
	}
	cm.chanMu.Unlock()

	if !exists {
		return nil
	}

	// Decrementa com proteção para não negativar
	for {
		oldVal := cm.activeGlobalCalls.Load()
		if oldVal <= 0 {
			break
		}
		if cm.activeGlobalCalls.CompareAndSwap(oldVal, oldVal-1) {
			break
		}
	}

	if channel.CallType == domain.CallTypeManual || channel.AgentID != nil {
		for {
			oldVal := cm.activeHumanCalls.Load()
			if oldVal <= 0 {
				break
			}
			if cm.activeHumanCalls.CompareAndSwap(oldVal, oldVal-1) {
				break
			}
		}
	}

	cm.trunkMu.RLock()
	activeCounter := cm.trunkActive[channel.TrunkID]
	cm.trunkMu.RUnlock()

	if activeCounter != nil {
		for {
			oldVal := activeCounter.Load()
			if oldVal <= 0 {
				break
			}
			if activeCounter.CompareAndSwap(oldVal, oldVal-1) {
				break
			}
		}
	}

	if cm.cache != nil {
		_ = cm.cache.DecrementTrunkChannel(ctx, channel.TrunkID, channelID)
	}
	return channel
}

// ReconcileCounters reconcilia contadores em memória com os canais ativos reais.
func (cm *ChannelManager) ReconcileCounters(ctx context.Context) {
	cm.chanMu.RLock()
	defer cm.chanMu.RUnlock()

	trunkCounts := make(map[string]int32)
	var globalCount int32
	var humanCount int32

	for _, ch := range cm.activeChannels {
		trunkCounts[ch.TrunkID]++
		globalCount++
		if ch.CallType == domain.CallTypeManual || ch.AgentID != nil {
			humanCount++
		}
	}

	cm.activeGlobalCalls.Store(globalCount)
	cm.activeHumanCalls.Store(humanCount)

	cm.trunkMu.Lock()
	defer cm.trunkMu.Unlock()
	for trunkID, counter := range cm.trunkActive {
		cnt := trunkCounts[trunkID]
		counter.Store(cnt)
	}
}

// GetGlobalStats retorna o snapshot atual de capacidade.
func (cm *ChannelManager) GetGlobalStats() (globalActive, humanActive, maxGlobal, humanQuota int) {
	return int(cm.activeGlobalCalls.Load()),
		int(cm.activeHumanCalls.Load()),
		cm.maxGlobalChannels,
		cm.humanReserveQuota
}

// GetTrunkActiveCount retorna a contagem ativa em tempo real de um tronco.
func (cm *ChannelManager) GetTrunkActiveCount(trunkID string) int {
	cm.trunkMu.RLock()
	defer cm.trunkMu.RUnlock()
	if counter, ok := cm.trunkActive[trunkID]; ok {
		return int(counter.Load())
	}
	return 0
}
