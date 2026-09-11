package ami

import (
	"bufio"
	"strings"
)

// Message representa um pacote AMI bruto (Action Response ou Event).
type Message struct {
	Type       string // "Response" ou "Event"
	ActionID   string
	EventName  string
	Attributes map[string]string
}

// ParsePacket lê um pacote completo delimitado por linha vazia a partir do scanner.
func ParsePacket(scanner *bufio.Scanner) (*Message, error) {
	attrs := make(map[string]string)
	hasData := false

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if line == "" {
			if hasData {
				break
			}
			continue
		}

		hasData = true
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if existing, ok := attrs[key]; ok && key == "Output" {
				attrs[key] = existing + "\n" + val
			} else {
				attrs[key] = val
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if !hasData {
		return nil, nil
	}

	msg := &Message{
		Attributes: attrs,
	}

	if evt, ok := attrs["Event"]; ok {
		msg.Type = "Event"
		msg.EventName = evt
	} else if resp, ok := attrs["Response"]; ok {
		msg.Type = "Response"
		msg.EventName = resp
	}

	if actionID, ok := attrs["ActionID"]; ok {
		msg.ActionID = actionID
	}

	return msg, nil
}
