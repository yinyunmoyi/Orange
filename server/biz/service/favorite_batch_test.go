package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNormalizeFavoriteImportText_BitsUT(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantType string
		wantText string
		wantErr  bool
	}{
		{name: "word", raw: "  Hello  ", wantType: ContextItemWord, wantText: "hello"},
		{name: "phrase", raw: " Look\t  Up ", wantType: ContextItemPhrase, wantText: "look up"},
		{name: "invalid word", raw: "hello!", wantErr: true},
		{name: "invalid phrase", raw: "one two three four five six seven eight nine", wantErr: true},
		{name: "blank", raw: " \n ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotText, err := NormalizeFavoriteImportText(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeFavoriteImportText(%q) error = nil", tt.raw)
				}
				return
			}
			if err != nil || gotType != tt.wantType || gotText != tt.wantText {
				t.Fatalf("NormalizeFavoriteImportText(%q) = (%q, %q, %v), want (%q, %q, nil)",
					tt.raw, gotType, gotText, err, tt.wantType, tt.wantText)
			}
		})
	}
}

func TestFavoriteAudioHealthy_BitsUT(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.mp3")
	if err := os.WriteFile(validPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyPath := filepath.Join(dir, "empty.mp3")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		size int64
		want bool
	}{
		{name: "healthy", path: validPath, size: 5, want: true},
		{name: "empty metadata path", size: 5},
		{name: "zero metadata size", path: validPath},
		{name: "missing file", path: filepath.Join(dir, "missing.mp3"), size: 5},
		{name: "directory", path: dir, size: 5},
		{name: "empty file", path: emptyPath, size: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := favoriteAudioHealthy(tt.path, tt.size)
			if got != tt.want {
				t.Fatalf("favoriteAudioHealthy(%q, %d) = (%t, %q), want %t",
					tt.path, tt.size, got, reason, tt.want)
			}
			if !got && reason == "" {
				t.Fatal("unhealthy audio missing reason")
			}
		})
	}
}

func TestRefreshImportedLearningOnlyTargetsNotStarted_BitsUT(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	queuedAt := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `user_item_learning` SET .*queued_at.* WHERE item_type = \\? AND item_id = \\? AND learning_status = \\?").
		WithArgs(queuedAt, sqlmock.AnyArg(), LearningItemPhrase, int64(9), learningrule.StatusNotStarted).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `user_item_learning`").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := refreshImportedLearning(context.Background(), LearningItemPhrase, 9, queuedAt); err != nil {
		t.Fatalf("refreshImportedLearning() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLearningQueueTailTimeUsesTimeBeforeOldestItem_BitsUT(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	oldest := time.Date(2026, 8, 15, 10, 0, 0, 123456000, time.UTC)
	mock.ExpectQuery("SELECT MIN\\(queued_at\\) AS oldest FROM `user_item_learning`").
		WillReturnRows(sqlmock.NewRows([]string{"oldest"}).AddRow(oldest))

	got, err := learningQueueTailTime(db.DB, time.Now())
	if err != nil {
		t.Fatalf("learningQueueTailTime() error = %v", err)
	}
	want := oldest.Add(-time.Microsecond)
	if !got.Equal(want) {
		t.Fatalf("learningQueueTailTime() = %s, want %s", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFavoriteBatchErrorStage_BitsUT(t *testing.T) {
	root := errors.New("tts failed")
	err := batchError("audio", root)
	if got := FavoriteBatchErrorStage(err); got != "audio" {
		t.Fatalf("FavoriteBatchErrorStage() = %q, want audio", got)
	}
	if !errors.Is(err, root) {
		t.Fatalf("batch error does not unwrap root: %v", err)
	}
}

func TestCollectFavoriteCleanupChangesSkipsLearningAfterAudioFailure_BitsUT(t *testing.T) {
	healthyPath := filepath.Join(t.TempDir(), "phrase.mp3")
	if err := os.WriteFile(healthyPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets := []favoriteBatchTarget{
		{
			ItemType: ContextItemWord, LearningType: LearningItemWord,
			ItemID: 7, Text: "look",
		},
		{
			ItemType: ContextItemPhrase, LearningType: LearningItemPhrase,
			ItemID: 7, Text: "look up", AudioPath: healthyPath, AudioSize: 5,
		},
	}
	learningKeys := map[learningItemKey]struct{}{
		learningKey(LearningItemWord, 7): {},
	}
	repairErr := errors.New("tts unavailable")
	report, pending := collectFavoriteCleanupChanges(
		context.Background(),
		targets,
		learningKeys,
		FavoriteBatchOptions{},
		func(context.Context, string, int64) (*model.FavoriteAudioData, error) {
			return nil, repairErr
		},
	)

	if report.Scanned != 2 || report.Healthy != 1 || report.AudioRepaired != 0 {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Failures) != 1 || !errors.Is(report.Failures[0].Err, repairErr) {
		t.Fatalf("failures = %+v", report.Failures)
	}
	if len(pending) != 1 ||
		pending[0].ItemType != LearningItemPhrase ||
		pending[0].ItemID != 7 {
		t.Fatalf("pending learning = %+v", pending)
	}
}
