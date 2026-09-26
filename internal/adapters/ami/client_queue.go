package ami

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// QueueAdd adiciona dinamicamente um membro/operador à fila do Asterisk.
//
// @pattern Adapter (AMI Client)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (c *AMIClient) QueueAdd(ctx context.Context, actionID, queue, iface, memberName string, penalty int, paused bool) error {
	pausedStr := "0"
	if paused {
		pausedStr = "1"
	}

	var sb strings.Builder
	sb.WriteString("Action: QueueAdd\r\n")
	sb.WriteString(fmt.Sprintf("ActionID: %s\r\n", actionID))
	sb.WriteString(fmt.Sprintf("Queue: %s\r\n", queue))
	sb.WriteString(fmt.Sprintf("Interface: %s\r\n", iface))
	if memberName != "" {
		sb.WriteString(fmt.Sprintf("MemberName: %s\r\n", memberName))
	}
	if penalty > 0 {
		sb.WriteString(fmt.Sprintf("Penalty: %d\r\n", penalty))
	}
	sb.WriteString(fmt.Sprintf("Paused: %s\r\n\r\n", pausedStr))

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(sb.String()); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" && !strings.Contains(resp.Attributes["Message"], "already exists") {
			return fmt.Errorf("erro no QueueAdd: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de QueueAdd")
	}
}

// QueueRemove remove dinamicamente um membro da fila do Asterisk.
//
// @pattern Adapter (AMI Client)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (c *AMIClient) QueueRemove(ctx context.Context, actionID, queue, iface string) error {
	var sb strings.Builder
	sb.WriteString("Action: QueueRemove\r\n")
	sb.WriteString(fmt.Sprintf("ActionID: %s\r\n", actionID))
	sb.WriteString(fmt.Sprintf("Queue: %s\r\n", queue))
	sb.WriteString(fmt.Sprintf("Interface: %s\r\n\r\n", iface))

	ch := c.registerAction(actionID)
	defer c.unregisterAction(actionID)

	if err := c.writeRaw(sb.String()); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Attributes["Response"] != "Success" && !strings.Contains(resp.Attributes["Message"], "not found") {
			return fmt.Errorf("erro no QueueRemove: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de QueueRemove")
	}
}

// QueuePause pausa ou retoma o atendimento de um membro na fila do Asterisk.
//
// @pattern Adapter (AMI Client)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (c *AMIClient) QueuePause(ctx context.Context, actionID, queue, iface string, paused bool, reason string) error {
	pausedStr := "0"
	if paused {
		pausedStr = "1"
	}

	var sb strings.Builder
	sb.WriteString("Action: QueuePause\r\n")
	sb.WriteString(fmt.Sprintf("ActionID: %s\r\n", actionID))
	if queue != "" {
		sb.WriteString(fmt.Sprintf("Queue: %s\r\n", queue))
	}
	sb.WriteString(fmt.Sprintf("Interface: %s\r\n", iface))
	sb.WriteString(fmt.Sprintf("Paused: %s\r\n", pausedStr))
	if reason != "" {
		sb.WriteString(fmt.Sprintf("Reason: %s\r\n", reason))
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
			return fmt.Errorf("erro no QueuePause: %s", resp.Attributes["Message"])
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout aguardando confirmação de QueuePause")
	}
}
