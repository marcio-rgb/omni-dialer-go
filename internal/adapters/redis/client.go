package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"

	"github.com/redis/go-redis/v9"
)

type RedisAdapter struct {
	client *redis.Client
}

var _ ports.CachePort = (*RedisAdapter)(nil)

func NewRedisAdapter(addr, password string, db int) (*RedisAdapter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     100,
		MinIdleConns: 10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("falha ao conectar no redis: %w", err)
	}

	return &RedisAdapter{client: client}, nil
}

func (r *RedisAdapter) GetWhitelistIPs(ctx context.Context) ([]string, error) {
	return r.client.SMembers(ctx, "dialer:whitelist_ips").Result()
}

func (r *RedisAdapter) AddWhitelistIP(ctx context.Context, ip string) error {
	return r.client.SAdd(ctx, "dialer:whitelist_ips", ip).Err()
}

func (r *RedisAdapter) SetTrunkHealth(ctx context.Context, trunkID string, health domain.TrunkHealth, ttl time.Duration) error {
	data, err := json.Marshal(health)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("dialer:trunks:health:%s", trunkID)
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisAdapter) GetTrunkHealth(ctx context.Context, trunkID string) (*domain.TrunkHealth, error) {
	key := fmt.Sprintf("dialer:trunks:health:%s", trunkID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var health domain.TrunkHealth
	if err := json.Unmarshal([]byte(val), &health); err != nil {
		return nil, err
	}
	return &health, nil
}

func (r *RedisAdapter) PushLeads(ctx context.Context, campaignID string, phoneList []string) error {
	if len(phoneList) == 0 {
		return nil
	}
	key := fmt.Sprintf("dialer:lead_queue:%s", campaignID)
	values := make([]interface{}, len(phoneList))
	for i, v := range phoneList {
		values[i] = v
	}
	return r.client.RPush(ctx, key, values...).Err()
}

func (r *RedisAdapter) PopLead(ctx context.Context, campaignID string) (string, error) {
	key := fmt.Sprintf("dialer:lead_queue:%s", campaignID)
	val, err := r.client.LPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (r *RedisAdapter) GetQueueLength(ctx context.Context, campaignID string) (int64, error) {
	key := fmt.Sprintf("dialer:lead_queue:%s", campaignID)
	return r.client.LLen(ctx, key).Result()
}

func (r *RedisAdapter) SetCampaignPaused(ctx context.Context, campaignID string, paused bool) error {
	key := fmt.Sprintf("dialer:campaign_paused:%s", campaignID)
	if paused {
		return r.client.Set(ctx, key, "1", 0).Err()
	}
	return r.client.Del(ctx, key).Err()
}

func (r *RedisAdapter) IsCampaignPaused(ctx context.Context, campaignID string) (bool, error) {
	key := fmt.Sprintf("dialer:campaign_paused:%s", campaignID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "1", nil
}

func (r *RedisAdapter) SetInflatedSuccessRate(ctx context.Context, ttl time.Duration) error {
	return r.client.Set(ctx, "dialer:inflated_success_rate", "1.0", ttl).Err()
}

func (r *RedisAdapter) HasInflatedSuccessRate(ctx context.Context) (bool, error) {
	val, err := r.client.Get(ctx, "dialer:inflated_success_rate").Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "1.0", nil
}

func (r *RedisAdapter) IncrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	key := fmt.Sprintf("dialer:active_channels_by_trunk:%s", trunkID)
	return r.client.SAdd(ctx, key, channelID).Err()
}

func (r *RedisAdapter) DecrementTrunkChannel(ctx context.Context, trunkID, channelID string) error {
	key := fmt.Sprintf("dialer:active_channels_by_trunk:%s", trunkID)
	return r.client.SRem(ctx, key, channelID).Err()
}

func (r *RedisAdapter) GetTrunkActiveChannelsCount(ctx context.Context, trunkID string) (int, error) {
	key := fmt.Sprintf("dialer:active_channels_by_trunk:%s", trunkID)
	count, err := r.client.SCard(ctx, key).Result()
	return int(count), err
}

// GetCallsSummaryBuffer busca o relatório em cache e calcula o TTL restante exato
func (r *RedisAdapter) GetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string) (*domain.CallsSummaryResponse, time.Duration, error) {
	key := fmt.Sprintf("dialer:buffer:calls_summary:%s:%s", tenantID, hashKey)
	pipe := r.client.Pipeline()
	getCmd := pipe.Get(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)
	_, err := pipe.Exec(ctx)

	if err == redis.Nil || getCmd.Val() == "" {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	var resp domain.CallsSummaryResponse
	if err := json.Unmarshal([]byte(getCmd.Val()), &resp); err != nil {
		return nil, 0, err
	}

	ttl := ttlCmd.Val()
	return &resp, ttl, nil
}

// SetCallsSummaryBuffer armazena o relatório com TTL rígido de 900s (15 min)
func (r *RedisAdapter) SetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string, data *domain.CallsSummaryResponse, ttl time.Duration) error {
	key := fmt.Sprintf("dialer:buffer:calls_summary:%s:%s", tenantID, hashKey)
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, bytes, ttl).Err()
}

