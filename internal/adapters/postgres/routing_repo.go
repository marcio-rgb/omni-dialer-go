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

type RoutingRepo struct {
	pool *pgxpool.Pool
}

var _ ports.RoutingRepository = (*RoutingRepo)(nil)

func NewRoutingRepo(pool *pgxpool.Pool) *RoutingRepo {
	return &RoutingRepo{pool: pool}
}

func (r *RoutingRepo) GetLastRouting(ctx context.Context, phone string) (*domain.PhoneTrunkMapping, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT phone, last_trunk, last_project, last_sip_route, tenant_id, updated_at
		FROM phone_trunk_mappings
		WHERE phone = $1
	`

	var m domain.PhoneTrunkMapping
	err := r.pool.QueryRow(ctx, query, phone).Scan(
		&m.Phone, &m.LastTrunk, &m.LastProject, &m.LastSIPRoute, &m.TenantID, &m.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &m, nil
}

func (r *RoutingRepo) SaveRouting(ctx context.Context, m *domain.PhoneTrunkMapping) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		INSERT INTO phone_trunk_mappings (phone, last_trunk, last_project, last_sip_route, tenant_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (phone) DO UPDATE SET
			last_trunk = EXCLUDED.last_trunk,
			last_project = EXCLUDED.last_project,
			last_sip_route = EXCLUDED.last_sip_route,
			tenant_id = EXCLUDED.tenant_id,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.pool.Exec(ctx, query, m.Phone, m.LastTrunk, m.LastProject, m.LastSIPRoute, m.TenantID, time.Now())
	return err
}
