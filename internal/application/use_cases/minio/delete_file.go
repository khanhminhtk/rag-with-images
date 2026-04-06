package minio

import (
	"context"
	"fmt"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
)

type DeleteFileInputUseCase struct {
	Bucket       string
	appLogger    util.Logger
	MinioStorage *minio.MinIOStorage
}

func (d *DeleteFileInputUseCase) NewDeleteFileInputUseCase(bucket string, appLogger util.Logger, minioStorage *minio.MinIOStorage) *DeleteFileInputUseCase {
	return &DeleteFileInputUseCase{
		Bucket:       bucket,
		appLogger:    appLogger,
		MinioStorage: minioStorage,
	}
}

func (d *DeleteFileInputUseCase) Execute(ctx context.Context, req *dtos.DeleteFileMinioRequest) error {
	if req == nil {
		err := fmt.Errorf("request is nil")
		d.appLogger.Error("invalid delete file request", err)
		return err
	}

	objectKey := strings.TrimSpace(req.ObjectKey)
	if objectKey == "" {
		err := fmt.Errorf("object key is empty")
		d.appLogger.Error("invalid object key", err)
		return err
	}

	if _, err := d.MinioStorage.StatObject(ctx, d.Bucket, objectKey); err != nil {
		d.appLogger.Error("stat object failed before delete", err, "bucket", d.Bucket, "object_key", objectKey)
		return err
	}

	if err := d.MinioStorage.DeleteObject(ctx, d.Bucket, objectKey); err != nil {
		d.appLogger.Error("delete object failed", err, "bucket", d.Bucket, "object_key", objectKey)
		return err
	}

	d.appLogger.Info("delete object succeeded", "bucket", d.Bucket, "object_key", objectKey)
	return nil
}
