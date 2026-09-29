package core

import (
	"strings"
)

// cleanAgentID extrai e higieniza o identificador/UUID único do operador telefônico.
// Remove sufixos como /n, canais locais Local/sala_agente_... e prefixos de interfaces.
//
// @pattern Utility / Sanitizer
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func cleanAgentID(member string) string {
	clean := strings.TrimSpace(member)
	if clean == "" {
		return ""
	}

	// Remove sufixos de dialplan Asterisk como /n ou /nj
	if idx := strings.LastIndex(clean, "/n"); idx != -1 && idx == len(clean)-2 {
		clean = clean[:idx]
	}

	// Se tiver formato de contexto Asterisk (ex: Local/sala_agente_UUID@from-internal)
	if strings.Contains(clean, "@") {
		clean = strings.Split(clean, "@")[0]
	}

	// Se tiver formato com barra (ex: PJSIP/UUID ou Local/UUID)
	if strings.Contains(clean, "/") {
		parts := strings.Split(clean, "/")
		clean = parts[len(parts)-1]
	}

	// Remove prefixos legados de salas do Omnichat ou agentes
	clean = strings.TrimPrefix(clean, "sala_agente_")
	clean = strings.TrimPrefix(clean, "agent_")

	return strings.TrimSpace(clean)
}

// formatAgentRoom retorna o nome canônico da sala LiveKit para o operador (sala_agente_<uuid>).
func formatAgentRoom(agentID, liveKitRoom, sipRoute string) string {
	room := strings.TrimSpace(liveKitRoom)
	if room == "" && strings.Contains(sipRoute, "sip:") {
		parts := strings.Split(sipRoute, "sip:")
		if len(parts) > 1 {
			sub := strings.Split(parts[1], "@")[0]
			if sub != "" {
				room = strings.TrimSpace(sub)
			}
		}
	}
	if room == "" && sipRoute != "" && !strings.HasPrefix(sipRoute, "PJSIP/") {
		room = strings.TrimSpace(sipRoute)
	}
	if room == "" {
		room = strings.TrimSpace(agentID)
	}
	if room == "" {
		return ""
	}

	// Se já possuir prefixo sala_ ou sala_agente_
	if strings.HasPrefix(room, "sala_agente_") || strings.HasPrefix(room, "sala_") {
		return room
	}
	return "sala_agente_" + room
}

// formatAgentQueueInterface retorna o formato canônico de interface para o Asterisk app_queue:
// Local/sala_agente_<uuid>@livekit-agent-queue/n
func formatAgentQueueInterface(agentID, liveKitRoom, sipRoute string) string {
	room := formatAgentRoom(agentID, liveKitRoom, sipRoute)
	if room == "" {
		return ""
	}
	return "Local/" + room + "@livekit-agent-queue/n"
}
