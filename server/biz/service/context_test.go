package service

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNormalizeContextTaskRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *model.ContextTaskRequest
		wantErr bool
	}{
		{
			name: "word",
			req: &model.ContextTaskRequest{
				ItemType: "word", ItemID: 1,
				Paragraph: "Hello world!", SelectionStart: 6, SelectionEnd: 11,
			},
		},
		{
			name: "phrase",
			req: &model.ContextTaskRequest{
				ItemType: "phrase", ItemID: 2,
				Paragraph: "Please look up the word.", SelectionStart: 7, SelectionEnd: 14,
			},
		},
		{
			name: "preserves raw paragraph",
			req: &model.ContextTaskRequest{
				ItemType: "word", ItemID: 1,
				Paragraph: "  “Hello,” she said.  ", SelectionStart: 3, SelectionEnd: 8,
			},
		},
		{
			name: "range out of bounds",
			req: &model.ContextTaskRequest{
				ItemType: "word", ItemID: 1,
				Paragraph: "Hello.", SelectionStart: 0, SelectionEnd: 99,
			},
			wantErr: true,
		},
		{
			name: "invalid item id",
			req: &model.ContextTaskRequest{
				ItemType: "word", ItemID: 0,
				Paragraph: "Hello.", SelectionStart: 0, SelectionEnd: 5,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			req: &model.ContextTaskRequest{
				ItemType: "sentence", ItemID: 1,
				Paragraph: "Hello.", SelectionStart: 0, SelectionEnd: 5,
			},
			wantErr: true,
		},
		{
			name: "too long",
			req: &model.ContextTaskRequest{
				ItemType: "word", ItemID: 1,
				Paragraph: strings.Repeat("a", maxContextParagraphUTF16+1), SelectionStart: 0, SelectionEnd: 5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeContextTaskRequest(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ItemType != tt.req.ItemType || got.ItemID != tt.req.ItemID {
				t.Fatalf("item = %s/%d", got.ItemType, got.ItemID)
			}
			if got.Paragraph != tt.req.Paragraph ||
				got.SelectionStart != tt.req.SelectionStart ||
				got.SelectionEnd != tt.req.SelectionEnd {
				t.Fatalf("raw input changed: got=%+v want=%+v", got, tt.req)
			}
		})
	}
}

func TestContextTaskDedupeKey(t *testing.T) {
	base := &model.ContextTaskRequest{
		ItemType: "word", ItemID: 1,
		Paragraph: "Hello world!", SelectionStart: 6, SelectionEnd: 11,
	}
	same := *base
	if ContextTaskDedupeKey(base) != ContextTaskDedupeKey(&same) {
		t.Fatal("same request produced different keys")
	}

	changed := *base
	changed.SelectionStart = 0
	if ContextTaskDedupeKey(base) == ContextTaskDedupeKey(&changed) {
		t.Fatal("different request produced same key")
	}

	otherItem := *base
	otherItem.ItemID = 2
	if ContextTaskDedupeKey(base) == ContextTaskDedupeKey(&otherItem) {
		t.Fatal("different item produced same key")
	}

	if contextTaskDedupeKey(base, "rule_en_v3") == contextTaskDedupeKey(base, "rule_en_v4") {
		t.Fatal("different splitter versions produced same key")
	}
	if contextTaskDedupeKey(base, "rule_en_v4") != contextTaskDedupeKey(&same, "rule_en_v4") {
		t.Fatal("same request and splitter version produced different keys")
	}
}

func TestContextDedupeKey(t *testing.T) {
	key := ContextDedupeKey("word", 1, "Hello world!", 6, 11)
	if key != ContextDedupeKey("word", 1, "Hello world!", 6, 11) {
		t.Fatal("same context produced different keys")
	}
	if key == ContextDedupeKey("word", 1, "Different sentence.", 0, 9) {
		t.Fatal("different context produced same key")
	}
}

func TestCanRetryContextTask(t *testing.T) {
	tests := []struct {
		name      string
		task      *db.ContextTask
		wantRetry bool
	}{
		{name: "first failure", task: &db.ContextTask{Status: ContextTaskFailed, Attempts: 1}, wantRetry: true},
		{name: "second failure", task: &db.ContextTask{Status: ContextTaskFailed, Attempts: 2}, wantRetry: true},
		{name: "exhausted", task: &db.ContextTask{Status: ContextTaskFailed, Attempts: 3}},
		{name: "processing", task: &db.ContextTask{Status: ContextTaskProcessing, Attempts: 1}},
		{name: "nil task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canRetryContextTask(tt.task); got != tt.wantRetry {
				t.Fatalf("canRetryContextTask() = %t, want %t", got, tt.wantRetry)
			}
		})
	}
}

func TestLockContextItemRejectsDeletedFavorite_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id` FROM `words` WHERE id = ? LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err := lockContextItem(gormDB, ContextItemWord, 7)
	if !errors.Is(err, ErrContextItemNotFound) {
		t.Fatalf("lockContextItem error = %v, want %v", err, ErrContextItemNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
