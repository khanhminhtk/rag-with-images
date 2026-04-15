package bootstrap

import (
	"os"
	"path/filepath"
	"runtime"
)

func ResolveProjectRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func ProjectPath(parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	segments = append(segments, ResolveProjectRoot())
	segments = append(segments, parts...)
	return filepath.Join(segments...)
}
