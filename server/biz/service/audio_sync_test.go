package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"aaa_word/biz/db"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetAudioSyncManifestIncludesWordAndPhrase(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	dir := t.TempDir()
	wordPath := filepath.Join(dir, "word.mp3")
	phrasePath := filepath.Join(dir, "phrase.mp3")
	if err := os.WriteFile(wordPath, []byte("word-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(phrasePath, []byte("phrase-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM `word_audios`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "word_id", "accent", "source_url", "file_path", "content_type", "file_size",
		}).AddRow(3, 7, "US", "tts://us/test", wordPath, "audio/mpeg", len("word-audio")))
	mock.ExpectQuery("SELECT .* FROM `phrases`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "phrase", "audio_file_path", "audio_content_type", "audio_file_size",
		}).AddRow(9, "look up", phrasePath, "audio/mpeg", len("phrase-audio")))

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	data, err := GetAudioSyncManifest(context.Background(), "server", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 2 {
		t.Fatalf("items=%+v", data.Items)
	}
	word, phrase := data.Items[0], data.Items[1]
	if word.ItemType != ContextItemWord || word.ItemID != 7 || word.AudioID != 3 ||
		word.PlaybackURL != "/api/v1/word/audio?src=tts%3A%2F%2Fus%2Ftest" {
		t.Fatalf("word=%+v", word)
	}
	if phrase.ItemType != ContextItemPhrase || phrase.ItemID != 9 || phrase.AudioID != 0 ||
		phrase.AudioURL == "" || phrase.PlaybackURL != "/api/v1/phrase/9/audio" {
		t.Fatalf("phrase=%+v", phrase)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAudioSyncManifestSkipsUnhealthyFile(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	mock.ExpectQuery("SELECT .* FROM `word_audios`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "word_id", "accent", "source_url", "file_path", "content_type", "file_size",
		}).AddRow(3, 7, "US", "tts://us/test", filepath.Join(t.TempDir(), "missing.mp3"), "audio/mpeg", 5))
	mock.ExpectQuery("SELECT .* FROM `phrases`").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "phrase", "audio_file_path", "audio_content_type", "audio_file_size",
		}))

	data, err := GetAudioSyncManifest(context.Background(), "server", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 0 {
		t.Fatalf("items=%+v", data.Items)
	}
}

func TestAudioSyncVersionChangesWithFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := audioSyncFile{
		ItemType: ContextItemWord,
		ItemID:   1,
		AudioID:  2,
		FilePath: path,
		FileSize: 3,
	}
	firstStat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	first := audioSyncVersion(file, firstStat)
	changedAt := firstStat.ModTime().Add(time.Second)
	if changeErr := os.Chtimes(path, changedAt, changedAt); changeErr != nil {
		t.Fatal(changeErr)
	}
	secondStat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	second := audioSyncVersion(file, secondStat)
	if first == second || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(second) {
		t.Fatalf("versions first=%q second=%q", first, second)
	}
}

func TestLookupAudioSyncFileRejectsStaleVersion(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	path := filepath.Join(t.TempDir(), "word.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM `word_audios` WHERE id = .*LIMIT").
		WithArgs(int64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "word_id", "accent", "source_url", "file_path", "content_type", "file_size",
		}).AddRow(3, 7, "US", "tts://us/test", path, "audio/mpeg", len("audio")))

	_, _, current, err := LookupAudioSyncFile(
		context.Background(),
		ContextItemWord,
		3,
		"stale-version",
	)

	if !errors.Is(err, ErrAudioSyncNotFound) || current == "" {
		t.Fatalf("version=%q err=%v", current, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
