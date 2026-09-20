package ports

import (
	"context"
	"io"
)

type StoragePort interface {
	DownloadFileStream(ctx context.Context, fileURI string) (io.ReadCloser, error)
}
