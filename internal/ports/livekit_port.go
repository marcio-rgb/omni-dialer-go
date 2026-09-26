package ports

import (
	"context"
	"time"
)

// LiveKitPort define o contrato canônico para orquestração, auto-provisionamento
// e diagnóstico de integridade da infraestrutura LiveKit SIP.
//
// @pattern Port (Hexagonal Architecture)
// @governedBy .agents/ARCHITECT.md#livekit-integration
//
// @preExecution
// - Contexto com cancelamento/timeout configurado
// - Credenciais de API (API Key e Secret) devidamente inicializadas
//
// @postExecution
// - Garante existência idempotente de Inbound Trunk e Dispatch Rule
// - Provê telemetria do estado da ponte LiveKit SIP para o Healthcheck
type LiveKitPort interface {
	// EnsureSIPTrunkAndRule valida e provisiona idempotentemente o tronco SIP de entrada
	// e a regra de despacho no LiveKit caso não existam.
	EnsureSIPTrunkAndRule(ctx context.Context) error

	// CheckSIPHealth verifica se a ponte SIP do LiveKit possui ao menos 1 tronco e 1 regra ativos.
	CheckSIPHealth(ctx context.Context) (bool, error)

	// StartReconciler inicia loop contínuo de reconciliação em background para auto-healing da ponte SIP.
	StartReconciler(ctx context.Context, interval time.Duration)
}
