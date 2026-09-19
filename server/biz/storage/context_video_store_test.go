package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveContextVideo_BitsUT(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CONTEXT_VIDEO_DIR", root)
	key := strings.Repeat("a", 64)

	path, size, err := SaveContextVideo(key, strings.NewReader("webm-data"))
	if err != nil {
		t.Fatalf("SaveContextVideo() error = %v", err)
	}
	if size != int64(len("webm-data")) {
		t.Fatalf("size = %d", size)
	}
	wantPath := filepath.Join(root, "aa", "aa", key+".webm")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "webm-data" {
		t.Fatalf("stored content = %q, err = %v", content, err)
	}
}

func TestContextVideoPathRejectsInvalidKey_BitsUT(t *testing.T) {
	if _, err := ContextVideoPath("not-a-key"); err == nil {
		t.Fatal("ContextVideoPath() error = nil")
	}
}
