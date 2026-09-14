package core

import (
	"context"
	"log"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// CallNotifier orquestra o despacho de notificações de desfecho de chamadas para sistemas upstream.
//
// @pattern Observer / Dispatcher
// @governedBy docs/rules/TELEPHONY_POLICIES.md#3-notificacoes-assincronas-e-webhooks
type CallNotifier struct {
	webhookClient ports.WebhookPort
	defaultURL    string
}

func NewCallNotifier(webhookClient ports.WebhookPort, defaultURL string) *CallNotifier {
	return &CallNotifier{
		webhookClient: webhookClient,
		defaultURL:    defaultURL,
	}
}

// ResolveHangupReason traduz causas ISDN/Q.850 e desfechos em mensagens amigáveis em português.
func ResolveHangupReason(disposition domain.CallDisposition, causeInt int, isAnswered bool) string {
	if isAnswered {
		return "Chamada Atendida e Encerrada"
	}

	switch disposition {
	case domain.DispositionBusy:
		return "Número Ocupado"
	case domain.DispositionNoAnswer:
		return "Não Atende / Tempo Esgotado"
	case domain.DispositionCongestion:
		return "Circuito / Tronco Congestionado"
	case domain.DispositionInvalidNumber:
		return "Número Inexistente ou Inválido"
	case domain.DispositionCancelled:
		return "Cancelada pelo Operador"
	case domain.DispositionVoicemail, domain.DispositionAMDMachine:
		return "Caixa Postal / Atendimento Automático"
	case domain.DispositionTrunkCapacity:
		return "Capacidade de Tronco Esgotada"
	default:
		switch causeInt {
		case 17:
			return "Número Ocupado"
		case 19:
			return "Não Atende / Tempo Esgotado"
		case 34:
			return "Circuito / Tronco Congestionado"
		case 1, 28:
			return "Número Inexistente ou Inválido"
		default:
			return "Falha na Ligação / Desconectado pela Operadora"
		}
	}
}

// DispatchCallEnded processa o término de qualquer chamada (MANUAL ou PREDICTIVE) e despacha webhook com a URL da gravação.
func (cn *CallNotifier) DispatchCallEnded(activeChan *domain.ActiveChannel, disposition domain.CallDisposition, causeInt int, duration, billsec, ringSeconds int, recordingURL string) {
	if cn == nil || cn.webhookClient == nil {
		return
	}
	if activeChan == nil {
		return
	}

	now := time.Now()
	reason := ResolveHangupReason(disposition, causeInt, activeChan.IsAnswered)

	eventName := "telephony.call_ended"
	if activeChan.CallType == domain.CallTypeManual {
		eventName = "telephony.manual_call_failed"
		if activeChan.IsAnswered {
			eventName = "telephony.manual_call_ended"
		}
	}

	targetURL := cn.defaultURL
	if activeChan.WebhookURL != nil && *activeChan.WebhookURL != "" {
		targetURL = *activeChan.WebhookURL
	}
	if targetURL == "" {
		return // Nenhuma URL de webhook configurada. Nada é direcionado para fora.
	}

	payload := &domain.CallEndedWebhookPayload{
		Event:           eventName,
		CallID:          activeChan.ChannelID,
		CallType:        activeChan.CallType,
		TenantID:        activeChan.TenantID,
		AgentID:         activeChan.AgentID,
		Phone:           activeChan.Phone,
		TrunkUsed:       activeChan.TrunkID,
		Disposition:     disposition,
		HangupCause:     causeInt,
		HangupReason:    reason,
		IsAnswered:      activeChan.IsAnswered,
		DurationSeconds: duration,
		BillsecSeconds:  billsec,
		RingSeconds:     ringSeconds,
		StartedAt:       activeChan.StartedAt,
		EndedAt:         now,
		RecordingURL:    recordingURL,
		Timestamp:       now.Unix(),
	}

	// Executa em goroutine sem bloquear o socket AMI ou a liberação de canais
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()

		if err := cn.webhookClient.NotifyCallEnded(ctx, targetURL, payload); err != nil {
			log.Printf("[WARN] [NOTIFIER] Erro ao notificar término da chamada %s (%s): %v", payload.CallID, payload.CallType, err)
		}
	}()
}

// DispatchManualHangup processa o término de uma chamada manual e envia webhook se houver falha ou encerramento.
//
// @pattern Strategy (Manual Call Notification)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#3-notificacoes-assincronas-e-webhooks
//
// @preExecution
// - Validar existência de webhookClient e dados do canal ativo
//
// @postExecution
// - Disparar goroutine assíncrona desacoplada do loop de sinalização telefônica
func (cn *CallNotifier) DispatchManualHangup(activeChan *domain.ActiveChannel, disposition domain.CallDisposition, causeInt int, duration, billsec, ringSeconds int) {
	if cn == nil || cn.webhookClient == nil {
		return
	}
	if activeChan == nil || activeChan.CallType != domain.CallTypeManual {
		return
	}

	now := time.Now()
	reason := ResolveHangupReason(disposition, causeInt, activeChan.IsAnswered)

	eventName := "telephony.manual_call_failed"
	if activeChan.IsAnswered {
		eventName = "telephony.manual_call_ended"
	}

	targetURL := cn.defaultURL
	if activeChan.WebhookURL != nil && *activeChan.WebhookURL != "" {
		targetURL = *activeChan.WebhookURL
	}
	if targetURL == "" {
		return // Nenhuma URL de webhook configurada. Nada é direcionado para fora.
	}

	payload := &domain.CallEndedWebhookPayload{
		Event:           eventName,
		CallID:          activeChan.ChannelID,
		CallType:        activeChan.CallType,
		TenantID:        activeChan.TenantID,
		AgentID:         activeChan.AgentID,
		Phone:           activeChan.Phone,
		TrunkUsed:       activeChan.TrunkID,
		Disposition:     disposition,
		HangupCause:     causeInt,
		HangupReason:    reason,
		IsAnswered:      activeChan.IsAnswered,
		DurationSeconds: duration,
		BillsecSeconds:  billsec,
		RingSeconds:     ringSeconds,
		StartedAt:       activeChan.StartedAt,
		EndedAt:         now,
		Timestamp:       now.Unix(),
	}

	// Executa em goroutine sem bloquear o socket AMI ou a liberação de canais
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()

		if err := cn.webhookClient.NotifyCallEnded(ctx, targetURL, payload); err != nil {
			log.Printf("[WARN] [NOTIFIER] Erro ao notificar término da chamada manual %s: %v", payload.CallID, err)
		}
	}()
}
