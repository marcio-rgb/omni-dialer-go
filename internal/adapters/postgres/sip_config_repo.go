package postgres

import (
	"context"
	"fmt"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SIPConfigRepo implementa a persistência da tabela sip_data no PostgreSQL.
//
// @pattern Repository Pattern
// @governedBy /database/schema.sql
type SIPConfigRepo struct {
	pool *pgxpool.Pool
}

var _ ports.SIPConfigRepository = (*SIPConfigRepo)(nil)

func NewSIPConfigRepo(pool *pgxpool.Pool) *SIPConfigRepo {
	return &SIPConfigRepo{pool: pool}
}

// GetFile busca o conteúdo de um arquivo na tabela sip_data pelo nome.
func (r *SIPConfigRepo) GetFile(ctx context.Context, filename string) (*domain.SIPConfigFile, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT file, data, updated_at
		FROM sip_data
		WHERE file = $1
	`

	var item domain.SIPConfigFile
	err := r.pool.QueryRow(ctx, query, filename).Scan(&item.File, &item.Data, &item.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: erro ao buscar arquivo %s em sip_data: %w", filename, err)
	}

	return &item, nil
}

// ListFiles retorna todos os arquivos cadastrados na tabela sip_data.
func (r *SIPConfigRepo) ListFiles(ctx context.Context) ([]domain.SIPConfigFile, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("postgres: pool não inicializado")
	}

	query := `
		SELECT file, data, updated_at
		FROM sip_data
		ORDER BY file ASC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: erro ao listar arquivos de sip_data: %w", err)
	}
	defer rows.Close()

	var results []domain.SIPConfigFile
	for rows.Next() {
		var item domain.SIPConfigFile
		if err := rows.Scan(&item.File, &item.Data, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: erro ao ler linha de sip_data: %w", err)
		}
		results = append(results, item)
	}

	return results, nil
}

// SaveFile insere ou atualiza um arquivo na tabela sip_data (UPSERT).
func (r *SIPConfigRepo) SaveFile(ctx context.Context, file *domain.SIPConfigFile) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}
	if file == nil || file.File == "" {
		return fmt.Errorf("postgres: nome de arquivo invalido")
	}

	query := `
		INSERT INTO sip_data (file, data, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (file) DO UPDATE SET
			data = EXCLUDED.data,
			updated_at = NOW()
	`

	_, err := r.pool.Exec(ctx, query, file.File, file.Data)
	if err != nil {
		return fmt.Errorf("postgres: erro ao salvar arquivo %s em sip_data: %w", file.File, err)
	}

	return nil
}

// DeleteFile remove um arquivo da tabela sip_data.
func (r *SIPConfigRepo) DeleteFile(ctx context.Context, filename string) error {
	if r.pool == nil {
		return fmt.Errorf("postgres: pool não inicializado")
	}

	query := `DELETE FROM sip_data WHERE file = $1`
	_, err := r.pool.Exec(ctx, query, filename)
	if err != nil {
		return fmt.Errorf("postgres: erro ao deletar arquivo %s em sip_data: %w", filename, err)
	}

	return nil
}
