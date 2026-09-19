package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aaa_word/biz/db"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRegenerateFavoriteAudioRejectsInvalidRequest_BitsUT(t *testing.T) {
	for _, tt := range []struct {
		itemType string
		itemID   int64
	}{
		{itemType: "context", itemID: 1},
		{itemType: ContextItemWord, itemID: 0},
		{itemType: ContextItemPhrase, itemID: -1},
	} {
		_, err := regenerateFavoriteAudio(
			context.Background(),
			tt.itemType,
			tt.itemID,
			"version-one",
			fakeFavoriteAudioSynth("new-audio", nil),
		)
		if !errors.Is(err, ErrFavoriteAudioInvalid) {
			t.Fatalf("regenerateFavoriteAudio(%q, %d) error = %v, want %v",
				tt.itemType, tt.itemID, err, ErrFavoriteAudioInvalid)
		}
	}
}

func TestRegenerateFavoriteAudioMissingItem_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT").
		WithArgs(int64(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phrase"}))

	_, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemPhrase,
		99,
		"version-missing",
		fakeFavoriteAudioSynth("new-audio", nil),
	)
	if !errors.Is(err, ErrFavoriteAudioNotFound) {
		t.Fatalf("regenerateFavoriteAudio missing error = %v, want %v", err, ErrFavoriteAudioNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateFavoriteAudioPhraseSwitchesFileAfterCommit_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	t.Setenv("AUDIO_DIR", audioDir)
	oldPath := writeFavoriteAudioTestFile(t, audioDir, "old-phrase.mp3", "old-audio")

	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "phrase", "audio_file_path", "audio_content_type", "audio_file_size",
		}).AddRow(9, "look up", oldPath, "audio/mpeg", 9))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phrase"}).AddRow(9, "look up"))
	mock.ExpectExec("UPDATE `phrases` SET .*audio_file_path.*audio_file_size.* WHERE id =").
		WithArgs("audio/mpeg", sqlmock.AnyArg(), int64(len("new-audio")), sqlmock.AnyArg(), int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	data, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemPhrase,
		9,
		"version-one",
		fakeFavoriteAudioSynth("new-audio", nil),
	)
	if err != nil {
		t.Fatalf("regenerateFavoriteAudio phrase error = %v", err)
	}
	if data.AudioURL != "/api/v1/phrase/9/audio?v=version-one" {
		t.Fatalf("audio URL = %q", data.AudioURL)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old audio still exists, stat error = %v", err)
	}
	newPath := filepath.Join(audioDir, "favorites", "phrase", "9", "version-one.mp3")
	content, readErr := os.ReadFile(newPath)
	if readErr != nil {
		t.Fatalf("read new audio: %v", readErr)
	}
	if string(content) != "new-audio" {
		t.Fatalf("new audio = %q", content)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateFavoriteAudioWordUpdatesExistingUSAudio_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	t.Setenv("AUDIO_DIR", audioDir)
	oldPath := writeFavoriteAudioTestFile(t, audioDir, "old-word.mp3", "old-audio")
	sourceURL := "tts://us/look"

	mock.ExpectQuery("SELECT .* FROM `words` WHERE id = .*LIMIT").
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word"}).AddRow(7, "look"))
	mock.ExpectQuery("SELECT .* FROM `word_audios` WHERE word_id = .*UPPER\\(accent\\) = .*LIMIT").
		WithArgs(int64(7), "US", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "word_id", "accent", "phonetic_text", "source_url", "file_path", "content_type", "file_size",
		}).AddRow(12, 7, "US", "/lʊk/", sourceURL, oldPath, "audio/mpeg", 9))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `words` WHERE id = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word"}).AddRow(7, "look"))
	mock.ExpectQuery("SELECT .* FROM `word_audios` WHERE word_id = .*UPPER\\(accent\\) = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(7), "US", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "word_id", "accent", "phonetic_text", "source_url", "file_path",
		}).AddRow(12, 7, "US", "/lʊk/", sourceURL, oldPath))
	mock.ExpectExec("UPDATE `word_audios` SET .*content_type.*file_path.*file_size.*source_url.* WHERE id =").
		WithArgs("audio/mpeg", sqlmock.AnyArg(), int64(len("new-word")), sourceURL, int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	data, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemWord,
		7,
		"version-word",
		fakeFavoriteAudioSynth("new-word", nil),
	)
	if err != nil {
		t.Fatalf("regenerateFavoriteAudio word error = %v", err)
	}
	if !strings.Contains(data.AudioURL, "src=tts%3A%2F%2Fus%2Flook") ||
		!strings.HasSuffix(data.AudioURL, "&v=version-word") {
		t.Fatalf("audio URL = %q", data.AudioURL)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old audio still exists, stat error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateFavoriteAudioWordCreatesMissingUSAudio_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	t.Setenv("AUDIO_DIR", t.TempDir())
	mock.ExpectQuery("SELECT .* FROM `words` WHERE id = .*LIMIT").
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word"}).AddRow(7, "look"))
	mock.ExpectQuery("SELECT .* FROM `word_audios` WHERE word_id = .*UPPER\\(accent\\) = .*LIMIT").
		WithArgs(int64(7), "US", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `words` WHERE id = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word"}).AddRow(7, "look"))
	mock.ExpectQuery("SELECT .* FROM `word_audios` WHERE word_id = .*UPPER\\(accent\\) = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(7), "US", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO `word_audios`").
		WithArgs(int64(7), "US", "", "tts://us/look", sqlmock.AnyArg(), "audio/mpeg", int64(len("new-word")), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(13, 1))
	mock.ExpectCommit()

	if _, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemWord,
		7,
		"version-create",
		fakeFavoriteAudioSynth("new-word", nil),
	); err != nil {
		t.Fatalf("regenerateFavoriteAudio create US error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateFavoriteAudioSynthesisFailureKeepsOldAudio_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	t.Setenv("AUDIO_DIR", audioDir)
	oldPath := writeFavoriteAudioTestFile(t, audioDir, "old-phrase.mp3", "old-audio")
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "phrase", "audio_file_path", "audio_content_type", "audio_file_size",
		}).AddRow(9, "look up", oldPath, "audio/mpeg", 9))

	synthErr := errors.New("tts unavailable")
	_, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemPhrase,
		9,
		"version-failed",
		fakeFavoriteAudioSynth("", synthErr),
	)
	if !errors.Is(err, synthErr) {
		t.Fatalf("regenerateFavoriteAudio error = %v, want %v", err, synthErr)
	}
	if content, readErr := os.ReadFile(oldPath); readErr != nil || string(content) != "old-audio" {
		t.Fatalf("old audio changed content=%q err=%v", content, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(audioDir, "favorites", "phrase", "9", "version-failed.mp3")); !os.IsNotExist(statErr) {
		t.Fatalf("failed synthesis wrote new file, stat error = %v", statErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateFavoriteAudioSaveFailureKeepsOldAudio_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	root := t.TempDir()
	audioDirFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(audioDirFile, []byte("block mkdir"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUDIO_DIR", audioDirFile)
	oldPath := writeFavoriteAudioTestFile(t, root, "old-phrase.mp3", "old-audio")
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "phrase", "audio_file_path", "audio_content_type", "audio_file_size",
		}).AddRow(9, "look up", oldPath, "audio/mpeg", 9))

	if _, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemPhrase,
		9,
		"version-save-failed",
		fakeFavoriteAudioSynth("new-audio", nil),
	); err == nil {
		t.Fatal("regenerateFavoriteAudio save error = nil")
	}
	if content, readErr := os.ReadFile(oldPath); readErr != nil || string(content) != "old-audio" {
		t.Fatalf("old audio changed content=%q err=%v", content, readErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateFavoriteAudioTransactionFailureCleansNewAndKeepsOld_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newFavoriteAudioMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	t.Setenv("AUDIO_DIR", audioDir)
	oldPath := writeFavoriteAudioTestFile(t, audioDir, "old-phrase.mp3", "old-audio")
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "phrase", "audio_file_path", "audio_content_type", "audio_file_size",
		}).AddRow(9, "look up", oldPath, "audio/mpeg", 9))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phrase"}).AddRow(9, "look up"))
	mock.ExpectExec("UPDATE `phrases` SET").
		WillReturnError(errors.New("db update failed"))
	mock.ExpectRollback()

	if _, err := regenerateFavoriteAudio(
		context.Background(),
		ContextItemPhrase,
		9,
		"version-rollback",
		fakeFavoriteAudioSynth("new-audio", nil),
	); err == nil {
		t.Fatal("regenerateFavoriteAudio transaction error = nil")
	}
	if content, readErr := os.ReadFile(oldPath); readErr != nil || string(content) != "old-audio" {
		t.Fatalf("old audio changed content=%q err=%v", content, readErr)
	}
	newPath := filepath.Join(audioDir, "favorites", "phrase", "9", "version-rollback.mp3")
	if _, statErr := os.Stat(newPath); !os.IsNotExist(statErr) {
		t.Fatalf("rolled back new file remains, stat error = %v", statErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func fakeFavoriteAudioSynth(content string, err error) favoriteAudioSynthFunc {
	return func(text, accent string) ([]byte, string, error) {
		if err != nil {
			return nil, "", err
		}
		return []byte(content), "audio/mpeg", nil
	}
}

func writeFavoriteAudioTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newFavoriteAudioMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatal(err)
	}
	return gormDB, mock, func() { _ = sqlDB.Close() }
}
