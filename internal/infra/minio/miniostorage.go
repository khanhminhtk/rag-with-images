package minio

import (
	"context"
	"io"
	"net/url"
	"time"

	miniosdk "github.com/minio/minio-go/v7"
	// "rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type MinIOStorage struct {
	MinioClient MinioCleant
	appLogger   util.Logger
}

func NewMinIOStorage(minioClient MinioCleant, appLogger util.Logger) *MinIOStorage {
	return &MinIOStorage{
		MinioClient: minioClient,
		appLogger:   appLogger,
	}
}

func (M *MinIOStorage) HealthCheck(ctx context.Context) error {
	ctxCancel, err := M.MinioClient.Client.HealthCheck(
		time.Duration(30 * time.Second),
	)
	if err != nil {
		return err
	}
	defer ctxCancel()
	return nil
}

func (M *MinIOStorage) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := M.MinioClient.Client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		err = M.MinioClient.Client.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (M *MinIOStorage) PutObject(ctx context.Context, input ports.PutObjectInput) (*ports.ObjectInfo, error) {
	uploadInfo, err := M.MinioClient.Client.PutObject(
		ctx,
		input.Bucket,
		input.ObjectKey,
		input.Reader,
		input.Size, miniosdk.PutObjectOptions{
			ContentType:  input.ContentType,
			UserMetadata: input.Metadata,
		},
	)
	if err != nil {
		M.appLogger.Error(
			"put object failed",
			err,
			"bucket", input.Bucket,
			"object_key", input.ObjectKey,
		)
		return nil, err
	}
	M.appLogger.Info(
		"put object success",
		"bucket", input.Bucket,
		"object_key", input.ObjectKey,
		"size", uploadInfo.Size,
		"etag", uploadInfo.ETag,
	)
	return &ports.ObjectInfo{
		Bucket:       input.Bucket,
		ObjectKey:    input.ObjectKey,
		Size:         uploadInfo.Size,
		ETag:         uploadInfo.ETag,
		LastModified: uploadInfo.LastModified,
		ContentType:  input.ContentType,
		Metadata:     input.Metadata,
	}, nil
}

func (M *MinIOStorage) GetObject(ctx context.Context, bucket string, objectKey string) (io.ReadCloser, *ports.ObjectInfo, error) {
	object, err := M.MinioClient.Client.GetObject(ctx, bucket, objectKey, miniosdk.GetObjectOptions{})
	if err != nil {
		M.appLogger.Error("get object failed", err, "bucket", bucket, "object_key", objectKey)
		return nil, nil, err
	}
	objectInfo, err := object.Stat()
	if err != nil {
		M.appLogger.Error("get object stat failed", err, "bucket", bucket, "object_key", objectKey)
		return nil, nil, err
	}
	M.appLogger.Info(
		"get object success",
		"bucket", bucket,
		"object_key", objectKey,
		"size", objectInfo.Size,
		"etag", objectInfo.ETag,
	)
	return object, &ports.ObjectInfo{
		Bucket:       bucket,
		ObjectKey:    objectKey,
		Size:         objectInfo.Size,
		ETag:         objectInfo.ETag,
		LastModified: objectInfo.LastModified,
		ContentType:  objectInfo.ContentType,
		Metadata:     objectInfo.UserMetadata,
	}, nil
}

func (M *MinIOStorage) StatObject(ctx context.Context, bucket string, objectKey string) (*ports.ObjectInfo, error) {
	objectInfo, err := M.MinioClient.Client.StatObject(ctx, bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		M.appLogger.Error("stat object failed", err, "bucket", bucket, "object_key", objectKey)
		return nil, err
	}
	M.appLogger.Info(
		"stat object success",
		"bucket", bucket,
		"object_key", objectKey,
		"size", objectInfo.Size,
		"etag", objectInfo.ETag,
	)
	return &ports.ObjectInfo{
		Bucket:       bucket,
		ObjectKey:    objectKey,
		Size:         objectInfo.Size,
		ETag:         objectInfo.ETag,
		LastModified: objectInfo.LastModified,
		ContentType:  objectInfo.ContentType,
		Metadata:     objectInfo.UserMetadata,
	}, nil
}

func (M *MinIOStorage) DeleteObject(ctx context.Context, bucket string, objectKey string) error {
	err := M.MinioClient.Client.RemoveObject(ctx, bucket, objectKey, miniosdk.RemoveObjectOptions{})
	if err != nil {
		M.appLogger.Error("delete object failed", err, "bucket", bucket, "object_key", objectKey)
		return err
	}
	M.appLogger.Info("delete object success", "bucket", bucket, "object_key", objectKey)
	return nil
}

func (M *MinIOStorage) PresignGetObject(ctx context.Context, bucket string, objectKey string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := M.MinioClient.Client.PresignedGetObject(ctx, bucket, objectKey, expiry, reqParams)
	if err != nil {
		M.appLogger.Error("presign get object failed", err, "bucket", bucket, "object_key", objectKey)
		return "", err
	}
	M.appLogger.Info("presign get object success", "bucket", bucket, "object_key", objectKey, "expiry_seconds", int(expiry.Seconds()))
	return presignedURL.String(), nil
}
