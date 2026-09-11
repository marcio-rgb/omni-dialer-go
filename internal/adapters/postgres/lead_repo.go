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

type LeadRepo struct {
	pool *pgxpool.Pool
}

var _ ports.LeadRepository = (*LeadRepo)(nil)

func NewLeadRepo(pool *pgxpool.Pool) *LeadRepo {
	return &LeadRepo{pool: pool}
}

func (r *LeadRepo) BatchInsert(ctx context.Context, leads []*domain.Lead) (int64, error) {
	if r.pool == nil {
		return 0, fmt.Errorf("postgres: pool não inicializado")
	}
	if len(leads) == 0 {
		return 0, nil
	}

	rows := make([][]interface{}, len(leads))
	for i, l := range leads {
		rows[i] = []interface{}{
			l.CampaignID, l.TenantID, l.CPF, l.Phone, string(l.Status), l.AttemptsCount, time.Now(), l.Name,
		}
	}

	copyCount, err := r.pool.CopyFrom(
		ctx,
		pgx.Identifier{"leads"},
		[]string{"campaign_id", "tenant_id", "cpf", "phone", "status", "attempts_count", "created_at", "name"},
		pgx.CopyFromRows(rows),
	)
	return copyCount, err
}

// FetchNextFILOBatch busca os leads respeitando o princípio FILO (id DESC) e cooldown de 2h
func (r *LeadRepo) FetchNextFILOBatch(ctx context.Context, campaignID string, limit int, cooldownHours int) ([]*domain.Lead, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}
	cooldownThreshold := time.Now().Add(-time.Duration(cooldownHours) * time.Hour)

	query := `
		SELECT id, campaign_id, tenant_id, cpf, phone, status, attempts_count, last_dialed_at, dialed_at, created_at, COALESCE(name, '') as name
		FROM leads
		WHERE campaign_id = $1
		  AND status IN ('NEW', 'QUEUED')
		  AND (last_dialed_at IS NULL OR last_dialed_at <= $2)
		ORDER BY id DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, campaignID, cooldownThreshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.Lead
	for rows.Next() {
		var l domain.Lead
		var statusStr string
		err := rows.Scan(
			&l.ID, &l.CampaignID, &l.TenantID, &l.CPF, &l.Phone, &statusStr, &l.AttemptsCount,
			&l.LastDialedAt, &l.DialedAt, &l.CreatedAt, &l.Name,
		)
		if err != nil {
			return nil, err
		}
		l.Status = domain.LeadStatus(statusStr)
		list = append(list, &l)
	}

	return list, nil
}

func (r *LeadRepo) MarkDialing(ctx context.Context, leadID int64) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	query := `UPDATE leads SET status = 'DIALING', attempts_count = attempts_count + 1, dialed_at = $2, last_dialed_at = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, leadID, time.Now())
	return err
}

func (r *LeadRepo) MarkCompleted(ctx context.Context, leadID int64, status domain.LeadStatus) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	query := `UPDATE leads SET status = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, leadID, string(status))
	return err
}

func (r *LeadRepo) CountCampaignLeads(ctx context.Context, campaignID string) (total, available, dialed int64, err error) {
	if r.pool == nil {
		return 0, 0, 0, fmt.Errorf("postgres: pool não inicializado")
	}
	query := `
		SELECT 
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status IN ('NEW', 'QUEUED')) AS available,
			COUNT(*) FILTER (WHERE status IN ('DIALING', 'COMPLETED')) AS dialed
		FROM leads
		WHERE campaign_id = $1
	`
	err = r.pool.QueryRow(ctx, query, campaignID).Scan(&total, &available, &dialed)
	return total, available, dialed, err
}

func (r *LeadRepo) ResetCampaignCycle(ctx context.Context, campaignID string) (int, error) {
	if r.pool == nil {
		return 0, fmt.Errorf("postgres: pool não inicializado")
	}
	// Reciclagem FILO: reabre os leads para novo ciclo preservando o histórico de tentativas
	query := `UPDATE leads SET status = 'QUEUED' WHERE campaign_id = $1 AND status != 'COMPLETED'`
	res, err := r.pool.Exec(ctx, query, campaignID)
	if err != nil {
		return 0, err
	}
	return int(res.RowsAffected()), nil
}

// RecoverStaleDialing recupera leads presos em status DIALING por mais de 2 minutos
func (r *LeadRepo) RecoverStaleDialing(ctx context.Context, campaignID string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	query := `UPDATE leads SET status = 'QUEUED' WHERE campaign_id = $1 AND status = 'DIALING' AND (last_dialed_at IS NULL OR last_dialed_at <= NOW() - INTERVAL '2 minutes')`
	_, err := r.pool.Exec(ctx, query, campaignID)
	return err
}
