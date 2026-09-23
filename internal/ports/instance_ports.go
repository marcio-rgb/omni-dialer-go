package ports

import (
	"context"
	"dialer-go/internal/domain"
)

// InstanceRepository define o contrato canônico para persistência e recuperação de instâncias Go.
//
// @pattern Repository (Port)
// @governedBy .agents/ARCHITECT.md
type InstanceRepository interface {
	Create(ctx context.Context, inst *domain.Instance) error
	GetByID(ctx context.Context, tenantID, id string) (*domain.Instance, error)
	ListByTenant(ctx context.Context, tenantID string, mode string) ([]*domain.Instance, error)
	Update(ctx context.Context, inst *domain.Instance) error
	Delete(ctx context.Context, tenantID, id string) error
}

// InstanceService define o contrato de negócio para gerenciamento e telemetria de instâncias.
//
// @pattern Service (Port)
// @governedBy .agents/ARCHITECT.md
type InstanceService interface {
	List(ctx context.Context, tenantID string, mode string) ([]*domain.Instance, error)
	Get(ctx context.Context, tenantID, id string) (*domain.Instance, error)
	Create(ctx context.Context, tenantID string, dto domain.CreateInstanceDTO) (*domain.Instance, error)
	Update(ctx context.Context, tenantID, id string, dto domain.UpdateInstanceDTO) (*domain.Instance, error)
	Delete(ctx context.Context, tenantID, id string) error
	Ping(ctx context.Context, tenantID, id string) (*domain.InstancePingResult, error)
}
