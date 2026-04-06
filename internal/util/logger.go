package util

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	l.handler.Info(withCaller(normalizeMessage(msg), 2), args...)
}

func (l *FileLogger) Debug(msg string, args ...any) {
	l.handler.Debug(withCaller(normalizeMessage(msg), 2), args...)
}

func (l *FileLogger) Error(msg string, err error, args ...any) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	l.handler.Error(withCaller(normalizeMessage(msg), 2), append(args, "error", errText)...)
}

func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func withCaller(msg string, skip int) string {
	pc, file, _, ok := runtime.Caller(skip)
	if !ok {
		return msg
	}

	fn := runtime.FuncForPC(pc)
	location := strings.TrimSpace(file)
	if fn != nil {
		location = fn.Name()
	}

	location = strings.ReplaceAll(location, "/", ".")
	location = strings.TrimLeft(location, ".")

	return fmt.Sprintf("%s.%s", location, msg)
}

func normalizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "event"
	}
	msg = strings.Trim(msg, "[]")
	msg = strings.ReplaceAll(msg, "/", ".")
	msg = strings.ReplaceAll(msg, "  ", " ")
	msg = strings.Trim(msg, ". ")
	if msg == "" {
		return "event"
	}
	return msg
}
