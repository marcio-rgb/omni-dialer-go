package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ResetConsecutiveErrors zera o contador de erros consecutivos de uma campanha no Redis.
//
// @pattern Adapter (Redis Cache)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (r *RedisAdapter) ResetConsecutiveErrors(ctx context.Context, campaignID string) error {
	if r == nil || r.client == nil {
		return nil
	}
	key := fmt.Sprintf("dialer:consecutive_errors:%s", campaignID)
	return r.client.Del(ctx, key).Err()
}

// IncrementConsecutiveErrors incrementa atomicamente o contador de falhas/erros de rota de uma campanha.
//
// @pattern Adapter (Redis Cache)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (r *RedisAdapter) IncrementConsecutiveErrors(ctx context.Context, campaignID string) (int64, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}
	key := fmt.Sprintf("dialer:consecutive_errors:%s", campaignID)
	val, err := r.client.Incr(ctx, key).Result()
	if err == nil {
		// Define expiração de 10 minutos para não reter erros obsoletos
		_ = r.client.Expire(ctx, key, 10*time.Minute).Err()
	}
	return val, err
}

// GetConsecutiveErrors recupera a quantidade atual de falhas consecutivas de uma campanha.
//
// @pattern Adapter (Redis Cache)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
func (r *RedisAdapter) GetConsecutiveErrors(ctx context.Context, campaignID string) (int64, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}
	key := fmt.Sprintf("dialer:consecutive_errors:%s", campaignID)
	val, err := r.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}
