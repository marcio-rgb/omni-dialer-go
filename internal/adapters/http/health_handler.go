package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	pool     *pgxpool.Pool
	cache    ports.CachePort
	ami      ports.AMIPort
	channels *core.ChannelManager
}

func NewHealthHandler(pool *pgxpool.Pool, cache ports.CachePort, ami ports.AMIPort, channels *core.ChannelManager) *HealthHandler {
	return &HealthHandler{
		pool:     pool,
		cache:    cache,
		ami:      ami,
		channels: channels,
	}
}

func (h *HealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	amiOK := h.ami != nil && h.ami.IsConnected()
	dbOK := h.pool != nil && h.pool.Ping(ctx) == nil
	redisOK := false
	if h.cache != nil {
		_, err := h.cache.GetWhitelistIPs(ctx)
		redisOK = err == nil
	}

	activeGlob, activeHum, maxGlob, humQuota := 0, 0, 0, 0
	if h.channels != nil {
		activeGlob, activeHum, maxGlob, humQuota = h.channels.GetGlobalStats()
	}

	allOK := amiOK && dbOK && redisOK
	statusStr := "healthy"
	httpCode := http.StatusOK
	if !allOK {
		statusStr = "degraded"
		httpCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": statusStr,
		"components": map[string]bool{
			"asterisk_ami": amiOK,
			"database":     dbOK,
			"cache":        redisOK,
		},
		"telephony_capacity": map[string]int{
			"active_global_channels": activeGlob,
			"active_human_channels":  activeHum,
			"max_global_channels":    maxGlob,
			"human_reserved_quota":   humQuota,
			"available_channels":     maxGlob - activeGlob,
		},
	})
}
