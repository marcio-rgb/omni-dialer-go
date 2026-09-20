package ami

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)


type AMIClient struct {
	host         string
	port         int
	username     string
	password     string
	conn         net.Conn
	reader       *bufio.Reader
	mu           sync.RWMutex
	writeMu      sync.Mutex
	connected    atomic.Bool
	stopCh       chan struct{}
	eventCh      chan ports.AMIEvent
	actionChans  map[string]chan *Message
	actionMu     sync.RWMutex
	reconnecting atomic.Bool
}

var _ ports.AMIPort = (*AMIClient)(nil)

func NewAMIClient(host string, port int, user, pass string) *AMIClient {
	return &AMIClient{
		host:        host,
		port:        port,
		username:    user,
		password:    pass,
		stopCh:      make(chan struct{}),
		eventCh:     make(chan ports.AMIEvent, 10000), // Buffer robusto para evitar gargalo
		actionChans: make(map[string]chan *Message),
	}
}

func (c *AMIClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("falha ao conectar no socket AMI (%s): %w", addr, err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.connected.Store(true)

	// Lê o banner do Asterisk (ex: Asterisk Call Manager/5.0.3)
	banner, err := c.reader.ReadString('\n')
	if err != nil {
		c.conn.Close()
		c.connected.Store(false)
		return fmt.Errorf("falha ao ler banner AMI: %w", err)
	}
	_ = banner

	// Inicia loop de escuta em goroutine
	go c.readLoop()

	// Efetua login
	if err := c.login(ctx); err != nil {
		c.Close()
		return fmt.Errorf("autenticação AMI falhou: %w", err)
	}

	return nil
}

func (c *AMIClient) login(ctx context.Context) error {
	actionID := fmt.Sprintf("login-%d", time.Now().UnixNano())
	payload := fmt.Sprintf("Action: Login\r\nUsername: %s\r\nSecret: %s\r\nActionID: %s\r\n\r\n",
		c.username, c.password, actionID)

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(payload); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" {
			return fmt.Errorf("rejeição no login: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando resposta de login AMI")
	}
}

func (c *AMIClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected.Swap(false) {
		return nil
	}

	close(c.stopCh)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *AMIClient) IsConnected() bool {
	return c.connected.Load()
}

func (c *AMIClient) SubscribeEvents() <-chan ports.AMIEvent {
	return c.eventCh
}

func (c *AMIClient) readLoop() {
	scanner := bufio.NewScanner(c.reader)
	for {
		select {
		case <-c.stopCh:
			return
		default:
			msg, err := ParsePacket(scanner)
			if err != nil || msg == nil {
				if c.connected.Load() {
					c.connected.Store(false)
					go c.tryReconnect()
				}
				return
			}

			if msg.Type == "Event" {
				c.dispatchRawEvent(msg)
			} else if msg.ActionID != "" {
				c.actionMu.RLock()
				ch, ok := c.actionChans[msg.ActionID]
				c.actionMu.RUnlock()
				if ok {
					select {
					case ch <- msg:
					default:
					}
				}
			}
		}
	}
}

func (c *AMIClient) tryReconnect() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return
	}
	defer c.reconnecting.Store(false)

	backoff := 500 * time.Millisecond
	for {
		select {
		case <-c.stopCh:
			return
		default:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.Connect(ctx)
			cancel()
			if err == nil {
				return
			}
			time.Sleep(backoff)
			if backoff < 10*time.Second {
				backoff *= 2
			}
		}
	}
}

// StartBackgroundReconnect inicia a rotina em segundo plano para tentar reconectar ao AMI de forma contínua.
func (c *AMIClient) StartBackgroundReconnect() {
	go c.tryReconnect()
}

func (c *AMIClient) dispatchRawEvent(msg *Message) {
	evt := ports.AMIEvent{
		Name:       msg.EventName,
		Attributes: msg.Attributes,
	}
	select {
	case c.eventCh <- evt:
	default:
		// Buffer cheio: descarta ou alerta para evitar bloqueio
	}
}

func (c *AMIClient) registerAction(actionID string) chan *Message {
	c.actionMu.Lock()
	defer c.actionMu.Unlock()
	ch := make(chan *Message, 1)
	c.actionChans[actionID] = ch
	return ch
}

func (c *AMIClient) unregisterAction(actionID string) {
	c.actionMu.Lock()
	defer c.actionMu.Unlock()
	delete(c.actionChans, actionID)
}

func (c *AMIClient) writeRaw(data string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if !c.connected.Load() || c.conn == nil {
		return fmt.Errorf("AMI socket não está conectado")
	}
	_, err := c.conn.Write([]byte(data))
	return err
}

func (c *AMIClient) Originate(ctx context.Context, actionID, channel, pbxContext, exten string, priority int, timeout int, callerID, account string, variables map[string]string) error {
	var sb strings.Builder
	sb.WriteString("Action: Originate\r\n")
	sb.WriteString(fmt.Sprintf("ActionID: %s\r\n", actionID))
	sb.WriteString(fmt.Sprintf("Channel: %s\r\n", channel))
	sb.WriteString(fmt.Sprintf("Context: %s\r\n", pbxContext))
	sb.WriteString(fmt.Sprintf("Exten: %s\r\n", exten))
	sb.WriteString(fmt.Sprintf("Priority: %d\r\n", priority))
	sb.WriteString(fmt.Sprintf("Timeout: %d\r\n", timeout*1000))
	if callerID != "" {
		sb.WriteString(fmt.Sprintf("CallerID: %s\r\n", callerID))
	}
	if account != "" {
		sb.WriteString(fmt.Sprintf("Account: %s\r\n", account))
	}
	sb.WriteString("Async: true\r\n")

	for k, v := range variables {
		sb.WriteString(fmt.Sprintf("Variable: %s=%s\r\n", k, v))
	}
	sb.WriteString("\r\n")

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(sb.String()); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" {
			return fmt.Errorf("erro no Originate: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de Originate")
	}
}

