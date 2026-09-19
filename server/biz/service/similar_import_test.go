package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMissingWordDataFields(t *testing.T) {
	complete := &model.WordData{
		Word:     "affect",
		Phonetic: "/əˈfekt/",
		Pronunciations: []model.Pronunciation{
			{Accent: "US", Text: "/əˈfekt/", AudioURL: "/api/v1/word/audio?src=tts"},
		},
		Meanings: []model.Meaning{
			{PartOfSpeech: "verb", Definitions: []model.Definition{{Definition: "have an effect on"}}},
		},
	}
	if missing := missingWordDataFields(complete); len(missing) != 0 {
		t.Fatalf("complete word data missing = %v", missing)
	}

	missing := missingWordDataFields(&model.WordData{Word: "affect"})
	want := []string{"audio"}
	if len(missing) != len(want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
	for index := range want {
		if missing[index] != want[index] {
			t.Fatalf("missing = %v, want %v", missing, want)
		}
	}
}

func TestWordInitializationStatusRequiresAudioAndChineseMeaning(t *testing.T) {
	complete := wordInitializationStatus{
		Audio: true, Phonetic: false, EnglishMeaning: false, ChineseMeaning: true,
	}
	if !complete.complete() {
		t.Fatal("complete status reported incomplete")
	}
	complete.ChineseMeaning = false
	if complete.complete() {
		t.Fatal("status without chinese meaning reported complete")
	}
}

func TestFindFullyFavoritedWordIDAcceptsMissingOptionalDictionaryFields(t *testing.T) {
	gormDB, mock, cleanup := newSimilarGroupMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	now := time.Now()
	audioPath := writeSimilarImportAudio(t)
	mock.ExpectQuery("SELECT \\* FROM `words` WHERE word = .* LIMIT .*").
		WithArgs("affect", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word", "created_at", "updated_at"}).
			AddRow(7, "affect", now, now))
	mock.ExpectQuery("SELECT `phonetic_text`,`file_path`,`file_size` FROM `word_audios` WHERE word_id = .*").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"phonetic_text", "file_path", "file_size"}).
			AddRow("", audioPath, 5))
	mock.ExpectQuery("SELECT `kind`,`definition` FROM `word_meanings` WHERE word_id = .* AND kind IN .*").
		WithArgs(int64(7), "en", "zh").
		WillReturnRows(sqlmock.NewRows([]string{"kind", "definition"}).
			AddRow("zh", "影响"))

	id, complete, err := findFullyFavoritedWordID(context.Background(), "affect")
	if err != nil || id != 7 || !complete {
		t.Fatalf("findFullyFavoritedWordID = (%d, %v, %v)", id, complete, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFindFullyFavoritedWordIDRejectsMissingChineseMeaning(t *testing.T) {
	gormDB, mock, cleanup := newSimilarGroupMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	now := time.Now()
	audioPath := writeSimilarImportAudio(t)
	mock.ExpectQuery("SELECT \\* FROM `words` WHERE word = .* LIMIT .*").
		WithArgs("affect", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word", "created_at", "updated_at"}).
			AddRow(7, "affect", now, now))
	mock.ExpectQuery("SELECT `phonetic_text`,`file_path`,`file_size` FROM `word_audios` WHERE word_id = .*").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"phonetic_text", "file_path", "file_size"}).
			AddRow("/əˈfekt/", audioPath, 5))
	mock.ExpectQuery("SELECT `kind`,`definition` FROM `word_meanings` WHERE word_id = .* AND kind IN .*").
		WithArgs(int64(7), "en", "zh").
		WillReturnRows(sqlmock.NewRows([]string{"kind", "definition"}).
			AddRow("en", "have an effect on"))

	id, complete, err := findFullyFavoritedWordID(context.Background(), "affect")
	if err != nil || id != 7 || complete {
		t.Fatalf("findFullyFavoritedWordID = (%d, %v, %v)", id, complete, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func writeSimilarImportAudio(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "affect.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
