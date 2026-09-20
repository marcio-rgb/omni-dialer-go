package ami

import (
	"bufio"
	"strings"
	"testing"
)

func TestParsePacket_Response(t *testing.T) {
	raw := "Response: Success\r\nActionID: login-123\r\nMessage: Authentication accepted\r\n\r\n"
	scanner := bufio.NewScanner(strings.NewReader(raw))

	msg, err := ParsePacket(scanner)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if msg == nil {
		t.Fatal("mensagem esperada não foi nula")
	}

	if msg.Type != "Response" {
		t.Errorf("tipo esperado Response, obtido %s", msg.Type)
	}

	if msg.ActionID != "login-123" {
		t.Errorf("ActionID esperado login-123, obtido %s", msg.ActionID)
	}

	if msg.Attributes["Message"] != "Authentication accepted" {
		t.Errorf("Message inesperada: %s", msg.Attributes["Message"])
	}
}

func TestParsePacket_Event(t *testing.T) {
	raw := "Event: ContactStatus\r\nPrivilege: system,all\r\nAOR: trunk_vivo_e1\r\nContactStatus: Reachable\r\nRoundtripUsec: 14200\r\n\r\n"
	scanner := bufio.NewScanner(strings.NewReader(raw))

	msg, err := ParsePacket(scanner)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if msg == nil {
		t.Fatal("mensagem esperada não foi nula")
	}

	if msg.Type != "Event" {
		t.Errorf("tipo esperado Event, obtido %s", msg.Type)
	}

	if msg.EventName != "ContactStatus" {
		t.Errorf("EventName esperado ContactStatus, obtido %s", msg.EventName)
	}

	if msg.Attributes["RoundtripUsec"] != "14200" {
		t.Errorf("RoundtripUsec esperado 14200, obtido %s", msg.Attributes["RoundtripUsec"])
	}
}
