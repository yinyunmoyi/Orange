package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestNormalizeFavoriteSentence(t *testing.T) {
	got, err := NormalizeFavoriteSentence("  We\t test\nthis.  ")
	if err != nil {
		t.Fatalf("NormalizeFavoriteSentence error: %v", err)
	}
	if got != "We test this." {
		t.Fatalf("normalized=%q, want %q", got, "We test this.")
	}

	for _, input := range []string{"", "中文句子", strings.Repeat("a", 501)} {
		if _, err := NormalizeFavoriteSentence(input); !errors.Is(err, ErrSentenceFavoriteInvalid) {
			t.Fatalf("input=%q err=%v, want ErrSentenceFavoriteInvalid", input, err)
		}
	}
}

func TestSentenceFavoriteDedupeKeyIsStable(t *testing.T) {
	first := SentenceFavoriteDedupeKey("We test this.")
	second := SentenceFavoriteDedupeKey("We test this.")
	if first != second || len(first) != 64 {
		t.Fatalf("unexpected keys first=%q second=%q", first, second)
	}
	if first == SentenceFavoriteDedupeKey("we test this.") {
		t.Fatal("dedupe key must preserve case")
	}
}

func TestSaveSentenceFavoriteRejectsInvalidTranslationBeforeDatabase(t *testing.T) {
	_, err := SaveSentenceFavorite(context.Background(), &model.SentenceFavoriteRequest{
		Sentence:    "We test this.",
		Translation: "   ",
	})
	if !errors.Is(err, ErrSentenceFavoriteInvalid) {
		t.Fatalf("blank translation err=%v, want ErrSentenceFavoriteInvalid", err)
	}

	_, err = SaveSentenceFavorite(context.Background(), &model.SentenceFavoriteRequest{
		Sentence:    "We test this.",
		Translation: strings.Repeat("好", MaxSentenceTranslationLength+1),
	})
	if !errors.Is(err, ErrSentenceTranslationLong) {
		t.Fatalf("long translation err=%v, want ErrSentenceTranslationLong", err)
	}
}

func TestSaveSentenceFavoriteDuplicateReturnsExistingRecordUnchanged(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	createdAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `sentence_favorites`")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `sentence_favorites` WHERE dedupe_key = \\?.*LIMIT \\?").
		WithArgs(SentenceFavoriteDedupeKey("We test this."), 1).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "sentence", "translation", "dedupe_key", "created_at"},
		).AddRow(
			7,
			"We test this.",
			"原有翻译。",
			SentenceFavoriteDedupeKey("We test this."),
			createdAt,
		))

	data, err := SaveSentenceFavorite(context.Background(), &model.SentenceFavoriteRequest{
		Sentence:    "  We   test this. ",
		Translation: "新翻译。",
	})
	if err != nil {
		t.Fatalf("SaveSentenceFavorite error: %v", err)
	}
	if data.ID != 7 || data.Translation != "原有翻译。" || data.CreatedAt != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSentenceFavoriteStatusMissingReturnsFalse(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT `id` FROM `sentence_favorites` WHERE dedupe_key = \\?.*LIMIT \\?").
		WithArgs(SentenceFavoriteDedupeKey("We test this."), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	data, err := GetSentenceFavoriteStatus(context.Background(), "We test this.")
	if err != nil {
		t.Fatalf("GetSentenceFavoriteStatus error: %v", err)
	}
	if data.Favorited || data.ID != 0 {
		t.Fatalf("unexpected status: %+v", data)
	}
}

func TestListSentenceFavoritesReturnsNewestFirst(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	newest := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	oldest := newest.Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `sentence_favorites` ORDER BY sentence_favorites.created_at DESC,sentence_favorites.id DESC",
	)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "sentence", "translation", "dedupe_key", "created_at"},
	).
		AddRow(2, "Newest.", "Newest translation.", SentenceFavoriteDedupeKey("Newest."), newest).
		AddRow(1, "Oldest.", "Oldest translation.", SentenceFavoriteDedupeKey("Oldest."), oldest))
	mock.ExpectQuery("SELECT relations\\.sentence_favorite_id.*FROM sentence_favorite_tags AS relations.*").
		WithArgs(2, 1).
		WillReturnRows(sqlmock.NewRows(
			[]string{"sentence_favorite_id", "tag_id", "name", "color"},
		))

	data, err := ListSentenceFavorites(context.Background())
	if err != nil {
		t.Fatalf("ListSentenceFavorites error: %v", err)
	}
	if data.Total != 2 || data.Items[0].ID != 2 || data.Items[1].ID != 1 {
		t.Fatalf("unexpected list: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListSentenceFavoritesFiltersByAnyTagAndAttachesNotes(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	createdAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT .* FROM `sentence_favorites` JOIN sentence_favorite_tags.*").
		WithArgs(2, 3).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "sentence", "translation", "dedupe_key", "created_at"},
		).AddRow(7, "Filtered.", "Translation.", "hash", createdAt))
	mock.ExpectQuery("SELECT relations\\.sentence_favorite_id.*FROM sentence_favorite_tags AS relations.*").
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows(
			[]string{"sentence_favorite_id", "tag_id", "name", "color"},
		).
			AddRow(7, 2, "grammar", "#E65100").
			AddRow(7, 3, "tense", "#1565C0"))

	data, err := ListSentenceFavorites(context.Background(), 2, 3)
	if err != nil {
		t.Fatalf("ListSentenceFavorites error: %v", err)
	}
	if data.Total != 1 || len(data.Items[0].Tags) != 2 {
		t.Fatalf("unexpected data: %+v", data)
	}
	if data.Items[0].Tags[0].Name != "grammar" {
		t.Fatalf("unexpected first tag: %+v", data.Items[0].Tags[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSentenceFavoriteMissingReturnsNotFound(t *testing.T) {
	mock, cleanup := installSentenceFavoriteMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `sentence_favorites` WHERE `sentence_favorites`.`id` = \\?.*LIMIT \\?").
		WithArgs(99, 1).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "sentence", "translation", "dedupe_key", "created_at"},
		))

	_, err := GetSentenceFavorite(context.Background(), 99)
	if !errors.Is(err, ErrSentenceFavoriteNotFound) {
		t.Fatalf("err=%v, want ErrSentenceFavoriteNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func installSentenceFavoriteMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
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
