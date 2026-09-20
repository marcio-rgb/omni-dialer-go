package ports

import (
	"context"

	"dialer-go/internal/domain"
)

// SIPConfigRepository define o contrato de persistência da tabela sip_data no PostgreSQL.
type SIPConfigRepository interface {
	GetFile(ctx context.Context, filename string) (*domain.SIPConfigFile, error)
	ListFiles(ctx context.Context) ([]domain.SIPConfigFile, error)
	SaveFile(ctx context.Context, file *domain.SIPConfigFile) error
	DeleteFile(ctx context.Context, filename string) error
}
