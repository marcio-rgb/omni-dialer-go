package ports

import (
	"context"

	"dialer-go/internal/domain"
)

// WebhookPort define o contrato de transporte para envio de notificações de telefonia via Webhook HTTP.
//
// @pattern Adapter (Port)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#3-notificacoes-assincronas-e-webhooks
type WebhookPort interface {
	// NotifyCallEnded dispara a notificação de encerramento da perna telefônica com o desfecho da operadora.
	//
	// @pattern Adapter (Method)
	// @governedBy docs/rules/TELEPHONY_POLICIES.md#3-notificacoes-assincronas-e-webhooks
	//
	// @preExecution
	// - Validar URL de destino não vazia e payload válido com CallID e TenantID
	//
	// @postExecution
	// - Enviar requisição HTTP POST com timeout curto e headers de rastreabilidade
	NotifyCallEnded(ctx context.Context, webhookURL string, payload *domain.CallEndedWebhookPayload) error
}
