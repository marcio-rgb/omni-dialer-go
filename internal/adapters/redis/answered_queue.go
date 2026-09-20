package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dialer-go/internal/domain"
)

// PushAnsweredLead serializa e enfileira o evento de chamada atendida por humano nas filas Redis.
//
// @pattern Adapter (Redis Cache / Queue)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
//
// @preExecution
// - Validar existência de evento e cliente Redis
//
// @postExecution
// - RPush atômico na fila global `dialer:answered_leads`
// - RPush opcional na fila específica `dialer:answered_leads:<campaign_id>`
func (r *RedisAdapter) PushAnsweredLead(ctx context.Context, event *domain.AnsweredLeadEvent) error {
	if r == nil || r.client == nil || event == nil {
		return nil
	}

	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}
	if event.Event == "" {
		event.Event = "call.answered"
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("redis: falha ao serializar answered lead event: %w", err)
	}

	pipe := r.client.Pipeline()
	// 1. Fila global de leads atendidos
	pipe.RPush(ctx, "dialer:answered_leads", data)

	// 2. Fila por campanha (se informada)
	if event.CampaignID != "" {
		pipe.RPush(ctx, fmt.Sprintf("dialer:answered_leads:%s", event.CampaignID), data)
	}

	_, err = pipe.Exec(ctx)
	return err
}
