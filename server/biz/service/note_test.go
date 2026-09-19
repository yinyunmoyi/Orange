package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestNormalizeNoteTarget(t *testing.T) {
	tests := []struct {
		name        string
		itemType    string
		text        string
		wantType    string
		wantText    string
		wantInvalid bool
	}{
		{name: "word", itemType: " WORD ", text: " Hello ", wantType: "word", wantText: "hello"},
		{name: "phrase", itemType: "phrase", text: " Look   Up ", wantType: "phrase", wantText: "look up"},
		{name: "invalid type", itemType: "sentence", text: "hello", wantInvalid: true},
		{name: "invalid word", itemType: "word", text: "two words", wantInvalid: true},
		{name: "invalid phrase", itemType: "phrase", text: "single", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotText, err := NormalizeNoteTarget(tt.itemType, tt.text)
			if tt.wantInvalid {
				if !errors.Is(err, ErrNoteInvalid) {
					t.Fatalf("err=%v, want ErrNoteInvalid", err)
				}
				return
			}
			if err != nil || gotType != tt.wantType || gotText != tt.wantText {
				t.Fatalf("got=(%q,%q,%v), want=(%q,%q,nil)", gotType, gotText, err, tt.wantType, tt.wantText)
			}
		})
	}
}

func TestSaveNoteRejectsOverLimitBeforeDatabase(t *testing.T) {
	_, err := SaveNote(context.Background(), &model.NoteRequest{
		ItemType: "word",
		Text:     "hello",
		Note:     strings.Repeat("好", MaxNoteLength+1),
	})
	if !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("err=%v, want ErrNoteTooLong", err)
	}
}

func TestValidateNoteContentUsesUnicodeCodePoints(t *testing.T) {
	if err := validateNoteContent(strings.Repeat("😀", MaxNoteLength)); err != nil {
		t.Fatalf("exact limit rejected: %v", err)
	}
	if err := validateNoteContent(strings.Repeat("😀", MaxNoteLength+1)); !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("over limit err=%v, want ErrNoteTooLong", err)
	}
}

func TestGetNoteMissingReturnsEmptyData(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `item_notes` WHERE item_type = \\? AND item_text = \\?.*LIMIT \\?").
		WithArgs("word", "hello", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_type", "item_text", "note", "created_at", "updated_at"}))

	data, err := GetNote(context.Background(), "word", "Hello")
	if err != nil {
		t.Fatalf("GetNote error: %v", err)
	}
	if data.ItemType != "word" || data.Text != "hello" || data.Note != "" {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveNoteBlankDeletesExistingRow(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `item_notes` WHERE item_type = \\? AND item_text = \\?").
		WithArgs("phrase", "look up").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	data, err := SaveNote(context.Background(), &model.NoteRequest{
		ItemType: "phrase",
		Text:     "Look   Up",
		Note:     " \n ",
	})
	if err != nil {
		t.Fatalf("SaveNote error: %v", err)
	}
	if data.Note != "" || data.Text != "look up" {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func installNoteMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatal(err)
	}
	previous := db.DB
	db.DB = gdb
	return mock, func() {
		db.DB = previous
		sqlDB.Close()
	}
}
