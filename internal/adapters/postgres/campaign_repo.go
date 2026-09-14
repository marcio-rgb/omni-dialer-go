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

func (r *CampaignRepo) ListByTenant(ctx context.Context, tenantID string, status *domain.CampaignStatus) ([]*domain.Campaign, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, name, mode, status, aggressiveness, trunk_name, cycle_count, saturation_level, last_cycle_at, created_at
		FROM campaigns
		WHERE tenant_id = $1
	`
	args := []any{tenantID}
	if status != nil && *status != "" {
		query += " AND status = $2"
		args = append(args, string(*status))
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
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
	if list == nil {
		list = make([]*domain.Campaign, 0)
	}
	return list, nil
}

func (r *CampaignRepo) Create(ctx context.Context, c *domain.Campaign) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		INSERT INTO campaigns (id, tenant_id, name, mode, status, aggressiveness, trunk_name, cycle_count, saturation_level, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	createdAt := c.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
		c.CreatedAt = createdAt
	}
	satLevel := c.SaturationLevel
	if satLevel == "" {
		satLevel = domain.SaturationNova
		c.SaturationLevel = satLevel
	}
	mode := c.Mode
	if mode == "" {
		mode = domain.CampaignModePredictive
		c.Mode = mode
	}
	status := c.Status
	if status == "" {
		status = domain.CampaignStatusActive
		c.Status = status
	}
	trunkName := c.TrunkName
	if trunkName == "" {
		trunkName = "auto"
		c.TrunkName = trunkName
	}
	aggressiveness := c.Aggressiveness
	if aggressiveness <= 0 {
		aggressiveness = 1.20
		c.Aggressiveness = aggressiveness
	}

	_, err := r.pool.Exec(ctx, query,
		c.ID, c.TenantID, c.Name, string(mode), string(status),
		aggressiveness, trunkName, c.CycleCount, string(satLevel), createdAt,
	)
	return err
}

func (r *CampaignRepo) Update(ctx context.Context, c *domain.Campaign) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		UPDATE campaigns
		SET name = $3, mode = $4, status = $5, aggressiveness = $6, trunk_name = $7
		WHERE id = $1 AND tenant_id = $2
	`
	tag, err := r.pool.Exec(ctx, query,
		c.ID, c.TenantID, c.Name, string(c.Mode), string(c.Status),
		c.Aggressiveness, c.TrunkName,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *CampaignRepo) Delete(ctx context.Context, tenantID, campaignID string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `DELETE FROM campaigns WHERE id = $1 AND tenant_id = $2`
	tag, err := r.pool.Exec(ctx, query, campaignID, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
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

