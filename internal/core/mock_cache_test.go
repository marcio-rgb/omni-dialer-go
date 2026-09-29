package core

import (
	"context"
	"time"

	"dialer-go/internal/domain"
)

func (m *mockCachePredictive) PublishCDREvent(ctx context.Context, event *domain.CDREvent) error {
	return nil
}
func (m *mockCachePredictive) ReadCDREvents(ctx context.Context, group, consumer string, count int64, block time.Duration) ([]*domain.CDREventMessage, error) {
	return nil, nil
}
func (m *mockCachePredictive) AckCDREvent(ctx context.Context, group string, id string) error {
	return nil
}
func (m *mockCachePredictive) SendCDRToDLQ(ctx context.Context, event *domain.CDREvent, reason string) error {
	return nil
}
func (m *mockCachePredictive) EnqueueTranscriptionJob(ctx context.Context, job *domain.TranscriptionJob) error {
	return nil
}
func (m *mockCachePredictive) ReadTranscriptionJobs(ctx context.Context, group, consumer string, count int64, block time.Duration) ([]*domain.TranscriptionJobMessage, error) {
	return nil, nil
}
func (m *mockCachePredictive) AckTranscriptionJob(ctx context.Context, group string, id string) error {
	return nil
}
func (m *mockCachePredictive) SetCallMetadata(ctx context.Context, callUUID string, meta domain.CallMetadata, ttl time.Duration) error {
	return nil
}
func (m *mockCachePredictive) GetCallMetadata(ctx context.Context, callUUID string) (*domain.CallMetadata, error) {
	return nil, nil
}
