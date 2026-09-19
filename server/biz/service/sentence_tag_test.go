package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestNormalizeSentenceTagFields(t *testing.T) {
	name, err := NormalizeSentenceTagName("  grammar   focus  ")
	if err != nil || name != "grammar focus" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	color, err := NormalizeSentenceTagColor(" #e65100 ")
	if err != nil || color != "#E65100" {
		t.Fatalf("color=%q err=%v", color, err)
	}
	note, err := NormalizeSentenceFavoriteNote("  Explains the conditional form.  ")
	if err != nil || note != "Explains the conditional form." {
		t.Fatalf("note=%q err=%v", note, err)
	}
	note, err = NormalizeSentenceFavoriteNote("First line.\r\nSecond line.")
	if err != nil || note != "First line.\nSecond line." {
		t.Fatalf("multiline note=%q err=%v", note, err)
	}
	note, err = NormalizeSentenceFavoriteNote("   ")
	if err != nil || note != "" {
		t.Fatalf("blank note=%q err=%v", note, err)
	}
}

func TestNormalizeSentenceTagFieldsRejectInvalidValues(t *testing.T) {
	for _, name := range []string{"", "line\nbreak", strings.Repeat("a", 25)} {
		if _, err := NormalizeSentenceTagName(name); !errors.Is(err, ErrSentenceTagInvalid) {
			t.Fatalf("name=%q err=%v, want ErrSentenceTagInvalid", name, err)
		}
	}
	for _, note := range []string{"tab\tcharacter", strings.Repeat("a", 301)} {
		if _, err := NormalizeSentenceFavoriteNote(note); !errors.Is(err, ErrSentenceTagInvalid) {
			t.Fatalf("note length=%d err=%v, want ErrSentenceTagInvalid", len(note), err)
		}
	}
	if _, err := NormalizeSentenceTagColor("#000000"); !errors.Is(err, ErrSentenceTagInvalid) {
		t.Fatalf("color err=%v, want ErrSentenceTagInvalid", err)
	}
}

func TestCreateSentenceTagMapsDuplicateName(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `sentence_tags`")).
		WillReturnError(&mysqlDriver.MySQLError{Number: 1062, Message: "duplicate"})
	mock.ExpectRollback()

	_, err := CreateSentenceTag(context.Background(), &model.SentenceTagCreateRequest{
		Name:  "grammar",
		Color: "#E65100",
	})
	if !errors.Is(err, ErrSentenceTagConflict) {
		t.Fatalf("err=%v, want ErrSentenceTagConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListSentenceTagsKeepsCreationOrder(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	first := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `sentence_tags` ORDER BY created_at ASC,id ASC",
	)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "name", "color", "created_at"},
	).
		AddRow(1, "grammar", "#E65100", first).
		AddRow(2, "tense", "#1565C0", first.Add(time.Minute)))

	data, err := ListSentenceTags(context.Background())
	if err != nil {
		t.Fatalf("ListSentenceTags error: %v", err)
	}
	if data.Total != 2 || data.Items[0].ID != 1 || data.Items[1].ID != 2 {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceSentenceFavoriteTagsRejectsInvalidAssignmentsBeforeDatabase(t *testing.T) {
	_, err := ReplaceSentenceFavoriteTags(
		context.Background(),
		7,
		[]int64{2, 2},
		"A sentence note.",
	)
	if !errors.Is(err, ErrSentenceTagDuplicate) {
		t.Fatalf("duplicate err=%v, want ErrSentenceTagDuplicate", err)
	}

	_, err = ReplaceSentenceFavoriteTags(
		context.Background(),
		7,
		[]int64{2},
		" ",
	)
	if !errors.Is(err, ErrSentenceTagInvalid) {
		t.Fatalf("blank note err=%v, want ErrSentenceTagInvalid", err)
	}
}

func TestReplaceSentenceFavoriteTagsRollsBackWhenTagMissing(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `sentence_favorites` WHERE `sentence_favorites`.`id` = \\?.*LIMIT \\? FOR UPDATE").
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `sentence_tags` WHERE id IN \\(\\?,\\?\\)").
		WithArgs(2, 3).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err := ReplaceSentenceFavoriteTags(
		context.Background(),
		7,
		[]int64{2, 3},
		"A sentence note.",
	)
	if !errors.Is(err, ErrSentenceTagNotFound) {
		t.Fatalf("err=%v, want ErrSentenceTagNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceSentenceFavoriteTagsStoresNotesAndReturnsUpdatedDetail(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	createdAt := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `sentence_favorites` WHERE `sentence_favorites`.`id` = \\?.*LIMIT \\? FOR UPDATE").
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `sentence_tags` WHERE id IN \\(\\?,\\?\\)").
		WithArgs(2, 3).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec("UPDATE `sentence_favorites` SET `note`=\\? WHERE id = \\?").
		WithArgs("A sentence note.", 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `sentence_favorite_tags` WHERE sentence_favorite_id = \\?").
		WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `sentence_favorite_tags`").
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `sentence_favorites` WHERE `sentence_favorites`.`id` = \\?.*LIMIT \\?").
		WithArgs(7, 1).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "sentence", "translation", "note", "dedupe_key", "created_at"},
		).AddRow(7, "A sentence.", "A translation.", "A sentence note.", "hash", createdAt))
	mock.ExpectQuery("SELECT relations\\.sentence_favorite_id.*FROM sentence_favorite_tags AS relations.*").
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows(
			[]string{"sentence_favorite_id", "tag_id", "name", "color"},
		).
			AddRow(7, 2, "grammar", "#E65100").
			AddRow(7, 3, "tense", "#1565C0"))

	data, err := ReplaceSentenceFavoriteTags(
		context.Background(),
		7,
		[]int64{2, 3},
		" A sentence note. ",
	)
	if err != nil {
		t.Fatalf("ReplaceSentenceFavoriteTags error: %v", err)
	}
	if len(data.Tags) != 2 ||
		data.Note != "A sentence note." ||
		data.Tags[1].Name != "tense" {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
