package ports

import (
	"context"
	"io"
	"time"
)

type PutObjectInput struct {
	Bucket      string
	ObjectKey   string
	Reader      io.Reader
	Size        int64
	ContentType string
	Metadata    map[string]string
}

type ObjectInfo struct {
	Bucket       string
	ObjectKey    string
	Size         int64
	ETag         string
	LastModified time.Time
	ContentType  string
	Metadata     map[string]string
}

type ObjectStorage interface {
	HealthCheck(ctx context.Context) error
	EnsureBucket(ctx context.Context, bucket string) error
	PutObject(ctx context.Context, input PutObjectInput) (*ObjectInfo, error)
	GetObject(ctx context.Context, bucket string, objectKey string) (io.ReadCloser, *ObjectInfo, error)
	StatObject(ctx context.Context, bucket string, objectKey string) (*ObjectInfo, error)
	DeleteObject(ctx context.Context, bucket string, objectKey string) error
	PresignGetObject(ctx context.Context, bucket string, objectKey string, expiry time.Duration) (string, error)
}
