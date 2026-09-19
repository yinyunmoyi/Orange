package syncidentity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"aaa_word/biz/config"
)

var (
	idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	mu        sync.Mutex
	currentID string
)

func Current() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if currentID != "" {
		return currentID, nil
	}
	id, err := loadOrCreate()
	if err != nil {
		return "", err
	}
	currentID = id
	return id, nil
}

func loadOrCreate() (string, error) {
	if configured := strings.TrimSpace(config.Get("CONTEXT_SYNC_SERVER_ID", "")); configured != "" {
		if !idPattern.MatchString(configured) {
			return "", fmt.Errorf("invalid CONTEXT_SYNC_SERVER_ID")
		}
		return configured, nil
	}
	path := config.Get("CONTEXT_SYNC_ID_FILE", "./data/context-sync-server-id")
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); idPattern.MatchString(id) {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read context sync server id: %w", err)
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate context sync server id: %w", err)
	}
	id := hex.EncodeToString(raw[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create context sync id directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("create context sync id temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(id + "\n"); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write context sync id: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync context sync id: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close context sync id: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("commit context sync id: %w", err)
	}
	return id, nil
}

func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	currentID = ""
}
