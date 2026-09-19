package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhraseAudioPathIsStableAndOpaque(t *testing.T) {
	first := PhraseAudioPath("look up")
	if first != PhraseAudioPath("look up") {
		t.Fatal("same phrase produced different paths")
	}
	if first == PhraseAudioPath("look after") {
		t.Fatal("different phrases produced the same path")
	}
	if strings.Contains(first, "look up") {
		t.Fatalf("path exposes phrase: %q", first)
	}
	if !strings.HasSuffix(first, ".mp3") {
		t.Fatalf("unexpected extension: %q", first)
	}
}

func TestDeleteRemovesFileAndTreatsMissingAsSuccess_BitsUT(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Delete(path); err != nil {
		t.Fatalf("Delete existing file error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists, stat error = %v", err)
	}
	if err := Delete(path); err != nil {
		t.Fatalf("Delete missing file error = %v", err)
	}
}

func TestDeleteReturnsFilesystemError_BitsUT(t *testing.T) {
	path := filepath.Join(t.TempDir(), "non-empty")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Delete(path); err == nil {
		t.Fatal("Delete non-empty directory error = nil")
	}
}

func TestFavoriteAudioVersionPathSeparatesItemAndVersion_BitsUT(t *testing.T) {
	t.Setenv("AUDIO_DIR", t.TempDir())

	wordV1, err := FavoriteAudioVersionPath("word", 7, "version-one")
	if err != nil {
		t.Fatalf("FavoriteAudioVersionPath word error = %v", err)
	}
	wordV2, err := FavoriteAudioVersionPath("word", 7, "version-two")
	if err != nil {
		t.Fatalf("FavoriteAudioVersionPath word second version error = %v", err)
	}
	phraseV1, err := FavoriteAudioVersionPath("phrase", 7, "version-one")
	if err != nil {
		t.Fatalf("FavoriteAudioVersionPath phrase error = %v", err)
	}
	if wordV1 == wordV2 {
		t.Fatalf("different versions share path %q", wordV1)
	}
	if wordV1 == phraseV1 {
		t.Fatalf("different item types share path %q", wordV1)
	}
	if !strings.HasSuffix(wordV1, ".mp3") {
		t.Fatalf("version path is not mp3: %q", wordV1)
	}
}

func TestSaveFavoriteAudioVersionWritesCompleteFile_BitsUT(t *testing.T) {
	t.Setenv("AUDIO_DIR", t.TempDir())

	path, size, err := SaveFavoriteAudioVersion(
		"phrase",
		9,
		"version-one",
		bytes.NewBufferString("new-audio"),
	)
	if err != nil {
		t.Fatalf("SaveFavoriteAudioVersion error = %v", err)
	}
	if size != int64(len("new-audio")) {
		t.Fatalf("size = %d, want %d", size, len("new-audio"))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(content) != "new-audio" {
		t.Fatalf("content = %q, want new-audio", content)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains, stat error = %v", err)
	}
}

func TestSaveFavoriteAudioVersionRejectsInvalidPathInput_BitsUT(t *testing.T) {
	t.Setenv("AUDIO_DIR", t.TempDir())

	for _, tt := range []struct {
		itemType string
		itemID   int64
		version  string
	}{
		{itemType: "context", itemID: 1, version: "version-one"},
		{itemType: "word", itemID: 0, version: "version-one"},
		{itemType: "word", itemID: 1, version: "../escape"},
	} {
		if _, _, err := SaveFavoriteAudioVersion(
			tt.itemType,
			tt.itemID,
			tt.version,
			bytes.NewBufferString("audio"),
		); err == nil {
			t.Fatalf("SaveFavoriteAudioVersion(%q, %d, %q) error = nil", tt.itemType, tt.itemID, tt.version)
		}
	}
}

func TestSaveContextOriginalAudio_BitsUT(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AUDIO_DIR", root)
	key := strings.Repeat("b", 64)

	path, size, err := SaveContextOriginalAudio(key, strings.NewReader("opus-webm"))
	if err != nil {
		t.Fatalf("SaveContextOriginalAudio() error = %v", err)
	}
	if size != int64(len("opus-webm")) {
		t.Fatalf("size = %d", size)
	}
	wantPath := filepath.Join(root, "contexts", "bb", "bb", key+".webm")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "opus-webm" {
		t.Fatalf("stored content = %q, err = %v", content, err)
	}
}
