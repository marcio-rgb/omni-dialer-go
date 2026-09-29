package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	StreamTranscriptionJobs = "dialer:stream:transcription_jobs"
	GroupTranscriptionJobs  = "cg_transcription_workers"
)

// EnqueueTranscriptionJob enfileira um job de transcrição assíncrona pós-chamada no Redis Stream.
//
// @pattern Adapter (Redis Stream Job Producer)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (r *RedisAdapter) EnqueueTranscriptionJob(ctx context.Context, job *domain.TranscriptionJob) error {
	if r == nil || r.client == nil || job == nil {
		return nil
	}

	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now()
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("redis: falha ao serializar TranscriptionJob: %w", err)
	}

	args := &redis.XAddArgs{
		Stream: StreamTranscriptionJobs,
		MaxLen: 50000,
		Approx: true,
		Values: map[string]interface{}{
			"call_id":   job.CallID,
			"tenant_id": job.TenantID,
			"payload":   string(data),
		},
	}

	return r.client.XAdd(ctx, args).Err()
}

// ReadTranscriptionJobs consome jobs de transcrição pendentes via Consumer Group.
//
// @pattern Adapter (Consumer Group Reader)
func (r *RedisAdapter) ReadTranscriptionJobs(ctx context.Context, group, consumer string, count int64, block time.Duration) ([]*domain.TranscriptionJobMessage, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("redis: cliente não inicializado")
	}

	if group == "" {
		group = GroupTranscriptionJobs
	}
	if consumer == "" {
		consumer = "whisper-worker-1"
	}
	if count <= 0 {
		count = 10
	}

	_ = r.ensureStreamGroup(ctx, StreamTranscriptionJobs, group)

	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{StreamTranscriptionJobs, ">"},
		Count:    count,
		Block:    block,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var messages []*domain.TranscriptionJobMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			payloadStr, ok := msg.Values["payload"].(string)
			if !ok {
				continue
			}

			var job domain.TranscriptionJob
			if err := json.Unmarshal([]byte(payloadStr), &job); err != nil {
				continue
			}

			messages = append(messages, &domain.TranscriptionJobMessage{
				ID:  msg.ID,
				Job: &job,
			})
		}
	}

	return messages, nil
}

// AckTranscriptionJob confirma o término do processamento de transcrição.
func (r *RedisAdapter) AckTranscriptionJob(ctx context.Context, group, id string) error {
	if r == nil || r.client == nil || id == "" {
		return nil
	}
	if group == "" {
		group = GroupTranscriptionJobs
	}
	return r.client.XAck(ctx, StreamTranscriptionJobs, group, id).Err()
}
