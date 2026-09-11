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

type CampaignRepo struct {
	pool *pgxpool.Pool
}

var _ ports.CampaignRepository = (*CampaignRepo)(nil)

func NewCampaignRepo(pool *pgxpool.Pool) *CampaignRepo {
	return &CampaignRepo{pool: pool}
}

func (r *CampaignRepo) GetByID(ctx context.Context, tenantID, campaignID string) (*domain.Campaign, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, name, mode, status, aggressiveness, trunk_name, cycle_count, saturation_level, last_cycle_at, created_at
		FROM campaigns
		WHERE id = $1 AND tenant_id = $2
	`

	var c domain.Campaign
	var modeStr, statusStr, satStr string

	err := r.pool.QueryRow(ctx, query, campaignID, tenantID).Scan(
		&c.ID, &c.TenantID, &c.Name, &modeStr, &statusStr, &c.Aggressiveness, &c.TrunkName,
		&c.CycleCount, &satStr, &c.LastCycleAt, &c.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.Mode = domain.CampaignMode(modeStr)
	c.Status = domain.CampaignStatus(statusStr)
	c.SaturationLevel = domain.SaturationLevel(satStr)

	return &c, nil
}

func (r *CampaignRepo) ListActive(ctx context.Context, tenantID string) ([]*domain.Campaign, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, name, mode, status, aggressiveness, trunk_name, cycle_count, saturation_level, last_cycle_at, created_at
		FROM campaigns
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.Campaign
	for rows.Next() {
		var c domain.Campaign
		var modeStr, statusStr, satStr string
		err := rows.Scan(
			&c.ID, &c.TenantID, &c.Name, &modeStr, &statusStr, &c.Aggressiveness, &c.TrunkName,
			&c.CycleCount, &satStr, &c.LastCycleAt, &c.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		c.Mode = domain.CampaignMode(modeStr)
		c.Status = domain.CampaignStatus(statusStr)
		c.SaturationLevel = domain.SaturationLevel(satStr)
		list = append(list, &c)
	}

	return list, nil
}

func (r *CampaignRepo) SetStatus(ctx context.Context, tenantID, campaignID string, status domain.CampaignStatus) (*domain.Campaign, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		INSERT INTO campaigns (id, tenant_id, name, mode, status, aggressiveness, trunk_name, cycle_count, saturation_level, created_at)
		VALUES ($1, $2, $1, 'PREDICTIVE', $3, 1.20, 'auto', 0, 'NOVA', NOW())
		ON CONFLICT (id) DO UPDATE
		SET status = EXCLUDED.status
		WHERE campaigns.tenant_id = $2
		RETURNING id, tenant_id, name, mode, status, aggressiveness, trunk_name, cycle_count, saturation_level, last_cycle_at, created_at
	`

	var c domain.Campaign
	var modeStr, statusStr, satStr string

	err := r.pool.QueryRow(ctx, query, campaignID, tenantID, string(status)).Scan(
		&c.ID, &c.TenantID, &c.Name, &modeStr, &statusStr, &c.Aggressiveness, &c.TrunkName,
		&c.CycleCount, &satStr, &c.LastCycleAt, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	c.Mode = domain.CampaignMode(modeStr)
	c.Status = domain.CampaignStatus(statusStr)
	c.SaturationLevel = domain.SaturationLevel(satStr)

	return &c, nil
}

func (r *CampaignRepo) IncrementCycle(ctx context.Context, campaignID string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		UPDATE campaigns
		SET cycle_count = cycle_count + 1, last_cycle_at = $2, saturation_level = 'RECICLADA'
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, campaignID, time.Now())
	return err
}
