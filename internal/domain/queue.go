package domain

import "strings"

// QueueMemberRequest representa o payload para adição de um atendente à fila Asterisk/Redis
type QueueMemberRequest struct {
	TenantID    string `json:"tenant_id"`
	CampaignID  string `json:"campaign_id"`
	AgentID     string `json:"agent_id"`
	LiveKitRoom string `json:"livekit_room,omitempty"`
	SIPRoute    string `json:"sip_route,omitempty"`
	Penalty     int    `json:"penalty,omitempty"`
	Paused      bool   `json:"paused,omitempty"`
}

// QueueMemberRemoveRequest representa o payload para remoção de um atendente da fila
type QueueMemberRemoveRequest struct {
	TenantID    string `json:"tenant_id,omitempty"`
	CampaignID  string `json:"campaign_id"`
	AgentID     string `json:"agent_id"`
	LiveKitRoom string `json:"livekit_room,omitempty"`
}

// QueueMemberPauseRequest representa o payload para pausa/despausa de um membro da fila
type QueueMemberPauseRequest struct {
	TenantID    string `json:"tenant_id,omitempty"`
	CampaignID  string `json:"campaign_id"`
	AgentID     string `json:"agent_id"`
	LiveKitRoom string `json:"livekit_room,omitempty"`
	Paused      bool   `json:"paused"`
	Reason      string `json:"reason,omitempty"`
}

// QueuePresenceEvent representa o payload de evento unificado de presença de fila
type QueuePresenceEvent struct {
	Action      string `json:"action"` // "add", "join", "online", "ready", "available", "pause", "unpause", "resume", "leave", "offline", "remove"
	TenantID    string `json:"tenant_id,omitempty"`
	CampaignID  string `json:"campaign_id"`
	AgentID     string `json:"agent_id"`
	LiveKitRoom string `json:"livekit_room,omitempty"`
	SIPRoute    string `json:"sip_route,omitempty"`
	Penalty     int    `json:"penalty,omitempty"`
	Paused      bool   `json:"paused,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// HangupCallRequest representa a requisição explícita de encerramento de chamada
type HangupCallRequest struct {
	CallID  string `json:"call_id,omitempty"`
	Channel string `json:"channel,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Cause   int    `json:"cause,omitempty"` // Código Q.850 (Padrão 16: Normal Clearing)
}

// HangupCallResponse representa o retorno da solicitação de encerramento
type HangupCallResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	CallID  string `json:"call_id,omitempty"`
	Channel string `json:"channel,omitempty"`
}

// FormatQueueName padroniza o nome da fila Asterisk como trim(tenant_id)-trim(campaign_id).
// Se tenant_id for omitido, assume "default" para manter paridade absoluta entre engine e controllers.
func FormatQueueName(tenantID, campaignID string) string {
	t := strings.TrimSpace(tenantID)
	if t == "" {
		t = "default"
	}
	c := strings.TrimSpace(campaignID)
	if c == "" {
		return "dialer-default-queue"
	}
	if strings.HasPrefix(c, "q_") {
		c = strings.TrimPrefix(c, "q_")
	}
	return t + "-" + c
}
