package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"dialer-go/internal/ports"
)

type StorageAdapter struct {
	endpoint  string
	accessKey string
	secretKey string
	useSSL    bool
	client    *http.Client
}

var _ ports.StoragePort = (*StorageAdapter)(nil)

func NewStorageAdapter(endpoint, accessKey, secretKey string, useSSL bool) *StorageAdapter {
	return &StorageAdapter{
		endpoint:  endpoint,
		accessKey: accessKey,
		secretKey: secretKey,
		useSSL:    useSSL,
		client: &http.Client{
			Timeout: 5 * time.Minute, // Timeout estrito de 5 minutos
		},
	}
}

// DownloadFileStream baixa o arquivo sob streaming direto sem buffering completo no disco
func (sa *StorageAdapter) DownloadFileStream(ctx context.Context, fileURI string) (io.ReadCloser, error) {
	// Se for arquivo local direto (para testes e contingência)
	if strings.HasPrefix(fileURI, "file://") {
		filePath := strings.TrimPrefix(fileURI, "file://")
		return os.Open(filePath)
	}

	// Se for HTTP / MinIO / S3 presigned URL
	reqURL := fileURI
	if strings.HasPrefix(fileURI, "s3://") {
		schema := "http"
		if sa.useSSL {
			schema = "https"
		}
		path := strings.TrimPrefix(fileURI, "s3://")
		reqURL = fmt.Sprintf("%s://%s/%s", schema, sa.endpoint, path)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar requisição de download: %w", err)
	}

	resp, err := sa.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha na conexão com storage: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("storage retornou status HTTP %d", resp.StatusCode)
	}

	return resp.Body, nil
}
