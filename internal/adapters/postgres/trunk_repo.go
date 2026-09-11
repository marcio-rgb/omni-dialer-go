package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TrunkRepo struct {
	pool *pgxpool.Pool
}

var _ ports.TrunkRepository = (*TrunkRepo)(nil)

func NewTrunkRepo(pool *pgxpool.Pool) *TrunkRepo {
	return &TrunkRepo{pool: pool}
}

func (r *TrunkRepo) Create(ctx context.Context, t *domain.Trunk) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	codecsJSON, err := json.Marshal(t.Codecs)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO trunks (
			id, tenant_id, name, direction, registration_mode, host, port, outbound_proxy, tech_prefix,
			auth_username, auth_password, auth_realm, from_user, from_domain, user_agent, transport, nat_mode,
			direct_media, codecs, dtmf_mode, qualify_frequency, qualify_timeout, max_channels, is_enabled
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24
		)
	`

	_, err = r.pool.Exec(ctx, query,
		t.ID, t.TenantID, t.Name, t.Direction, t.RegistrationMode, t.Host, t.Port, t.OutboundProxy, t.TechPrefix,
		t.AuthUsername, t.AuthPassword, t.AuthRealm, t.FromUser, t.FromDomain, t.UserAgent, t.Transport, t.NATMode,
		t.DirectMedia, codecsJSON, t.DTMFMode, t.QualifyFrequency, t.QualifyTimeout, t.MaxChannels, t.IsEnabled,
	)
	return err
}

func (r *TrunkRepo) GetByID(ctx context.Context, tenantID, trunkID string) (*domain.Trunk, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, name, direction, registration_mode, host, port, outbound_proxy, tech_prefix,
		       auth_username, auth_password, auth_realm, from_user, from_domain, user_agent, transport, nat_mode,
		       direct_media, codecs, dtmf_mode, qualify_frequency, qualify_timeout, max_channels, is_enabled,
		       created_at, updated_at
		FROM trunks
		WHERE id = $1 AND tenant_id = $2
	`

	var t domain.Trunk
	var codecsJSON []byte

	err := r.pool.QueryRow(ctx, query, trunkID, tenantID).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Direction, &t.RegistrationMode, &t.Host, &t.Port, &t.OutboundProxy, &t.TechPrefix,
		&t.AuthUsername, &t.AuthPassword, &t.AuthRealm, &t.FromUser, &t.FromDomain, &t.UserAgent, &t.Transport, &t.NATMode,
		&t.DirectMedia, &codecsJSON, &t.DTMFMode, &t.QualifyFrequency, &t.QualifyTimeout, &t.MaxChannels, &t.IsEnabled,
		&t.CreatedAt, &t.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal(codecsJSON, &t.Codecs)
	return &t, nil
}

func (r *TrunkRepo) ListByTenant(ctx context.Context, tenantID string) ([]*domain.Trunk, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, name, direction, registration_mode, host, port, outbound_proxy, tech_prefix,
		       auth_username, auth_password, auth_realm, from_user, from_domain, user_agent, transport, nat_mode,
		       direct_media, codecs, dtmf_mode, qualify_frequency, qualify_timeout, max_channels, is_enabled,
		       created_at, updated_at
		FROM trunks
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.Trunk
	for rows.Next() {
		var t domain.Trunk
		var codecsJSON []byte
		err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.Direction, &t.RegistrationMode, &t.Host, &t.Port, &t.OutboundProxy, &t.TechPrefix,
			&t.AuthUsername, &t.AuthPassword, &t.AuthRealm, &t.FromUser, &t.FromDomain, &t.UserAgent, &t.Transport, &t.NATMode,
			&t.DirectMedia, &codecsJSON, &t.DTMFMode, &t.QualifyFrequency, &t.QualifyTimeout, &t.MaxChannels, &t.IsEnabled,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(codecsJSON, &t.Codecs)
		list = append(list, &t)
	}

	return list, nil
}

func (r *TrunkRepo) Update(ctx context.Context, t *domain.Trunk) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	codecsJSON, err := json.Marshal(t.Codecs)
	if err != nil {
		return err
	}

	query := `
		UPDATE trunks SET
			name = $3, direction = $4, registration_mode = $5, host = $6, port = $7, outbound_proxy = $8,
			tech_prefix = $9, auth_username = $10, auth_password = $11, auth_realm = $12, from_user = $13,
			from_domain = $14, user_agent = $15, transport = $16, nat_mode = $17, codecs = $18, dtmf_mode = $19,
			qualify_frequency = $20, qualify_timeout = $21, max_channels = $22, is_enabled = $23,
			updated_at = $24
		WHERE id = $1 AND tenant_id = $2
	`

	res, err := r.pool.Exec(ctx, query,
		t.ID, t.TenantID, t.Name, t.Direction, t.RegistrationMode, t.Host, t.Port, t.OutboundProxy,
		t.TechPrefix, t.AuthUsername, t.AuthPassword, t.AuthRealm, t.FromUser, t.FromDomain, t.UserAgent, t.Transport,
		t.NATMode, codecsJSON, t.DTMFMode, t.QualifyFrequency, t.QualifyTimeout, t.MaxChannels, t.IsEnabled,
		time.Now(),
	)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("tronco não encontrado")
	}
	return nil
}

func (r *TrunkRepo) Delete(ctx context.Context, tenantID, trunkID string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `DELETE FROM trunks WHERE id = $1 AND tenant_id = $2`
	res, err := r.pool.Exec(ctx, query, trunkID, tenantID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("tronco não encontrado")
	}
	return nil
}

func (r *TrunkRepo) ListAllEnabled(ctx context.Context) ([]*domain.Trunk, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT id, tenant_id, name, direction, registration_mode, host, port, outbound_proxy, tech_prefix,
		       auth_username, auth_password, auth_realm, from_user, from_domain, user_agent, transport, nat_mode,
		       direct_media, codecs, dtmf_mode, qualify_frequency, qualify_timeout, max_channels, is_enabled,
		       created_at, updated_at
		FROM trunks
		WHERE is_enabled = true
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.Trunk
	for rows.Next() {
		var t domain.Trunk
		var codecsJSON []byte
		err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.Direction, &t.RegistrationMode, &t.Host, &t.Port, &t.OutboundProxy, &t.TechPrefix,
			&t.AuthUsername, &t.AuthPassword, &t.AuthRealm, &t.FromUser, &t.FromDomain, &t.UserAgent, &t.Transport, &t.NATMode,
			&t.DirectMedia, &codecsJSON, &t.DTMFMode, &t.QualifyFrequency, &t.QualifyTimeout, &t.MaxChannels, &t.IsEnabled,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(codecsJSON, &t.Codecs)
		list = append(list, &t)
	}

	return list, nil
}
