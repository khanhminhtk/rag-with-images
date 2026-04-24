package trainingfile

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func discoverProcessedArtifacts(processDir string) (string, []string, []string, error) {
	markdownPath := ""
	imagePaths := make([]string, 0)
	artifactPaths := make([]string, 0)

	err := filepath.WalkDir(processDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".md", ".markdown":
			artifactPaths = append(artifactPaths, path)
			if markdownPath == "" {
				markdownPath = path
			}
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
			imagePaths = append(imagePaths, path)
			artifactPaths = append(artifactPaths, path)
		}
		return nil
	})
	if err != nil {
		return "", nil, nil, err
	}
	if markdownPath == "" {
		return "", nil, nil, errors.New("no markdown file found in processed directory")
	}
	return markdownPath, imagePaths, artifactPaths, nil
}

func (uc *trainingFileUseCase) uploadArtifactsToMinio(ctx context.Context, uploadDir string, filePaths []string) (int, error) {
	uploaded := 0
	for _, p := range filePaths {
		path := strings.TrimSpace(p)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return uploaded, err
		}
		fileURL := (&url.URL{
			Scheme: "file",
			Path:   filepath.ToSlash(absPath),
		}).String()
		ok, err := uc.UploadToMinio(ctx, uploadDir, fileURL)
		if err != nil {
			return uploaded, err
		}
		if ok {
			uploaded++
		}
	}
	return uploaded, nil
}

func resolveImagePath(markdownPath, rawPath string) string {
	clean := strings.TrimSpace(rawPath)
	if clean == "" {
		return ""
	}
	if filepath.IsAbs(clean) {
		if _, err := os.Stat(clean); err == nil {
			return clean
		}
		return ""
	}
	base := filepath.Dir(markdownPath)
	candidate := filepath.Clean(filepath.Join(base, clean))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
