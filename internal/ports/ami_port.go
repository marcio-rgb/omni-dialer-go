package ports

import "context"

// AMIEvent representa um pacote de evento emitido assincronamente pelo socket do Asterisk.
type AMIEvent struct {
	Name       string
	Attributes map[string]string
}

// AMIPort define os contratos de baixo nível para comunicação com o Asterisk PBX.
type AMIPort interface {
	Connect(ctx context.Context) error
	Close() error
	IsConnected() bool
	
	// Ações
	Originate(ctx context.Context, actionID, channel, context, exten string, priority int, timeout int, callerID, account string, variables map[string]string) error
	Redirect(ctx context.Context, actionID, channel, extraChannel, context, exten string, priority int) error
	Hangup(ctx context.Context, actionID, channel string, cause int) error
	SetVar(ctx context.Context, actionID, channel, variable, value string) error
	Command(ctx context.Context, actionID, command string) (string, error)
	
	// Stream de Eventos
	SubscribeEvents() <-chan AMIEvent
}
