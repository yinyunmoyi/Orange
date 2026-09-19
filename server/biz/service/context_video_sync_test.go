package service

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"aaa_word/biz/db"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetContextVideoSyncManifestQueriesCandidatesOnce(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	videoPath := filepath.Join(t.TempDir(), "video.webm")
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, learningLocation())
	due := now.Add(12 * time.Hour)
	created := now.Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{
			"video_id", "context_id", "item_type", "item_id",
			"duration_ms", "file_size", "file_path", "version",
			"created_at", "due_at", "priority",
		}).AddRow(
			9, 7, ContextItemWord, 11,
			8_000, 5, videoPath, "version-one",
			created, due, 0,
		))

	data, err := getContextVideoSyncManifest(
		context.Background(),
		"server-id",
		7,
		1000,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("items=%+v", data.Items)
	}
	item := data.Items[0]
	if item.VideoID != 9 || item.Priority != 0 ||
		item.VideoURL != "/api/v1/context-videos/9/video" ||
		item.SubtitleURL != "/api/v1/context-videos/9/subtitles.vtt" {
		t.Fatalf("item=%+v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetContextVideoSyncManifestSkipsMissingFile(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	now := time.Date(2026, 8, 16, 12, 0, 0, 0, learningLocation())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{
			"video_id", "context_id", "item_type", "item_id",
			"duration_ms", "file_size", "file_path", "version",
			"created_at", "due_at", "priority",
		}).AddRow(
			9, 7, ContextItemPhrase, 11,
			8_000, 5, filepath.Join(t.TempDir(), "missing.webm"), "version-one",
			now, nil, 1,
		))

	data, err := getContextVideoSyncManifest(
		context.Background(),
		"server-id",
		7,
		1000,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 0 {
		t.Fatalf("items=%+v", data.Items)
	}
}

func TestGetContextVideoSyncManifestRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		serverID string
		days     int
		limit    int
	}{
		{"", 7, 1000},
		{"server", 0, 1000},
		{"server", 8, 1000},
		{"server", 7, 0},
		{"server", 7, 1001},
	} {
		if _, err := getContextVideoSyncManifest(
			context.Background(),
			test.serverID,
			test.days,
			test.limit,
			time.Now(),
		); err != ErrLearningInvalid {
			t.Fatalf("input=%+v err=%v", test, err)
		}
	}
}
