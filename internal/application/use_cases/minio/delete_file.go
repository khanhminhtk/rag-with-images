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
		d.appLogger.Error("internal.application.use_cases.minio.delete_file.Execute: Invalid request due to: ", err)
		return err
	}

	objectKey := strings.TrimSpace(req.ObjectKey)
	if objectKey == "" {
		err := fmt.Errorf("object key is empty")
		d.appLogger.Error("internal.application.use_cases.minio.delete_file.Execute: Invalid object key due to: ", err)
		return err
	}

	if _, err := d.MinioStorage.StatObject(ctx, d.Bucket, objectKey); err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.delete_file.Execute: Don't stat object from minio due to: ", err)
		return err
	}

	if err := d.MinioStorage.DeleteObject(ctx, d.Bucket, objectKey); err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.delete_file.Execute: Don't delete object from minio due to: ", err)
		return err
	}

	d.appLogger.Info("internal.application.use_cases.minio.delete_file.Execute: Delete object from minio successfully, bucket: ", d.Bucket, " objectKey: ", objectKey)
	return nil
}
