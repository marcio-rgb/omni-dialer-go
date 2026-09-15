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
	NotifyCallEnded(ctx context.Context, webhookURL string, payload *domain.CallEndedWebhookPayload) error

	// NotifyInjectLead dispara o webhook GET assíncrono para o tenant ao conectar a chamada com os dados do lead.
	NotifyInjectLead(ctx context.Context, webhookURL string, params *domain.InjectLeadParams) error
}
