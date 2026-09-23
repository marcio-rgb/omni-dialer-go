package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InstanceRepo implementa o contrato ports.InstanceRepository sobre o PostgreSQL dialer_db.
//
// @pattern Repository
// @governedBy .agents/ARCHITECT.md
type InstanceRepo struct {
	pool *pgxpool.Pool
}

var _ ports.InstanceRepository = (*InstanceRepo)(nil)

// NewInstanceRepo instancia o repositório de instâncias Go.
func NewInstanceRepo(pool *pgxpool.Pool) *InstanceRepo {
	return &InstanceRepo{pool: pool}
}

// Create persiste uma nova instância no banco relacional.
func (r *InstanceRepo) Create(ctx context.Context, inst *domain.Instance) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool nao inicializado")
	}

	metadataJSON, err := json.Marshal(inst.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	now := time.Now().UTC()
	inst.CreatedAt = now
	inst.UpdatedAt = now

	query := `
		INSERT INTO instances (
			id, tenant_id, name, mode, host_url, api_key, max_channels, is_active, description, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`

	_, err = r.pool.Exec(ctx, query,
		inst.ID,
		inst.TenantID,
		inst.Name,
		string(inst.Mode),
		inst.HostURL,
		inst.APIKey,
		inst.MaxChannels,
		inst.IsActive,
		inst.Description,
		metadataJSON,
		inst.CreatedAt,
		inst.UpdatedAt,
	)
	return err
}

// GetByID busca uma instância pelo identificador e tenant.
func (r *InstanceRepo) GetByID(ctx context.Context, tenantID, id string) (*domain.Instance, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool nao inicializado")
	}

	query := `
		SELECT id, tenant_id, name, mode, host_url, COALESCE(api_key, ''), max_channels, is_active,
		       COALESCE(description, ''), COALESCE(metadata, '{}'::jsonb), created_at, updated_at
		FROM instances
		WHERE id = $1 AND tenant_id = $2
	`

	var inst domain.Instance
	var modeStr string
	var metaBytes []byte

	err := r.pool.QueryRow(ctx, query, id, tenantID).Scan(
		&inst.ID,
		&inst.TenantID,
		&inst.Name,
		&modeStr,
		&inst.HostURL,
		&inst.APIKey,
		&inst.MaxChannels,
		&inst.IsActive,
		&inst.Description,
		&metaBytes,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	inst.Mode = domain.InstanceMode(modeStr)
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &inst.Metadata)
	}
	if inst.Metadata == nil {
		inst.Metadata = make(map[string]any)
	}

	return &inst, nil
}

// ListByTenant lista instâncias cadastradas de um tenant, com filtro opcional de modo.
func (r *InstanceRepo) ListByTenant(ctx context.Context, tenantID string, mode string) ([]*domain.Instance, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool nao inicializado")
	}

	var query string
	var args []any

	if mode != "" {
		query = `
			SELECT id, tenant_id, name, mode, host_url, COALESCE(api_key, ''), max_channels, is_active,
			       COALESCE(description, ''), COALESCE(metadata, '{}'::jsonb), created_at, updated_at
			FROM instances
			WHERE tenant_id = $1 AND mode = $2
			ORDER BY created_at ASC
		`
		args = []any{tenantID, mode}
	} else {
		query = `
			SELECT id, tenant_id, name, mode, host_url, COALESCE(api_key, ''), max_channels, is_active,
			       COALESCE(description, ''), COALESCE(metadata, '{}'::jsonb), created_at, updated_at
			FROM instances
			WHERE tenant_id = $1
			ORDER BY created_at ASC
		`
		args = []any{tenantID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.Instance
	for rows.Next() {
		var inst domain.Instance
		var modeStr string
		var metaBytes []byte

		if err := rows.Scan(
			&inst.ID,
			&inst.TenantID,
			&inst.Name,
			&modeStr,
			&inst.HostURL,
			&inst.APIKey,
			&inst.MaxChannels,
			&inst.IsActive,
			&inst.Description,
			&metaBytes,
			&inst.CreatedAt,
			&inst.UpdatedAt,
		); err != nil {
			return nil, err
		}

		inst.Mode = domain.InstanceMode(modeStr)
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &inst.Metadata)
		}
		if inst.Metadata == nil {
			inst.Metadata = make(map[string]any)
		}

		result = append(result, &inst)
	}

	return result, rows.Err()
}

// Update altera as propriedades de uma instância cadastrada.
func (r *InstanceRepo) Update(ctx context.Context, inst *domain.Instance) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool nao inicializado")
	}

	metadataJSON, err := json.Marshal(inst.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	inst.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE instances
		SET name = $1,
		    mode = $2,
		    host_url = $3,
		    api_key = $4,
		    max_channels = $5,
		    is_active = $6,
		    description = $7,
		    metadata = $8,
		    updated_at = $9
		WHERE id = $10 AND tenant_id = $11
	`

	cmdTag, err := r.pool.Exec(ctx, query,
		inst.Name,
		string(inst.Mode),
		inst.HostURL,
		inst.APIKey,
		inst.MaxChannels,
		inst.IsActive,
		inst.Description,
		metadataJSON,
		inst.UpdatedAt,
		inst.ID,
		inst.TenantID,
	)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("instancia '%s' nao encontrada para o tenant informado", inst.ID)
	}
	return nil
}

// Delete remove uma instância do catálogo.
func (r *InstanceRepo) Delete(ctx context.Context, tenantID, id string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool nao inicializado")
	}

	query := `DELETE FROM instances WHERE id = $1 AND tenant_id = $2`
	cmdTag, err := r.pool.Exec(ctx, query, id, tenantID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("instancia '%s' nao encontrada para remocao", id)
	}
	return nil
}
