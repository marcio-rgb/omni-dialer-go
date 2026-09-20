package core

import (
	"dialer-go/internal/domain"
)

// SetTranscription atualiza o texto transcrito em tempo real associado ao canal ativo.
//
// @pattern Observer / Pub-Sub (Live State Synchronization)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#triagem-ativa-full-duplex
func (cm *ChannelManager) SetTranscription(astChannel, uniqueID, transcription string) {
	if transcription == "" {
		return
	}
	callID := cm.GetCallIDByAsterisk(astChannel, uniqueID)
	if callID == "" {
		return
	}
	cm.chanMu.Lock()
	defer cm.chanMu.Unlock()
	if ch, exists := cm.activeChannels[callID]; exists {
		ch.Transcription = transcription
	}
}

// GetTranscription retorna a transcrição acumulada até o momento para o canal especificado.
func (cm *ChannelManager) GetTranscription(callID string) string {
	if callID == "" {
		return ""
	}
	cm.chanMu.RLock()
	defer cm.chanMu.RUnlock()
	if ch, exists := cm.activeChannels[callID]; exists {
		return ch.Transcription
	}
	return ""
}

// GetActiveChannel retorna o canal ativo registrado em memória pelo callID.
func (cm *ChannelManager) GetActiveChannel(callID string) *domain.ActiveChannel {
	if callID == "" {
		return nil
	}
	cm.chanMu.RLock()
	defer cm.chanMu.RUnlock()
	return cm.activeChannels[callID]
}

// GetActiveChannelByAsterisk busca o canal ativo a partir do canal Asterisk ou UniqueID.
func (cm *ChannelManager) GetActiveChannelByAsterisk(astChannel, uniqueID string) *domain.ActiveChannel {
	callID := cm.GetCallIDByAsterisk(astChannel, uniqueID)
	if callID == "" {
		return nil
	}
	return cm.GetActiveChannel(callID)
}