func (c *AMIClient) Redirect(ctx context.Context, actionID, channel, extraChannel, pbxContext, exten string, priority int) error {
	var sb strings.Builder
	sb.WriteString("Action: Redirect\r\n")
	sb.WriteString(fmt.Sprintf("ActionID: %s\r\n", actionID))
	sb.WriteString(fmt.Sprintf("Channel: %s\r\n", channel))
	if extraChannel != "" {
		sb.WriteString(fmt.Sprintf("ExtraChannel: %s\r\n", extraChannel))
	}
	sb.WriteString(fmt.Sprintf("Context: %s\r\n", pbxContext))
	sb.WriteString(fmt.Sprintf("Exten: %s\r\n", exten))
	sb.WriteString(fmt.Sprintf("Priority: %d\r\n\r\n", priority))

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(sb.String()); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" {
			return fmt.Errorf("erro no Redirect: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de Redirect")
	}
}

func sanitizeSIPHeaderValue(val string) string {
	clean := strings.ReplaceAll(strings.ReplaceAll(val, "\r", ""), "\n", " ")
	clean = strings.ReplaceAll(clean, "\"", "'")
	return strings.TrimSpace(clean)
}

// TransferToLiveKit transfere o canal do Asterisk para o LiveKit SIP via contexto cos-all-custom injetando variáveis de canal
func (c *AMIClient) TransferToLiveKit(ctx context.Context, channel string, agent *domain.AgentRedisData, customer *domain.CustomerMetadata) error {
	actionID := fmt.Sprintf("transfer-livekit-%d", time.Now().UnixNano())

	if agent != nil && agent.LiveKitRoom != "" {
		_ = c.SetVar(ctx, fmt.Sprintf("setvar-room-%s", actionID), channel, "AGENT_ROOM", sanitizeSIPHeaderValue(agent.LiveKitRoom))
	}
	if customer != nil {
		if customer.CustomerID != "" {
			_ = c.SetVar(ctx, fmt.Sprintf("setvar-cid-%s", actionID), channel, "CUSTOMER_ID", sanitizeSIPHeaderValue(customer.CustomerID))
		}
		if customer.Name != "" {
			_ = c.SetVar(ctx, fmt.Sprintf("setvar-cname-%s", actionID), channel, "CUSTOMER_NAME", sanitizeSIPHeaderValue(customer.Name))
		}
		if customer.Phone != "" {
			_ = c.SetVar(ctx, fmt.Sprintf("setvar-cphone-%s", actionID), channel, "CUSTOMER_PHONE", sanitizeSIPHeaderValue(customer.Phone))
		}
		if customer.Att1 != "" {
			_ = c.SetVar(ctx, fmt.Sprintf("setvar-catt1-%s", actionID), channel, "CUSTOMER_ATT1", sanitizeSIPHeaderValue(customer.Att1))
		}
		if customer.Att2 != "" {
			_ = c.SetVar(ctx, fmt.Sprintf("setvar-catt2-%s", actionID), channel, "CUSTOMER_ATT2", sanitizeSIPHeaderValue(customer.Att2))
		}
		if customer.Att3 != "" {
			_ = c.SetVar(ctx, fmt.Sprintf("setvar-catt3-%s", actionID), channel, "CUSTOMER_ATT3", sanitizeSIPHeaderValue(customer.Att3))
		}
	}

	return c.Redirect(ctx, actionID, channel, "", "cos-all-custom", "9999", 1)
}


func (c *AMIClient) Hangup(ctx context.Context, actionID, channel string, cause int) error {
	payload := fmt.Sprintf("Action: Hangup\r\nActionID: %s\r\nChannel: %s\r\nCause: %d\r\n\r\n",
		actionID, channel, cause)

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(payload); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" {
			return fmt.Errorf("erro no Hangup: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de Hangup")
	}
}

func (c *AMIClient) SetVar(ctx context.Context, actionID, channel, variable, value string) error {
	payload := fmt.Sprintf("Action: Setvar\r\nActionID: %s\r\nChannel: %s\r\nVariable: %s\r\nValue: %s\r\n\r\n",
		actionID, channel, variable, value)

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(payload); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" {
			return fmt.Errorf("erro no Setvar: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de Setvar")
	}
}

func (c *AMIClient) Command(ctx context.Context, actionID, command string) (string, error) {
	payload := fmt.Sprintf("Action: Command\r\nActionID: %s\r\nCommand: %s\r\n\r\n",
		actionID, command)

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(payload); err != nil {
		return "", err
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-ch:
		return resp.Attributes["Output"], nil
	case <-time.After(5 * time.Second):
		return "", fmt.Errorf("timeout aguardando saída do comando AMI")
	}
}
