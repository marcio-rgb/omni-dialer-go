package domain

import (
	"fmt"
	"strings"
	"time"
)

// InstanceMode define o modo operacional da instância Go: dialer ou dispatcher.
type InstanceMode string

const (
	ModeDialer     InstanceMode = "dialer"
	ModeDispatcher InstanceMode = "dispatcher"
)

// IsValid verifica se o modo informado é suportado.
func (m InstanceMode) IsValid() bool {
	return m == ModeDialer || m == ModeDispatcher
}

// Instance representa o registro canônico de uma instância do Dialer-Go ou Dispatcher-Go.
//
// @pattern Entity
// @governedBy .agents/ARCHITECT.md
type Instance struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	Mode        InstanceMode   `json:"mode"`
	HostURL     string         `json:"host_url"`
	APIKey      string         `json:"api_key,omitempty"`
	MaxChannels int            `json:"max_channels"`
	IsActive    bool           `json:"is_active"`
	Description string         `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// CreateInstanceDTO especifica o contrato de entrada para cadastro de nova instância.
//
// @pattern DTO
// @governedBy .agents/ARCHITECT.md
type CreateInstanceDTO struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	Mode        InstanceMode   `json:"mode"`
	HostURL     string         `json:"host_url"`
	APIKey      string         `json:"api_key,omitempty"`
	MaxChannels int            `json:"max_channels"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Description string         `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Validate executa a validação estrita dos campos requeridos na criação.
func (dto *CreateInstanceDTO) Validate() error {
	dto.ID = strings.TrimSpace(dto.ID)
	dto.Name = strings.TrimSpace(dto.Name)
	dto.HostURL = strings.TrimSpace(dto.HostURL)

	if dto.ID == "" {
		return fmt.Errorf("o campo 'id' e obrigatorio")
	}
	if dto.Name == "" {
		return fmt.Errorf("o campo 'name' e obrigatorio")
	}
	if !dto.Mode.IsValid() {
		return fmt.Errorf("o campo 'mode' deve ser 'dialer' ou 'dispatcher'")
	}
	if dto.HostURL == "" {
		return fmt.Errorf("o campo 'host_url' e obrigatorio")
	}
	if !strings.HasPrefix(dto.HostURL, "http://") && !strings.HasPrefix(dto.HostURL, "https://") {
		return fmt.Errorf("o campo 'host_url' deve iniciar com http:// ou https://")
	}
	if dto.MaxChannels <= 0 {
		dto.MaxChannels = 30
	}
	if dto.TenantID == "" {
		dto.TenantID = "default"
	}
	return nil
}

// UpdateInstanceDTO especifica o contrato para atualização parcial ou total de uma instância.
//
// @pattern DTO
// @governedBy .agents/ARCHITECT.md
type UpdateInstanceDTO struct {
	Name        *string        `json:"name,omitempty"`
	Mode        *InstanceMode  `json:"mode,omitempty"`
	HostURL     *string        `json:"host_url,omitempty"`
	APIKey      *string        `json:"api_key,omitempty"`
	MaxChannels *int           `json:"max_channels,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Validate executa a validação dos campos informados na atualização.
func (dto *UpdateInstanceDTO) Validate() error {
	if dto.Name != nil && strings.TrimSpace(*dto.Name) == "" {
		return fmt.Errorf("o campo 'name' nao pode ser vazio")
	}
	if dto.Mode != nil && !dto.Mode.IsValid() {
		return fmt.Errorf("o campo 'mode' deve ser 'dialer' ou 'dispatcher'")
	}
	if dto.HostURL != nil {
		url := strings.TrimSpace(*dto.HostURL)
		if url == "" || (!strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://")) {
			return fmt.Errorf("o campo 'host_url' deve iniciar com http:// ou https://")
		}
	}
	if dto.MaxChannels != nil && *dto.MaxChannels <= 0 {
		return fmt.Errorf("o campo 'max_channels' deve ser maior que zero")
	}
	return nil
}

// InstancePingResult detalha o resultado da averiguação de saúde em tempo real.
//
// @pattern DTO
// @governedBy .agents/ARCHITECT.md
type InstancePingResult struct {
	InstanceID     string       `json:"instance_id"`
	Mode           InstanceMode `json:"mode"`
	Status         string       `json:"status"` // "online" ou "offline"
	LatencyMS      int64        `json:"latency_ms"`
	ActiveChannels int          `json:"active_channels"`
	MaxChannels    int          `json:"max_channels"`
	CheckedAt      time.Time    `json:"checked_at"`
	Details        any          `json:"details,omitempty"`
}