// StoreAvailableAgents armazena a lista de operadores disponíveis ordenados para a campanha
func (r *RedisAdapter) StoreAvailableAgents(ctx context.Context, campaignID string, agents []domain.AgentDemandDTO, ttl time.Duration) error {
	key := fmt.Sprintf("dialer:available_agents:%s", campaignID)
	if len(agents) == 0 {
		return r.client.Del(ctx, key).Err()
	}
	bytes, err := json.Marshal(agents)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, bytes, ttl).Err()
}

// GetNextAvailableAgent recupera e remove o próximo operador disponível (mais ocioso) para entrega da chamada
func (r *RedisAdapter) GetNextAvailableAgent(ctx context.Context, campaignID string) (*domain.AgentDemandDTO, error) {
	key := fmt.Sprintf("dialer:available_agents:%s", campaignID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil || val == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var agents []domain.AgentDemandDTO
	if err := json.Unmarshal([]byte(val), &agents); err != nil {
		return nil, err
	}

	if len(agents) == 0 {
		return nil, nil
	}

	selected := agents[0]
	remaining := agents[1:]

	if len(remaining) > 0 {
		bytes, _ := json.Marshal(remaining)
		ttl, _ := r.client.TTL(ctx, key).Result()
		if ttl > 0 {
			_ = r.client.Set(ctx, key, bytes, ttl).Err()
		}
	} else {
		_ = r.client.Del(ctx, key).Err()
	}

	return &selected, nil
}

// PopIdleAgent remove e retorna o próximo operador ocioso da fila global dialer:idle_agents
func (r *RedisAdapter) PopIdleAgent(ctx context.Context, timeout time.Duration) (*domain.AgentRedisData, error) {
	key := "dialer:idle_agents"
	if timeout > 0 {
		res, err := r.client.BLPop(ctx, timeout, key).Result()
		if err == redis.Nil || len(res) < 2 {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var agent domain.AgentRedisData
		if err := json.Unmarshal([]byte(res[1]), &agent); err != nil {
			return nil, err
		}
		return &agent, nil
	}

	val, err := r.client.LPop(ctx, key).Result()
	if err == redis.Nil || val == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var agent domain.AgentRedisData
	if err := json.Unmarshal([]byte(val), &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// PushIdleAgent adiciona um operador ocioso ao final da fila global dialer:idle_agents
func (r *RedisAdapter) PushIdleAgent(ctx context.Context, agent *domain.AgentRedisData) error {
	if agent == nil {
		return nil
	}
	key := "dialer:idle_agents"
	data, err := json.Marshal(agent)
	if err != nil {
		return err
	}
	return r.client.RPush(ctx, key, data).Err()
}


