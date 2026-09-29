package core

import (
	"context"
	"log"
	"sync"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// CDRPersisterWorker processa em background eventos de CDR desacoplados do tráfego telefônico via Redis Streams.
//
// @pattern Consumer Worker / Batch Processor
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type CDRPersisterWorker struct {
	cache   ports.CachePort
	repRepo ports.ReportRepository
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewCDRPersisterWorker instancia o worker de persistência de CDR.
func NewCDRPersisterWorker(cache ports.CachePort, repRepo ports.ReportRepository) *CDRPersisterWorker {
	return &CDRPersisterWorker{
		cache:   cache,
		repRepo: repRepo,
		stopCh:  make(chan struct{}),
	}
}

// StartDaemon inicia o loop consumidor de eventos do Redis Stream.
func (w *CDRPersisterWorker) StartDaemon(ctx context.Context) {
	if w.cache == nil || w.repRepo == nil {
		log.Println("[WARN] [CDR-WORKER] Cache ou Repositório nulos. Persister worker não iniciado.")
		return
	}

	w.wg.Add(1)
	go w.workerLoop(ctx)
	log.Println("[INFO] [CDR-WORKER] Daemon de persistência de CDR via Redis Streams inicializado.")
}

// Stop sinaliza a parada graciosa do worker.
func (w *CDRPersisterWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *CDRPersisterWorker) workerLoop(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		messages, err := w.cache.ReadCDREvents(ctx, "cg_cdr_persister", "worker-1", 50, 2*time.Second)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if len(messages) == 0 {
			continue
		}

		for _, msg := range messages {
			if msg == nil || msg.Event == nil {
				continue
			}

			if err := w.processEvent(ctx, msg.Event); err != nil {
				log.Printf("[ERROR] [CDR-WORKER] Falha ao persistir evento %s para chamada %s: %v", msg.Event.Type, msg.Event.CallID, err)
				_ = w.cache.SendCDRToDLQ(ctx, msg.Event, err.Error())
			}

			_ = w.cache.AckCDREvent(ctx, "cg_cdr_persister", msg.ID)
		}
	}
}

func (w *CDRPersisterWorker) processEvent(ctx context.Context, event *domain.CDREvent) error {
	return w.repRepo.SaveCDREvent(ctx, event)
}
