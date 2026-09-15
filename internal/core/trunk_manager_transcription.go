package core

import (
	"context"
	"log"
)

// handleTranscriptionUpdate centraliza a recepção de transcrição do Vosk em tempo real,
// atualizando a memória do ChannelManager e persistindo imediatamente na tabela cdrs.
//
// @pattern Observer / Pub-Sub (Streaming Real-Time Synchronization)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#triagem-ativa-full-duplex
//
// @preExecution
// - Validação de canal Asterisk e UniqueID
// - Atualização thread-safe do ActiveChannel em memória via `channels.SetTranscription`
//
// @postExecution
// - Atualização atômica assíncrona na tabela `cdrs` via `ReportRepo.UpdateCDRTranscription`
func (tm *TrunkManager) handleTranscriptionUpdate(ctx context.Context, astChannel, uniqueID, text string) {
	if text == "" {
		return
	}

	tm.channels.SetTranscription(astChannel, uniqueID, text)

	callID := tm.channels.GetCallIDByAsterisk(astChannel, uniqueID)
	if callID == "" {
		return
	}

	tm.mu.RLock()
	repRepo := tm.reportRepo
	tm.mu.RUnlock()

	if repRepo != nil {
		if err := repRepo.UpdateCDRTranscription(ctx, callID, text); err != nil {
			log.Printf("[TRUNK_MGR] Falha ao persistir transcrição em tempo real (callID=%s): %v", callID, err)
		}
	}
}
