package service

import (
	"context"
	"database/sql/driver"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDeleteFavoriteRejectsInvalidRequest_BitsUT(t *testing.T) {
	tests := []struct {
		itemType string
		itemID   int64
	}{
		{itemType: "sentence", itemID: 1},
		{itemType: ContextItemWord, itemID: 0},
		{itemType: ContextItemPhrase, itemID: -1},
	}
	for _, tt := range tests {
		if err := DeleteFavorite(context.Background(), tt.itemType, tt.itemID); err != ErrFavoriteDeleteInvalid {
			t.Fatalf("DeleteFavorite(%q, %d) error = %v, want %v", tt.itemType, tt.itemID, err, ErrFavoriteDeleteInvalid)
		}
	}
}

func TestDeleteFavoriteMissingItemIsIdempotent_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `phrases` WHERE id = ? LIMIT ? FOR UPDATE")).
		WithArgs(int64(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phrase", "audio_file_path"}))
	mock.ExpectCommit()

	if err := DeleteFavorite(context.Background(), ContextItemPhrase, 99); err != nil {
		t.Fatalf("DeleteFavorite missing phrase error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteFavoritePhraseCascadesAndDeletesAudioAfterCommit_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	phraseAudio := writeDeleteTestFile(t, audioDir, "phrase.mp3")
	contextAudio := writeDeleteTestFile(t, audioDir, "context.mp3")
	contextVideo := writeDeleteTestFile(t, audioDir, "context.webm")

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*FOR UPDATE").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phrase", "audio_file_path"}).
			AddRow(9, "look up", phraseAudio))
	mock.ExpectQuery("SELECT `id`,`audio_file_path` FROM `contexts` WHERE item_type = .* AND item_id = .*").
		WithArgs(ContextItemPhrase, int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "audio_file_path"}).AddRow(5, contextAudio))
	mock.ExpectQuery("SELECT `file_path` FROM `context_videos` WHERE context_id IN .*").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(contextVideo))
	expectFavoriteDeleteExec(mock, "context_videos", "context_id IN .*", int64(5))
	mock.ExpectQuery("SELECT .* FROM `user_learning_queue` WHERE item_type = .* AND item_id = .* ORDER BY session_id, id").
		WithArgs(LearningItemPhrase, int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "session_id"}))
	expectFavoriteDeleteExec(mock, "user_learning_queue", "item_type = .* AND item_id = .*", LearningItemPhrase, int64(9))
	expectFavoriteDeleteExec(mock, "user_item_learning", "item_type = .* AND item_id = .*", LearningItemPhrase, int64(9))
	expectFavoriteDeleteExec(mock, "context_tasks", "item_type = .* AND item_id = .*", ContextItemPhrase, int64(9))
	expectFavoriteDeleteExec(mock, "contexts", "item_type = .* AND item_id = .*", ContextItemPhrase, int64(9))
	expectFavoriteDeleteExec(mock, "phrase_meanings", "phrase_id = .*", int64(9))
	expectFavoriteDeleteExec(mock, "phrases", "id = .*", int64(9))
	mock.ExpectCommit()

	if err := DeleteFavorite(context.Background(), ContextItemPhrase, 9); err != nil {
		t.Fatalf("DeleteFavorite phrase error = %v", err)
	}
	for _, path := range []string{phraseAudio, contextAudio, contextVideo} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("audio path %q still exists, stat error = %v", path, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteFavoriteWordCascadesAndDeletesEmptyGroup_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	wordAudio := writeDeleteTestFile(t, audioDir, "word.mp3")
	contextAudio := writeDeleteTestFile(t, audioDir, "context.mp3")
	contextVideo := writeDeleteTestFile(t, audioDir, "context.webm")

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `words` WHERE id = .*FOR UPDATE").
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word"}).AddRow(7, "look"))
	mock.ExpectQuery("SELECT `file_path` FROM `word_audios` WHERE word_id = .*").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(wordAudio))
	mock.ExpectQuery("SELECT `id`,`audio_file_path` FROM `contexts` WHERE item_type = .* AND item_id = .*").
		WithArgs(ContextItemWord, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "audio_file_path"}).AddRow(6, contextAudio))
	mock.ExpectQuery("SELECT `file_path` FROM `context_videos` WHERE context_id IN .*").
		WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"file_path"}).AddRow(contextVideo))
	expectFavoriteDeleteExec(mock, "context_videos", "context_id IN .*", int64(6))
	mock.ExpectQuery("SELECT .* FROM `user_learning_queue` WHERE item_type = .* AND item_id = .* ORDER BY session_id, id").
		WithArgs(LearningItemWord, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "session_id"}))
	expectFavoriteDeleteExec(mock, "user_learning_queue", "item_type = .* AND item_id = .*", LearningItemWord, int64(7))
	expectFavoriteDeleteExec(mock, "user_item_learning", "item_type = .* AND item_id = .*", LearningItemWord, int64(7))
	expectFavoriteDeleteExec(mock, "context_tasks", "item_type = .* AND item_id = .*", ContextItemWord, int64(7))
	expectFavoriteDeleteExec(mock, "contexts", "item_type = .* AND item_id = .*", ContextItemWord, int64(7))
	mock.ExpectQuery("SELECT .* FROM `word_group_members` WHERE word_id = .*").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "word_id"}).AddRow(1, 3, 7))
	expectFavoriteDeleteExec(mock, "word_group_members", "word_id = .*", int64(7))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `word_group_members` WHERE group_id = .*").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectFavoriteDeleteExec(mock, "word_groups", "id = .*", int64(3))
	expectFavoriteDeleteExec(mock, "word_meanings", "word_id = .*", int64(7))
	expectFavoriteDeleteExec(mock, "word_audios", "word_id = .*", int64(7))
	expectFavoriteDeleteExec(mock, "words", "id = .*", int64(7))
	mock.ExpectCommit()

	if err := DeleteFavorite(context.Background(), ContextItemWord, 7); err != nil {
		t.Fatalf("DeleteFavorite word error = %v", err)
	}
	for _, path := range []string{wordAudio, contextAudio, contextVideo} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("audio path %q still exists, stat error = %v", path, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteFavoriteRollbackKeepsAudio_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioPath := writeDeleteTestFile(t, t.TempDir(), "phrase.mp3")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `phrases` WHERE id = .*FOR UPDATE").
		WithArgs(int64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phrase", "audio_file_path"}).
			AddRow(9, "look up", audioPath))
	mock.ExpectQuery("SELECT `id`,`audio_file_path` FROM `contexts` WHERE item_type = .* AND item_id = .*").
		WithArgs(ContextItemPhrase, int64(9)).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	if err := DeleteFavorite(context.Background(), ContextItemPhrase, 9); err == nil {
		t.Fatal("DeleteFavorite rollback error = nil")
	}
	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("audio should remain after rollback: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdjustSessionsAfterFavoriteDeleteSelectsNextWithoutAdvancingTurn_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()

	currentID := int64(10)
	queues := []db.UserLearningQueue{
		{ID: 10, SessionID: 5, ItemType: LearningItemWord, ItemID: 7},
	}
	sessionRows := sqlmock.NewRows([]string{
		"id", "study_date", "status", "turn_no", "current_queue_item_id", "created_at", "updated_at",
	}).AddRow(5, time.Now(), sessionActive, 4, currentID, time.Now(), time.Now())
	mock.ExpectQuery("SELECT .* FROM `learning_sessions` WHERE id IN .*ORDER BY id FOR UPDATE").
		WithArgs(int64(5)).
		WillReturnRows(sessionRows)
	mock.ExpectExec("DELETE FROM `user_learning_queue` WHERE item_type = .* AND item_id = .*").
		WithArgs(LearningItemWord, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `user_learning_queue` WHERE .*session_id = .*status <>.*").
		WithArgs(int64(5), learningrule.QueueCompleted).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "answer_count",
		}).AddRow(20, 5, LearningItemPhrase, 8, learningrule.QueueNew, learningrule.QueueNew, 2, learningrule.QueuePending, 0))
	mock.ExpectExec("UPDATE `learning_sessions` SET .*current_queue_item_id.*turn_no.*updated_at.* WHERE id = .*").
		WithArgs(int64(20), 4, sqlmock.AnyArg(), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := adjustLearningSessionsForFavoriteDelete(gormDB, LearningItemWord, 7, queues, time.Now()); err != nil {
		t.Fatalf("adjustLearningSessionsForFavoriteDelete error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdjustSessionsAfterFavoriteDeleteCompletesEmptySession_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()

	currentID := int64(10)
	queues := []db.UserLearningQueue{{ID: 10, SessionID: 5, ItemType: LearningItemPhrase, ItemID: 9}}
	mock.ExpectQuery("SELECT .* FROM `learning_sessions` WHERE id IN .*ORDER BY id FOR UPDATE").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id", "created_at", "updated_at",
		}).AddRow(5, time.Now(), sessionActive, 2, currentID, time.Now(), time.Now()))
	mock.ExpectExec("DELETE FROM `user_learning_queue` WHERE item_type = .* AND item_id = .*").
		WithArgs(LearningItemPhrase, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `user_learning_queue` WHERE .*session_id = .*status <>.*").
		WithArgs(int64(5), learningrule.QueueCompleted).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "answer_count",
		}))
	mock.ExpectExec("UPDATE `learning_sessions` SET .*completed_at.*current_queue_item_id.*status.*turn_no.*updated_at.* WHERE id = .*").
		WithArgs(sqlmock.AnyArg(), nil, sessionCompleted, 2, sqlmock.AnyArg(), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := adjustLearningSessionsForFavoriteDelete(gormDB, LearningItemPhrase, 9, queues, time.Now()); err != nil {
		t.Fatalf("adjustLearningSessionsForFavoriteDelete error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newDeleteMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		sqlDB.Close()
		t.Fatal(err)
	}
	return gormDB, mock, func() { _ = sqlDB.Close() }
}

func expectFavoriteDeleteExec(mock sqlmock.Sqlmock, table, where string, args ...driver.Value) {
	mock.ExpectExec("DELETE FROM `" + table + "` WHERE " + where).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func writeDeleteTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
