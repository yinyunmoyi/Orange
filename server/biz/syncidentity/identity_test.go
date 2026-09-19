package syncidentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentPersistsGeneratedID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-id")
	t.Setenv("CONTEXT_SYNC_SERVER_ID", "")
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

func TestCurrentUsesConfiguredID(t *testing.T) {
	const configured = "0123456789abcdef0123456789abcdef"
	t.Setenv("CONTEXT_SYNC_SERVER_ID", configured)
	ResetForTest()
	got, err := Current()
	if err != nil || got != configured {
		t.Fatalf("Current()=(%q,%v)", got, err)
	}
}

func TestCurrentReplacesInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-id")
	if err := os.WriteFile(path, []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTEXT_SYNC_SERVER_ID", "")
	t.Setenv("CONTEXT_SYNC_ID_FILE", path)
	ResetForTest()
	got, err := Current()
	if err != nil || len(got) != 32 || got == "invalid" {
		t.Fatalf("Current()=(%q,%v)", got, err)
	}
}
