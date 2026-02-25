package util

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, err error, args ...any)
	Debug(msg string, args ...any)
	Close() error
}
type FileLogger struct {
	handler *slog.Logger
	file    *os.File
}

func NewFileLogger(filePath string, level slog.Level) (Logger, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.New(slog.NewJSONHandler(f, opts))

	return &FileLogger{
		handler: handler,
		file:    f,
	}, nil
}

func (l *FileLogger) Info(msg string, args ...any) {
	l.handler.Info(msg, args...)
}

func (l *FileLogger) Debug(msg string, args ...any) {
	l.handler.Debug(msg, args...)
}

func (l *FileLogger) Error(msg string, err error, args ...any) {
	l.handler.Error(msg, append(args, "error", err.Error())...)
}

func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
