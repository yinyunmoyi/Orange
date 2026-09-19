package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"aaa_word/biz/config"
)

var contextVideoKeyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ContextVideoDir() string {
	return config.ContextVideoDir()
}

func ContextVideoPath(dedupeKey string) (string, error) {
	if !contextVideoKeyPattern.MatchString(dedupeKey) {
		return "", fmt.Errorf("invalid context video key")
	}
	return filepath.Join(
		ContextVideoDir(),
		dedupeKey[:2],
		dedupeKey[2:4],
		dedupeKey+".webm",
	), nil
}

func SaveContextVideo(dedupeKey string, body io.Reader) (path string, size int64, err error) {
	path, err = ContextVideoPath(dedupeKey)
	if err != nil {
		return "", 0, err
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755); mkdirErr != nil {
		return "", 0, fmt.Errorf("mkdir context video: %w", mkdirErr)
	}

	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", 0, fmt.Errorf("create context video tmp: %w", err)
	}
	tmpPath := file.Name()

	size, err = io.Copy(file, body)
	if closeErr := file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("copy context video: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("rename context video: %w", err)
	}
	return path, size, nil
}
