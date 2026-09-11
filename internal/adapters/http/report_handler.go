package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type ReportHandler struct {
	repo  ports.ReportRepository
	cache ports.CachePort
}

func NewReportHandler(repo ports.ReportRepository, cache ports.CachePort) *ReportHandler {
	return &ReportHandler{
		repo:  repo,
		cache: cache,
	}
}

// GetCallsSummary manipula requisições GET /api/v1/reports/calls-summary com buffer estrito de 15 minutos
func (h *ReportHandler) GetCallsSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Extrai tenant_id obrigatório (query param ou header)
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}

	if tenantID == "" {
		errResp := domain.NewErrBadRequest("MISSING_TENANT_ID",
			"O parâmetro obrigatório 'tenant_id' não foi informado via query parameter (?tenant_id=...) nem via header HTTP (X-Tenant-Id).")
		errResp.WriteJSON(w)
		return
	}

	// 2. Extrai e normaliza start_date e end_date opcionais
	var startDate, endDate time.Time
	var err error

	startDateStr := r.URL.Query().Get("start_date")
	if startDateStr != "" {
		startDate, err = time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			errResp := domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'start_date' deve ser ISO 8601 UTC (ex: 2026-09-09T00:00:00Z)",
				domain.InvalidParam{Name: "start_date", Reason: "formato inválido"})
			errResp.WriteJSON(w)
			return
		}
	} else {
		// Default: Início do dia atual 00:00:00 UTC
		now := time.Now().UTC()
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	endDateStr := r.URL.Query().Get("end_date")
	if endDateStr != "" {
		endDate, err = time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			errResp := domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'end_date' deve ser ISO 8601 UTC (ex: 2026-09-09T06:30:00Z)",
				domain.InvalidParam{Name: "end_date", Reason: "formato inválido"})
			errResp.WriteJSON(w)
			return
		}
	} else {
		// Default: Instante atual
		endDate = time.Now().UTC()
	}

	if startDate.After(endDate) {
		errResp := domain.NewErrUnprocessable("INVALID_DATE_RANGE",
			fmt.Sprintf("A data inicial 'start_date' (%s) não pode ser posterior à data final 'end_date' (%s).",
				startDate.Format(time.RFC3339), endDate.Format(time.RFC3339)),
			domain.InvalidParam{Name: "start_date", Reason: "posterior a end_date"})
		errResp.WriteJSON(w)
		return
	}

	var campaignIDPtr *string
	campIDStr := r.URL.Query().Get("campaign_id")
	if campIDStr != "" {
		campaignIDPtr = &campIDStr
	}

	// 3. Gera hash determinístico dos parâmetros para o buffer
	campKey := "all"
	if campaignIDPtr != nil {
		campKey = *campaignIDPtr
	}
	rawHash := fmt.Sprintf("%s|%s|%s", startDate.Format(time.RFC3339), endDate.Format(time.RFC3339), campKey)
	hasher := sha256.New()
	hasher.Write([]byte(rawHash))
	hashKey := hex.EncodeToString(hasher.Sum(nil))

	// 4. Consulta o buffer no Redis (15 minutos = 900s)
	cachedData, ttlRemaining, err := h.cache.GetCallsSummaryBuffer(ctx, tenantID, hashKey)
	if err == nil && cachedData != nil {
		// CACHE HIT: Entrega instantânea da memória (< 1ms)
		cachedData.CacheMeta.Cached = true
		cachedData.CacheMeta.TTLRemainingSeconds = int64(ttlRemaining.Seconds())
		cachedData.CacheMeta.LastRefreshReason = "BUFFER_HIT"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    cachedData,
		})
		return
	}

	// 5. CACHE MISS: Consulta o PostgreSQL dialer_db com agregação em passada única
	freshData, err := h.repo.GetCallsSummary(ctx, tenantID, startDate, endDate, campaignIDPtr)
	if err != nil {
		errResp := domain.NewErrInternal(fmt.Sprintf("Falha ao processar agregação analítica no banco: %s", err.Error()))
		errResp.WriteJSON(w)
		return
	}

	// 6. Preenche metadados do buffer e salva no Redis com TTL = 900s (15 min)
	now := time.Now().UTC()
	bufferTTL := 900 * time.Second
	freshData.CacheMeta = domain.CacheMetaDTO{
		Cached:                 false,
		CachedAt:               now.Format(time.RFC3339),
		CacheExpiresAt:         now.Add(bufferTTL).Format(time.RFC3339),
		TTLRemainingSeconds:    900,
		BufferDurationSeconds:  900,
		RefreshIntervalMinutes: 15,
		LastRefreshReason:      "FRESH_CALCULATION",
	}

	_ = h.cache.SetCallsSummaryBuffer(ctx, tenantID, hashKey, freshData, bufferTTL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    freshData,
	})
}
