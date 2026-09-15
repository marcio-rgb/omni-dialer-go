package postgres

import (
	"context"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TenantRepo struct {
	pool *pgxpool.Pool
}

var _ ports.TenantRepository = (*TenantRepo)(nil)

func NewTenantRepo(pool *pgxpool.Pool) *TenantRepo {
	return &TenantRepo{pool: pool}
}

func (r *TenantRepo) GetByID(ctx context.Context, tenantID string) (*domain.Tenant, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, name, COALESCE(webhook, ''), created_at, updated_at
		FROM tenants
		WHERE id = $1
	`

	var t domain.Tenant
	err := r.pool.QueryRow(ctx, query, tenantID).Scan(
		&t.ID, &t.Name, &t.Webhook, &t.CreatedAt, &t.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func (r *TenantRepo) Save(ctx context.Context, t *domain.Tenant) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		INSERT INTO tenants (id, name, webhook, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			webhook = EXCLUDED.webhook,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query, t.ID, t.Name, t.Webhook, t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *TenantRepo) ListAll(ctx context.Context) ([]*domain.Tenant, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, name, COALESCE(webhook, ''), created_at, updated_at
		FROM tenants
		ORDER BY id ASC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []*domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Webhook, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, &t)
	}

	return tenants, rows.Err()
}
