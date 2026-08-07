package oss

import (
	"context"
	"io"
)

type OSSUploader interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64) (string, error)
	GetPresignedURL(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
}
