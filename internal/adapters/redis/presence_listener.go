package redis

import (
	"context"
	"encoding/json"
	"log"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

// StartPresenceListener escuta canais Pub/Sub do Redis para inserção e remoção automática de operadores em filas.
//
// @pattern Observer / Pub-Sub (Redis Presence Event Stream)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (r *RedisAdapter) StartPresenceListener(ctx context.Context, queueMgr *core.AgentQueueManager) {
	if queueMgr == nil {
		return
	}

	channels := []string{
		"dialer:agent_presence",
		"dialer:queue:member",
		"agent:presence",
		"agent_presence",
	}

	go func() {
		pubsub := r.client.Subscribe(ctx, channels...)
		defer pubsub.Close()

		log.Printf("[INFO] [REDIS-PUBSUB] Escutando eventos de presença de operadores nos canais: %v", channels)
		ch := pubsub.Channel()

		for {
			select {
			case <-ctx.Done():
				log.Println("[INFO] [REDIS-PUBSUB] Encerrando listener de presença Redis.")
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg.Payload == "" {
					continue
				}

				var event domain.QueuePresenceEvent
				if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
					log.Printf("[WARN] [REDIS-PUBSUB] Erro ao deserializar evento no canal %s: %v (payload: %s)", msg.Channel, err, msg.Payload)
					continue
				}

				log.Printf("[INFO] [REDIS-PUBSUB] Evento de presença recebido: ação=%s, agente=%s, campanha=%s", event.Action, event.AgentID, event.CampaignID)
				if err := queueMgr.ProcessPresenceEvent(ctx, &event); err != nil {
					log.Printf("[ERROR] [REDIS-PUBSUB] Falha ao processar evento de presença: %v", err)
				}
			}
		}
	}()
}
