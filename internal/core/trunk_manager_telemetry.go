package core

import (
	"context"
	"strconv"
	"strings"
	"time"

	"dialer-go/internal/domain"
)

// cleanTrunkID normaliza identificadores de troncos SIP/PJSIP.
func cleanTrunkID(raw string) string {
	trunkID := raw
	if strings.HasPrefix(trunkID, "sip:") {
		trunkID = strings.TrimPrefix(trunkID, "sip:")
	}
	if idx := strings.Index(trunkID, "/"); idx != -1 {
		trunkID = trunkID[:idx]
	}
	if idx := strings.Index(trunkID, "@"); idx != -1 {
		trunkID = trunkID[:idx]
	}
	if idx := strings.Index(trunkID, ":"); idx != -1 {
		trunkID = trunkID[:idx]
	}
	trunkID = strings.TrimSuffix(trunkID, "-aor")
	trunkID = strings.TrimSuffix(trunkID, "-reg")
	return trunkID
}

// handleContactStatus atualiza a telemetria de latência (RTT) e saúde dos endpoints PJSIP.
//
// @pattern Observer / Telemetry Sink
// @governedBy docs/rules/TELEPHONY_POLICIES.md#telemetria-de-troncos
func (tm *TrunkManager) handleContactStatus(ctx context.Context, attrs map[string]string) {
	rawID := attrs["EndpointName"]
	if rawID == "" {
		rawID = attrs["AOR"]
	}
	trunkID := cleanTrunkID(rawID)
	if trunkID == "" {
		return
	}

	status := attrs["ContactStatus"] // Reachable, Unreachable, NonQualified, Unknown, Removed
	rttUS, _ := strconv.ParseFloat(attrs["RoundtripUsec"], 64)
	rttMS := rttUS / 1000.0

	healthStatus := "ONLINE"
	if status == "Unreachable" || status == "Removed" {
		healthStatus = "UNREACHABLE"
	} else if status == "Unknown" {
		healthStatus = "OFFLINE"
	}

	activeChans := tm.channels.GetTrunkActiveCount(trunkID)
	health := domain.TrunkHealth{
		Status:         healthStatus,
		LatencyMS:      rttMS,
		ActiveChannels: activeChans,
		LastQualifyAt:  time.Now(),
	}

	_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
}

// handleRegistry rastreia o estado de registro SIP de troncos autenticados com operadoras.
//
// @pattern Observer / Telemetry Sink
func (tm *TrunkManager) handleRegistry(ctx context.Context, attrs map[string]string) {
	trunkID := cleanTrunkID(attrs["Username"])
	if trunkID == "" {
		return
	}
	state := attrs["Status"] // Registered, Rejected, Request Sent, Failed, Unregistered

	healthStatus := "REGISTERED"
	if state == "Rejected" {
		healthStatus = "REJECTED"
	} else if state == "Unregistered" || state == "Failed" {
		healthStatus = "OFFLINE"
	} else if state != "Registered" {
		healthStatus = state
	}

	activeChans := tm.channels.GetTrunkActiveCount(trunkID)
	health := domain.TrunkHealth{
		Status:         healthStatus,
		ActiveChannels: activeChans,
		LastQualifyAt:  time.Now(),
	}

	_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
}
