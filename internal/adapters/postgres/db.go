package postgres

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// cleanConnString remove parâmetros específicos do Prisma (ex: ?schema=public) que não são parâmetros de configuração válidos no PostgreSQL nativo.
func cleanConnString(connString string) string {
	u, err := url.Parse(connString)
	if err != nil {
		return connString
	}
	q := u.Query()
	if q.Has("schema") {
		q.Del("schema")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// NewConnectionPool inicializa o pool de conexões otimizado com o PostgreSQL (pgxpool) com tolerância a atrasos no boot.
func NewConnectionPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	cleaned := cleanConnString(connString)
	config, err := pgxpool.ParseConfig(cleaned)
	if err != nil {
		return nil, fmt.Errorf("falha ao interpretar string de conexão do postgres: %w", err)
	}

	config.MaxConns = 50
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 15 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	maxAttempts := 10
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			pingErr := pool.Ping(pingCtx)
			cancel()

			if pingErr == nil {
				autoMigrate(ctx, pool)
				return pool, nil
			}
			pool.Close()
			lastErr = pingErr
		} else {
			lastErr = err
		}

		if attempt < maxAttempts {
			time.Sleep(1500 * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("falha ao conectar no banco dialer_db após %d tentativas: %w", maxAttempts, lastErr)
}

// autoMigrate garante de forma idempotente que colunas essenciais existam sem exigir migrações manuais.
func autoMigrate(ctx context.Context, pool *pgxpool.Pool) {
	migrateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	queries := []string{
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS recording_file VARCHAR(512);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS recording_url VARCHAR(512);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS transcription TEXT;`,
	}
	for _, q := range queries {
		_, _ = pool.Exec(migrateCtx, q)
	}
}
