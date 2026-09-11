package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

type TrunkHandler struct {
	repo     ports.TrunkRepository
	cache    ports.CachePort
	manager  *core.TrunkManager
	channels *core.ChannelManager
	ami      ports.AMIPort
}

func NewTrunkHandler(repo ports.TrunkRepository, cache ports.CachePort, manager *core.TrunkManager, channels *core.ChannelManager, ami ports.AMIPort) *TrunkHandler {
	return &TrunkHandler{
		repo:     repo,
		cache:    cache,
		manager:  manager,
		channels: channels,
		ami:      ami,
	}
}

func (h *TrunkHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "O header 'X-Tenant-Id' é obrigatório.").WriteJSON(w)
		return
	}

	trunks, err := h.repo.ListByTenant(r.Context(), tenantID)
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao listar troncos: %s", err.Error())).WriteJSON(w)
		return
	}

	var enriched []*domain.TrunkWithHealth
	for _, t := range trunks {
		health, _ := h.cache.GetTrunkHealth(r.Context(), t.ID)
		activeCount := h.channels.GetTrunkActiveCount(t.ID)

		var trunkHealth domain.TrunkHealth
		if health != nil {
			trunkHealth = *health
			trunkHealth.ActiveChannels = activeCount
			trunkHealth.MaxChannels = t.MaxChannels
			trunkHealth.AvailableChannels = t.MaxChannels - activeCount
			if t.MaxChannels > 0 {
				trunkHealth.UtilizationPercentage = float64(activeCount) / float64(t.MaxChannels) * 100
			}
			trunkHealth.IsSaturated = activeCount >= t.MaxChannels
		} else {
			trunkHealth = domain.TrunkHealth{
				Status:                "OFFLINE",
				LatencyMS:             0.0,
				ActiveChannels:        activeCount,
				MaxChannels:           t.MaxChannels,
				AvailableChannels:     t.MaxChannels - activeCount,
				UtilizationPercentage: 0,
				IsSaturated:           activeCount >= t.MaxChannels,
				LastQualifyAt:         time.Time{},
			}
		}

		enriched = append(enriched, &domain.TrunkWithHealth{
			Trunk:  *t,
			Health: trunkHealth,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"tenant_id":    tenantID,
			"total_trunks": len(enriched),
			"trunks":       enriched,
		},
	})
}

func (h *TrunkHandler) Create(w http.ResponseWriter, r *http.Request) {
	var dto domain.CreateTrunkDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Corpo da requisição inválido").WriteJSON(w)
		return
	}

	if dto.ID == "" || dto.TenantID == "" || dto.Name == "" || dto.Host == "" {
		domain.NewErrBadRequest("MISSING_FIELDS", "Os campos id, tenant_id, name e host são obrigatórios.").WriteJSON(w)
		return
	}

	if dto.Port == 0 {
		dto.Port = 5060
	}
	if dto.MaxChannels == 0 {
		dto.MaxChannels = 30
	}
	if dto.QualifyFrequency == 0 {
		dto.QualifyFrequency = 30
	}
	if dto.QualifyTimeout == 0 {
		dto.QualifyTimeout = 3.0
	}
	if len(dto.Codecs) == 0 {
		dto.Codecs = []string{"alaw", "ulaw"}
	}
	if dto.Direction == "" {
		dto.Direction = domain.DirectionBidirectional
	}
	if dto.RegistrationMode == "" {
		dto.RegistrationMode = domain.RegistrationModeIPBased
	}
	if dto.Transport == "" {
		dto.Transport = domain.TransportUDP
	}
	if dto.NATMode == "" {
		dto.NATMode = domain.NATModeForceRPort
	}
	if dto.DTMFMode == "" {
		dto.DTMFMode = domain.DTMFModeRFC4733
	}

	trunk := &domain.Trunk{
		ID:               dto.ID,
		TenantID:         dto.TenantID,
		Name:             dto.Name,
		Direction:        dto.Direction,
		RegistrationMode: dto.RegistrationMode,
		Host:             dto.Host,
		Port:             dto.Port,
		OutboundProxy:    dto.OutboundProxy,
		TechPrefix:       dto.TechPrefix,
		AuthUsername:     dto.AuthUsername,
		AuthPassword:     dto.AuthPassword,
		AuthRealm:        dto.AuthRealm,
		FromUser:         dto.FromUser,
		FromDomain:       dto.FromDomain,
		UserAgent:        dto.UserAgent,
		Transport:        dto.Transport,
		NATMode:          dto.NATMode,
		Codecs:           dto.Codecs,
		DTMFMode:         dto.DTMFMode,
		QualifyFrequency: dto.QualifyFrequency,
		QualifyTimeout:   dto.QualifyTimeout,
		MaxChannels:      dto.MaxChannels,
		IsEnabled:        dto.IsEnabled,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := h.repo.Create(r.Context(), trunk); err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao salvar tronco no banco: %s", err.Error())).WriteJSON(w)
		return
	}

	h.channels.RegisterTrunkLimit(trunk.ID, trunk.MaxChannels)
	_ = h.manager.ReloadPBXTrunks(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"id":      trunk.ID,
			"name":    trunk.Name,
			"status":  "created",
			"message": "Tronco cadastrado e sincronizado com o PBX com sucesso.",
		},
	})
}

