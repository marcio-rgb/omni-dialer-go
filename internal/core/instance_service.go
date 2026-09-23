package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// InstanceService gerencia a lógica de negócio e averiguação de saúde das instâncias Go.
//
// @pattern Service
// @governedBy .agents/ARCHITECT.md
type InstanceService struct {
	repo       ports.InstanceRepository
	httpClient *http.Client
}

var _ ports.InstanceService = (*InstanceService)(nil)

// NewInstanceService cria uma nova instância do serviço de governança de nós.
func NewInstanceService(repo ports.InstanceRepository) *InstanceService {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	return &InstanceService{
		repo:       repo,
		httpClient: client,
	}
}

// List retorna a lista de instâncias registradas.
func (s *InstanceService) List(ctx context.Context, tenantID string, mode string) ([]*domain.Instance, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("instance_service: repositorio nao inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	return s.repo.ListByTenant(ctx, tenantID, strings.ToLower(mode))
}

// Get recupera uma instância específica por ID e Tenant.
func (s *InstanceService) Get(ctx context.Context, tenantID, id string) (*domain.Instance, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("instance_service: repositorio nao inicializado")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	inst, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("instancia '%s' nao encontrada", id)
	}
	return inst, nil
}

// Create valida e cadastra uma nova instância do Go.
func (s *InstanceService) Create(ctx context.Context, tenantID string, dto domain.CreateInstanceDTO) (*domain.Instance, error) {
	if tenantID != "" {
		dto.TenantID = tenantID
	}
	if err := dto.Validate(); err != nil {
		return nil, fmt.Errorf("dados invalidos: %w", err)
	}

	existing, err := s.repo.GetByID(ctx, dto.TenantID, dto.ID)
	if err != nil {
		return nil, fmt.Errorf("falha ao verificar unicidade da instancia: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("instancia com ID '%s' ja existe", dto.ID)
	}

	isActive := true
	if dto.IsActive != nil {
		isActive = *dto.IsActive
	}

	meta := dto.Metadata
	if meta == nil {
		meta = make(map[string]any)
	}

	inst := &domain.Instance{
		ID:          dto.ID,
		TenantID:    dto.TenantID,
		Name:        dto.Name,
		Mode:        dto.Mode,
		HostURL:     strings.TrimRight(dto.HostURL, "/"),
		APIKey:      dto.APIKey,
		MaxChannels: dto.MaxChannels,
		IsActive:    isActive,
		Description: dto.Description,
		Metadata:    meta,
	}

	if err := s.repo.Create(ctx, inst); err != nil {
		return nil, fmt.Errorf("falha ao persistir instancia: %w", err)
	}

	return inst, nil
}

// Update altera as propriedades de uma instância existente.
func (s *InstanceService) Update(ctx context.Context, tenantID, id string, dto domain.UpdateInstanceDTO) (*domain.Instance, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if err := dto.Validate(); err != nil {
		return nil, fmt.Errorf("dados invalidos: %w", err)
	}

	inst, err := s.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if dto.Name != nil {
		inst.Name = strings.TrimSpace(*dto.Name)
	}
	if dto.Mode != nil {
		inst.Mode = *dto.Mode
	}
	if dto.HostURL != nil {
		inst.HostURL = strings.TrimRight(strings.TrimSpace(*dto.HostURL), "/")
	}
	if dto.APIKey != nil {
		inst.APIKey = strings.TrimSpace(*dto.APIKey)
	}
	if dto.MaxChannels != nil {
		inst.MaxChannels = *dto.MaxChannels
	}
	if dto.IsActive != nil {
		inst.IsActive = *dto.IsActive
	}
	if dto.Description != nil {
		inst.Description = strings.TrimSpace(*dto.Description)
	}
	if dto.Metadata != nil {
		inst.Metadata = dto.Metadata
	}

	if err := s.repo.Update(ctx, inst); err != nil {
		return nil, fmt.Errorf("falha ao atualizar instancia: %w", err)
	}

	return inst, nil
}

// Delete remove uma instância do sistema.
func (s *InstanceService) Delete(ctx context.Context, tenantID, id string) error {
	if tenantID == "" {
		tenantID = "default"
	}
	return s.repo.Delete(ctx, tenantID, id)
}

// Ping executa um diagnóstico HTTP imediato no endpoint /health da instância remota.
func (s *InstanceService) Ping(ctx context.Context, tenantID, id string) (*domain.InstancePingResult, error) {
	inst, err := s.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	targetURL := fmt.Sprintf("%s/health", strings.TrimRight(inst.HostURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return &domain.InstancePingResult{
			InstanceID:  inst.ID,
			Mode:        inst.Mode,
			Status:      "offline",
			LatencyMS:   0,
			MaxChannels: inst.MaxChannels,
			CheckedAt:   time.Now().UTC(),
		}, nil
	}

	req.Header.Set("User-Agent", "Dialer-Go-Prober/1.0")
	if inst.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+inst.APIKey)
	}

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return &domain.InstancePingResult{
			InstanceID:  inst.ID,
			Mode:        inst.Mode,
			Status:      "offline",
			LatencyMS:   latency,
			MaxChannels: inst.MaxChannels,
			CheckedAt:   time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()

	var healthData struct {
		Status            string `json:"status"`
		TelephonyCapacity struct {
			ActiveGlobalChannels int `json:"active_global_channels"`
			MaxGlobalChannels    int `json:"max_global_channels"`
		} `json:"telephony_capacity"`
		Components map[string]any `json:"components"`
	}

	activeChans := 0
	maxChans := inst.MaxChannels

	if err := json.NewDecoder(resp.Body).Decode(&healthData); err == nil {
		if healthData.TelephonyCapacity.MaxGlobalChannels > 0 {
			maxChans = healthData.TelephonyCapacity.MaxGlobalChannels
		}
		activeChans = healthData.TelephonyCapacity.ActiveGlobalChannels
	}

	return &domain.InstancePingResult{
		InstanceID:     inst.ID,
		Mode:           inst.Mode,
		Status:         "online",
		LatencyMS:      latency,
		ActiveChannels: activeChans,
		MaxChannels:    maxChans,
		CheckedAt:      time.Now().UTC(),
		Details:        healthData.Components,
	}, nil
}
