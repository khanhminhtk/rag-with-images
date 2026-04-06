package minio

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
)

type UploadLocalFileToMinIOUseCase struct {
	Bucket       string
	appLogger    util.Logger
	MinioStorage *minio.MinIOStorage
}

func NewUploadLocalFileToMinIOUseCase(bucket string, appLogger util.Logger, minioStorage *minio.MinIOStorage) *UploadLocalFileToMinIOUseCase {
	return &UploadLocalFileToMinIOUseCase{
		Bucket:       bucket,
		appLogger:    appLogger,
		MinioStorage: minioStorage,
	}
}

func (d *UploadLocalFileToMinIOUseCase) getReaderFile(filePath string) (io.ReadCloser, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		d.appLogger.Error("open file failed", err, "file_path", filePath)
		return nil, 0, err
	}

	d.appLogger.Info("open file succeeded", "file_path", filePath)

	info, err := file.Stat()
	if err != nil {
		file.Close()
		d.appLogger.Error("stat file failed", err, "file_path", filePath)
		return nil, 0, err
	}

	size := info.Size()

	d.appLogger.Info("stat file succeeded", "file_path", filePath, "size", size)

	return file, size, nil
}

func getContentType(filename string) string {
	ext := strings.TrimSpace(filepath.Ext(filename))
	if ext != "" {
		return mime.TypeByExtension(ext)
	}
	return ""
}

func (d *UploadLocalFileToMinIOUseCase) sanitizeObjectKey(parsedURL *neturl.URL) string {
	objectKey := path.Base(parsedURL.Path)
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" || objectKey == "." || objectKey == "/" {
		return "uploaded-file"
	}
	return objectKey
}

func (d *UploadLocalFileToMinIOUseCase) buildTemporaryUploadFile(ctx context.Context, folderDownload string, rawURL string) (tempDir string, localFilePath string, objectKey string, err error) {
	if err = os.MkdirAll(folderDownload, 0755); err != nil {
		return "", "", "", err
	}

	parsedURL, err := neturl.Parse(rawURL)
	if err != nil {
		return "", "", "", err
	}
	objectKey = d.sanitizeObjectKey(parsedURL)

	tempDir, err = os.MkdirTemp(folderDownload, "minio-upload-*")
	if err != nil {
		return "", "", "", err
	}
	defer func() {
		if err == nil || tempDir == "" {
			return
		}
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			d.appLogger.Error("cleanup temporary folder on failure failed", removeErr, "temp_dir", tempDir)
		}
	}()

	localFilePath = filepath.Join(tempDir, objectKey)
	dstFile, err := os.Create(localFilePath)
	if err != nil {
		return tempDir, "", "", err
	}
	defer dstFile.Close()

	switch strings.ToLower(parsedURL.Scheme) {
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return tempDir, "", "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return tempDir, "", "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return tempDir, "", "", fmt.Errorf("download failed with status code %d", resp.StatusCode)
		}
		if _, err := io.Copy(dstFile, resp.Body); err != nil {
			return tempDir, "", "", err
		}
	default:
		localSource := rawURL
		if strings.EqualFold(parsedURL.Scheme, "file") {
			localSource = parsedURL.Path
		}
		srcFile, err := os.Open(localSource)
		if err != nil {
			return tempDir, "", "", err
		}
		defer srcFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return tempDir, "", "", err
		}
	}

	return tempDir, localFilePath, objectKey, nil
}

func (d *UploadLocalFileToMinIOUseCase) Execute(ctx context.Context, req *dtos.UploadFileMinioRequest) (io.ReadCloser, error) {
	if req == nil {
		err := fmt.Errorf("request is nil")
		d.appLogger.Error("invalid upload request", err)
		return nil, err
	}

	folderDownload := strings.TrimSpace(req.FolderDownload)
	urlDownload := strings.TrimSpace(req.UrlDownload)
	if folderDownload == "" {
		err := fmt.Errorf("folder download is empty")
		d.appLogger.Error("invalid folder download", err)
		return nil, err
	}
	if urlDownload == "" {
		err := fmt.Errorf("url download is empty")
		d.appLogger.Error("invalid url download", err)
		return nil, err
	}

	tempDir, localFilePath, objectKey, err := d.buildTemporaryUploadFile(ctx, folderDownload, urlDownload)
	if err != nil {
		d.appLogger.Error("prepare temporary upload file failed", err, "url_download", urlDownload)
		return nil, err
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			d.appLogger.Error("cleanup temporary folder failed", removeErr, "temp_dir", tempDir)
			return
		}
		d.appLogger.Info("cleanup temporary folder succeeded", "temp_dir", tempDir)
	}()

	reader, size, err := d.getReaderFile(localFilePath)
	if err != nil {
		d.appLogger.Error("open temporary file for upload failed", err, "file_path", localFilePath)
		return nil, err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			d.appLogger.Error("close temporary file reader failed", closeErr, "file_path", localFilePath)
			return
		}
		d.appLogger.Info("close temporary file reader succeeded", "file_path", localFilePath)
	}()

	input := ports.PutObjectInput{
		Bucket:      d.Bucket,
		ObjectKey:   objectKey,
		Reader:      reader,
		Size:        size,
		ContentType: getContentType(objectKey),
	}

	err = d.MinioStorage.EnsureBucket(ctx, d.Bucket)
	if err != nil {
		d.appLogger.Error("ensure bucket failed", err, "bucket", d.Bucket)
		return nil, err
	}
	_, err = d.MinioStorage.PutObject(ctx, input)
	if err != nil {
		d.appLogger.Error("put object to minio failed", err, "bucket", d.Bucket, "object_key", objectKey)
		return nil, err
	}
	d.appLogger.Info("put object to minio succeeded", "bucket", d.Bucket, "object_key", objectKey)

	return nil, nil
}
