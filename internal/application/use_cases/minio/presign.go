package minio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
)

type PresignGetObjectUseCase struct {
	Bucket        string
	PresignExpiry time.Duration
	appLogger     util.Logger
	MinioStorage  *minio.MinIOStorage
}

func (p *PresignGetObjectUseCase) NewPresignGetObjectUseCase(bucket string, presignExpiry time.Duration, appLogger util.Logger, minioStorage *minio.MinIOStorage) *PresignGetObjectUseCase {
	return &PresignGetObjectUseCase{
		Bucket:        bucket,
		PresignExpiry: presignExpiry,
		appLogger:     appLogger,
		MinioStorage:  minioStorage,
	}
}

func (p *PresignGetObjectUseCase) Execute(ctx context.Context, req *dtos.PresignGetObjectMinioRequest) (string, error) {
	objectKey := strings.TrimSpace(req.ObjectKey)
	if objectKey == "" {
		err := fmt.Errorf("object key is empty")
		p.appLogger.Error("presign request invalid object key", err, "bucket", p.Bucket)
		return "", err
	}

	if p.PresignExpiry <= 0 {
		err := fmt.Errorf("presign expiry must be greater than zero")
		p.appLogger.Error("presign request invalid expiry", err, "bucket", p.Bucket)
		return "", err
	}

	if _, err := p.MinioStorage.StatObject(ctx, p.Bucket, objectKey); err != nil {
		p.appLogger.Error("presign stat object failed", err, "bucket", p.Bucket, "object_key", objectKey)
		return "", err
	}

	url, err := p.MinioStorage.PresignGetObject(ctx, p.Bucket, objectKey, p.PresignExpiry)
	if err != nil {
		p.appLogger.Error("presign create url failed", err, "bucket", p.Bucket, "object_key", objectKey)
		return "", err
	}

	p.appLogger.Info(
		"presign get object success",
		"bucket", p.Bucket,
		"object_key", objectKey,
		"expiry_seconds", int(p.PresignExpiry.Seconds()),
	)
	return url, nil
}
