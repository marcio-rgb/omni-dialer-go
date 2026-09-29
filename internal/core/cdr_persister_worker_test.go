package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

type mockPersisterCache struct {
	mockCachePredictive
	mu       sync.Mutex
	events   []*domain.CDREventMessage
	ackedIDs []string
	dlqCount int
}

func (m *mockPersisterCache) ReadCDREvents(ctx context.Context, group, consumer string, count int64, block time.Duration) ([]*domain.CDREventMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) > 0 {
		batch := m.events
		m.events = nil
		return batch, nil
	}
	return nil, nil
}

func (m *mockPersisterCache) AckCDREvent(ctx context.Context, group string, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ackedIDs = append(m.ackedIDs, id)
	return nil
}

func (m *mockPersisterCache) SendCDRToDLQ(ctx context.Context, event *domain.CDREvent, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dlqCount++
	return nil
}

type mockPersisterRepo struct {
	mockReportRepoForTM
	mu           sync.Mutex
	savedEvents  []*domain.CDREvent
}

func (m *mockPersisterRepo) SaveCDREvent(ctx context.Context, event *domain.CDREvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedEvents = append(m.savedEvents, event)
	return nil
}

func TestCDRPersisterWorker_ProcessStreamBatch(t *testing.T) {
	cache := &mockPersisterCache{
		events: []*domain.CDREventMessage{
			{
				ID: "msg-101",
				Event: &domain.CDREvent{
					CallID:      "pred-12345",
					TenantID:    "tenant-alpha",
					Phone:       "11988887777",
					CallType:    domain.CallTypePredictive,
					Disposition: domain.DispositionAnswered,
					Type:        domain.CDREventAnswered,
					Timestamp:   time.Now().Unix(),
				},
			},
		},
	}
	repo := &mockPersisterRepo{}

	worker := NewCDRPersisterWorker(cache, repo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.StartDaemon(ctx)
	time.Sleep(100 * time.Millisecond)
	worker.Stop()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.savedEvents) != 1 {
		t.Fatalf("Esperava 1 evento salvo no repositório, obteve %d", len(repo.savedEvents))
	}

	if repo.savedEvents[0].CallID != "pred-12345" {
		t.Errorf("CallID incorreto: %s", repo.savedEvents[0].CallID)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.ackedIDs) != 1 || cache.ackedIDs[0] != "msg-101" {
		t.Errorf("Mensagem msg-101 não recebeu ACK esperado: %v", cache.ackedIDs)
	}
}