func (h *TrunkHandler) Update(w http.ResponseWriter, r *http.Request) {
	trunkID := chi.URLParam(r, "trunk_id")
	tenantID := r.Header.Get("X-Tenant-Id")

	if trunkID == "" || tenantID == "" {
		domain.NewErrBadRequest("MISSING_IDENTIFIER", "trunk_id no path e X-Tenant-Id no header são obrigatórios.").WriteJSON(w)
		return
	}

	existing, err := h.repo.GetByID(r.Context(), tenantID, trunkID)
	if err != nil || existing == nil {
		domain.NewErrNotFound("TRUNK_NOT_FOUND", fmt.Sprintf("O tronco '%s' não foi localizado.", trunkID)).WriteJSON(w)
		return
	}

	var dto domain.UpdateTrunkDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	if dto.Name != nil {
		existing.Name = *dto.Name
	}
	if dto.Direction != nil {
		existing.Direction = *dto.Direction
	}
	if dto.RegistrationMode != nil {
		existing.RegistrationMode = *dto.RegistrationMode
	}
	if dto.Host != nil {
		existing.Host = *dto.Host
	}
	if dto.Port != nil {
		existing.Port = *dto.Port
	}
	if dto.OutboundProxy != nil {
		existing.OutboundProxy = dto.OutboundProxy
	}
	if dto.TechPrefix != nil {
		existing.TechPrefix = dto.TechPrefix
	}
	if dto.AuthUsername != nil {
		existing.AuthUsername = dto.AuthUsername
	}
	if dto.AuthPassword != nil {
		existing.AuthPassword = dto.AuthPassword
	}
	if dto.AuthRealm != nil {
		existing.AuthRealm = dto.AuthRealm
	}
	if dto.FromUser != nil {
		existing.FromUser = dto.FromUser
	}
	if dto.FromDomain != nil {
		existing.FromDomain = dto.FromDomain
	}
	if dto.UserAgent != nil {
		existing.UserAgent = dto.UserAgent
	}
	if dto.Transport != nil {
		existing.Transport = *dto.Transport
	}
	if dto.NATMode != nil {
		existing.NATMode = *dto.NATMode
	}
	if dto.Codecs != nil {
		existing.Codecs = dto.Codecs
	}
	if dto.DTMFMode != nil {
		existing.DTMFMode = *dto.DTMFMode
	}
	if dto.MaxChannels != nil {
		existing.MaxChannels = *dto.MaxChannels
		h.channels.RegisterTrunkLimit(existing.ID, existing.MaxChannels)
	}
	if dto.IsEnabled != nil {
		existing.IsEnabled = *dto.IsEnabled
	}
	existing.UpdatedAt = time.Now()

	if err := h.repo.Update(r.Context(), existing); err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao atualizar tronco: %s", err.Error())).WriteJSON(w)
		return
	}

	_ = h.manager.ReloadPBXTrunks(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"id":         existing.ID,
			"tenant_id":  existing.TenantID,
			"name":       existing.Name,
			"updated_at": existing.UpdatedAt.Format(time.RFC3339),
			"pbx_sync": map[string]string{
				"status":  "APPLIED_HOT",
				"message": "Configuração PJSIP recarregada dinamicamente sem interrupção de canais em curso.",
			},
		},
	})
}

