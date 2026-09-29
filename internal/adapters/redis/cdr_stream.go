package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	StreamCDREvents = "dialer:stream:cdr_events"
	StreamCDRDLQ    = "dialer:stream:cdr_dlq"
	GroupCDREvents  = "cg_cdr_persister"
)

// PublishCDREvent enfileira atômica e imediatamente um evento de CDR no Redis Stream.
//
// @pattern Adapter (Redis Stream Event Publisher)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
// @preExecution Validação de cliente Redis e payload do evento
// @postExecution Ingestão com XADD no stream dialer:stream:cdr_events (< 0.5ms)
func (r *RedisAdapter) PublishCDREvent(ctx context.Context, event *domain.CDREvent) error {
	if r == nil || r.client == nil || event == nil {
		return nil
	}

	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("redis: erro ao serializar CDREvent: %w", err)
	}

	args := &redis.XAddArgs{
		Stream: StreamCDREvents,
		MaxLen: 100000,
		Approx: true,
		Values: map[string]interface{}{
			"call_id":   event.CallID,
			"type":      string(event.Type),
			"tenant_id": event.TenantID,
			"payload":   string(data),
		},
	}

	return r.client.XAdd(ctx, args).Err()
}

// ReadCDREvents consome lotes de eventos do stream via Consumer Group com garantia At-Least-Once.
//
// @pattern Adapter (Consumer Group Reader)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (r *RedisAdapter) ReadCDREvents(ctx context.Context, group, consumer string, count int64, block time.Duration) ([]*domain.CDREventMessage, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("redis: cliente não inicializado")
	}

	if group == "" {
		group = GroupCDREvents
	}
	if consumer == "" {
		consumer = "persister-1"
	}
	if count <= 0 {
		count = 50
	}

	// Garante que o stream e o consumer group existam
	_ = r.ensureStreamGroup(ctx, StreamCDREvents, group)

	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{StreamCDREvents, ">"},
		Count:    count,
		Block:    block,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var messages []*domain.CDREventMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			payloadStr, ok := msg.Values["payload"].(string)
			if !ok {
				continue
			}

			var event domain.CDREvent
			if err := json.Unmarshal([]byte(payloadStr), &event); err != nil {
				continue
			}

			messages = append(messages, &domain.CDREventMessage{
				ID:    msg.ID,
				Event: &event,
			})
		}
	}

	return messages, nil
}

// AckCDREvent confirma o processamento bem-sucedido de uma mensagem no stream.
func (r *RedisAdapter) AckCDREvent(ctx context.Context, group, id string) error {
	if r == nil || r.client == nil || id == "" {
		return nil
	}
	if group == "" {
		group = GroupCDREvents
	}
	return r.client.XAck(ctx, StreamCDREvents, group, id).Err()
}

// SendCDRToDLQ move um evento com falha irrecuperável para a Dead Letter Queue.
func (r *RedisAdapter) SendCDRToDLQ(ctx context.Context, event *domain.CDREvent, reason string) error {
	if r == nil || r.client == nil || event == nil {
		return nil
	}

	data, _ := json.Marshal(event)
	args := &redis.XAddArgs{
		Stream: StreamCDRDLQ,
		MaxLen: 50000,
		Approx: true,
		Values: map[string]interface{}{
			"call_id":   event.CallID,
			"type":      string(event.Type),
			"tenant_id": event.TenantID,
			"reason":    reason,
			"payload":   string(data),
			"failed_at": time.Now().Format(time.RFC3339),
		},
	}
	return r.client.XAdd(ctx, args).Err()
}

func (r *RedisAdapter) ensureStreamGroup(ctx context.Context, stream, group string) error {
	err := r.client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}
