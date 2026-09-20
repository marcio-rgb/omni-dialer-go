package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dialer-go/internal/domain"
)

// PollAsteriskEndpoints consulta ativamente o estado de todos os endpoints e registros PJSIP no Asterisk.
//
// @pattern Adapter (AMI Telemetry Poller)
// @governedBy docs/rules/TRUNKS_LIFECYCLE.md#3-hot-reload-a-quente-no-asterisk-pbx
//
// @preExecution
// - Disparo de comandos AMI `pjsip show endpoints` e `pjsip show registrations`
//
// @postExecution
// - Parsing dos estados (`ONLINE`, `REGISTERED`, `UNREACHABLE`, `REJECTED`, `OFFLINE`)
// - Atualização de latência RTT em milissegundos
// - Persistência do estado em cache Redis com TTL de 24h
func (tm *TrunkManager) PollAsteriskEndpoints(ctx context.Context) {
	actID1 := fmt.Sprintf("poll-ep-%d", time.Now().UnixNano())
	outEp, _ := tm.ami.Command(ctx, actID1, "pjsip show endpoints")
	if outEp != "" {
		lines := strings.Split(outEp, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Endpoint:") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					trunkID := cleanTrunkID(parts[1])
					if trunkID == "" {
						continue
					}
					status := "OFFLINE"
					if strings.Contains(line, "Not in use") || strings.Contains(line, "In use") {
						status = "ONLINE"
					} else if strings.Contains(line, "Unavailable") {
						status = "UNREACHABLE"
					}
					existing, _ := tm.cache.GetTrunkHealth(ctx, trunkID)
					if existing != nil && existing.Status == "REGISTERED" && status == "ONLINE" {
						status = "REGISTERED"
					}
					health := domain.TrunkHealth{
						Status:         status,
						ActiveChannels: tm.channels.GetTrunkActiveCount(trunkID),
						LastQualifyAt:  time.Now(),
					}
					if existing != nil && existing.LatencyMS > 0 {
						health.LatencyMS = existing.LatencyMS
					}
					_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
				}
			} else if strings.Contains(line, "Contact:") && strings.Contains(line, "Avail") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 5 {
					trunkID := cleanTrunkID(parts[1])
					if trunkID != "" {
						rttStr := parts[len(parts)-1]
						rtt, _ := strconv.ParseFloat(rttStr, 64)
						existing, _ := tm.cache.GetTrunkHealth(ctx, trunkID)
						status := "ONLINE"
						if existing != nil && existing.Status == "REGISTERED" {
							status = "REGISTERED"
						}
						health := domain.TrunkHealth{
							Status:         status,
							LatencyMS:      rtt,
							ActiveChannels: tm.channels.GetTrunkActiveCount(trunkID),
							LastQualifyAt:  time.Now(),
						}
						_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
					}
				}
			}
		}
	}

	actID2 := fmt.Sprintf("poll-reg-%d", time.Now().UnixNano())
	outReg, _ := tm.ami.Command(ctx, actID2, "pjsip show registrations")
	if outReg != "" {
		lines := strings.Split(outReg, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "-reg/") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 3 {
					trunkID := cleanTrunkID(parts[0])
					if trunkID == "" {
						continue
					}
					regStatus := parts[2]
					healthStatus := "REGISTERED"
					if regStatus == "Rejected" {
						healthStatus = "REJECTED"
					} else if regStatus == "Unregistered" || regStatus == "Failed" {
						healthStatus = "OFFLINE"
					} else if regStatus != "Registered" {
						healthStatus = regStatus
					}
					existing, _ := tm.cache.GetTrunkHealth(ctx, trunkID)
					health := domain.TrunkHealth{
						Status:         healthStatus,
						ActiveChannels: tm.channels.GetTrunkActiveCount(trunkID),
						LastQualifyAt:  time.Now(),
					}
					if existing != nil && existing.LatencyMS > 0 {
						health.LatencyMS = existing.LatencyMS
					}
					_ = tm.cache.SetTrunkHealth(ctx, trunkID, health, 24*time.Hour)
				}
			}
		}
	}
}