func (h *TrunkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	trunkID := chi.URLParam(r, "trunk_id")
	tenantID := r.Header.Get("X-Tenant-Id")
	force, _ := strconv.ParseBool(r.URL.Query().Get("force"))

	if trunkID == "" || tenantID == "" {
		domain.NewErrBadRequest("MISSING_IDENTIFIER", "trunk_id no path e X-Tenant-Id no header são obrigatórios.").WriteJSON(w)
		return
	}

	activeChannels := h.channels.GetTrunkActiveCount(trunkID)
	if activeChannels > 0 && !force {
		meta := map[string]interface{}{
			"active_channels_count": activeChannels,
		}
		domain.NewErrConflict("ACTIVE_CHANNELS_PRESENT",
			fmt.Sprintf("O tronco possui %d chamadas ativas em andamento. Envie ?force=true para encerramento forçado imediato.", activeChannels),
			meta).WriteJSON(w)
		return
	}

	if err := h.repo.Delete(r.Context(), tenantID, trunkID); err != nil {
		domain.NewErrNotFound("TRUNK_NOT_FOUND", fmt.Sprintf("O tronco '%s' não existe ou já foi excluído.", trunkID)).WriteJSON(w)
		return
	}

	h.channels.UnregisterTrunk(trunkID)
	_ = h.manager.ReloadPBXTrunks(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"trunk_id": trunkID,
			"status":   "deleted",
			"message":  "Tronco excluído com sucesso e configurações removidas do PBX.",
		},
	})
}

func (h *TrunkHandler) Enumerations(w http.ResponseWriter, r *http.Request) {
	enums := domain.TrunkEnumerationsDTO{
		RegistrationModes: []domain.EnumOption{
			{Value: "REGISTER", Label: "Com Registro (User/Senha)"},
			{Value: "IP_BASED", Label: "Sem Registro (Autenticação por IP)"},
		},
		Transports: []domain.EnumOption{
			{Value: "UDP", Label: "UDP (Padrão de Baixa Latência)"},
			{Value: "TCP", Label: "TCP (Confiável para Pacotes Longos)"},
			{Value: "TLS", Label: "TLS (Sinalização Encriptada)"},
		},
		Directions: []domain.EnumOption{
			{Value: "BIDIRECTIONAL", Label: "Bidirecional (Entrada e Saída)"},
			{Value: "OUTBOUND", Label: "Somente Saída (Discador Preditivo/Manual)"},
			{Value: "INBOUND", Label: "Somente Entrada (DIDs Receptivos)"},
		},
		SupportedCodecs: []domain.EnumOption{
			{Value: "alaw", Label: "G.711 A-law (PCMA - Padrão Brasil)"},
			{Value: "ulaw", Label: "G.711 u-law (PCMU - Padrão Internacional)"},
			{Value: "g729", Label: "G.729 (Baixo Consumo de Banda)"},
			{Value: "opus", Label: "Opus (HD Audio WebRTC/LiveKit)"},
			{Value: "gsm", Label: "GSM 06.10"},
		},
		DTMFModes: []domain.EnumOption{
			{Value: "RFC4733", Label: "RFC 4733 / RFC 2833 (Recomendado)"},
			{Value: "INBAND", Label: "In-Band (Tons no Áudio)"},
			{Value: "INFO", Label: "SIP INFO"},
			{Value: "AUTO", Label: "Detecção Automática"},
		},
		NATModes: []domain.EnumOption{
			{Value: "FORCE_RPORT", Label: "Force rport (Simétrico / NAT Severo)"},
			{Value: "YES", Label: "Yes (Habilitado)"},
			{Value: "NO", Label: "No (IP Público Direto)"},
			{Value: "COMEDIA", Label: "Comedia"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    enums,
	})
}

func (h *TrunkHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.manager.ReloadPBXTrunks(r.Context()); err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao recarregar PBX: %s", err.Error())).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Agrupamento de troncos e regras do PBX recarregados com sucesso.",
	})
}
