package syncidentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentPersistsGeneratedID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-id")
	t.Setenv("CONTEXT_SYNC_ID_FILE", path)
	ResetForTest()
	first, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 {
		t.Fatalf("id length=%d", len(first))
	}
	ResetForTest()
	second, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("generated id changed: %q != %q", first, second)
	}
}

func TestCurrentRejectsInvalidIdentityFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-id")
	if err := os.WriteFile(path, []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTEXT_SYNC_ID_FILE", path)
	ResetForTest()
	if got, err := Current(); err == nil || got != "" {
		t.Fatalf("Current()=(%q,%v), want invalid file error", got, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "invalid\n" {
		t.Fatalf("invalid identity file was replaced: %q", data)
	}
}
