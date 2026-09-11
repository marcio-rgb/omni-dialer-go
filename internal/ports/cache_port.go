package ports

import (
	"context"
	"dialer-go/internal/domain"
	"time"
)

type CachePort interface {
	// Whitelist em Cache
	GetWhitelistIPs(ctx context.Context) ([]string, error)
	AddWhitelistIP(ctx context.Context, ip string) error

	// Troncos & Telemetria
	SetTrunkHealth(ctx context.Context, trunkID string, health domain.TrunkHealth, ttl time.Duration) error
	GetTrunkHealth(ctx context.Context, trunkID string) (*domain.TrunkHealth, error)

	// Fila de Leads & Pacing
	PushLeads(ctx context.Context, campaignID string, phoneList []string) error
	PopLead(ctx context.Context, campaignID string) (string, error)
	GetQueueLength(ctx context.Context, campaignID string) (int64, error)
	
	// Trava de Campanha (Toggle)
	SetCampaignPaused(ctx context.Context, campaignID string, paused bool) error
	IsCampaignPaused(ctx context.Context, campaignID string) (bool, error)

	// Flag Pós-Abandono (< 2s)
	SetInflatedSuccessRate(ctx context.Context, ttl time.Duration) error
	HasInflatedSuccessRate(ctx context.Context) (bool, error)

	// Canais Ativos Granulares
	IncrementTrunkChannel(ctx context.Context, trunkID, channelID string) error
	DecrementTrunkChannel(ctx context.Context, trunkID, channelID string) error
	GetTrunkActiveChannelsCount(ctx context.Context, trunkID string) (int, error)

	// Buffer Analítico de 15 Minutos (900s)
	GetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string) (*domain.CallsSummaryResponse, time.Duration, error)
	SetCallsSummaryBuffer(ctx context.Context, tenantID, hashKey string, data *domain.CallsSummaryResponse, ttl time.Duration) error

	// Operadores Disponíveis para Entrega Preditiva
	StoreAvailableAgents(ctx context.Context, campaignID string, agents []domain.AgentDemandDTO, ttl time.Duration) error
	GetNextAvailableAgent(ctx context.Context, campaignID string) (*domain.AgentDemandDTO, error)
}
