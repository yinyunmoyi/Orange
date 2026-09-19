package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAddWordToSimilarGroupSupportsAllMergeActions(t *testing.T) {
	for _, action := range []MergeAction{MergeCreated, MergeJoined, MergeNoop, MergeMerged} {
		t.Run(string(action), func(t *testing.T) {
			ensured := make([]string, 0, 2)
			ensure := func(_ context.Context, word string, opts EnsureOptions) (int64, EnsureAction, error) {
				if opts.ForceRefresh {
					t.Fatal("ForceRefresh = true")
				}
				ensured = append(ensured, word)
				if word == "affect" {
					return 7, EnsureCached, nil
				}
				return 8, EnsureFetched, nil
			}
			merge := func(_ context.Context, ids []int64) (int64, MergeAction, error) {
				if len(ids) != 2 || ids[0] != 7 || ids[1] != 8 {
					t.Fatalf("merge ids = %v", ids)
				}
				return 3, action, nil
			}
			load := func(_ context.Context, groupID int64) (*model.SimilarGroupDetailData, error) {
				if groupID != 3 {
					t.Fatalf("load groupID = %d", groupID)
				}
				return &model.SimilarGroupDetailData{
					ID:          3,
					MemberCount: 2,
					Members: []model.SimilarGroupMember{
						{ItemID: 7, Word: "affect"},
						{ItemID: 8, Word: "effect"},
					},
				}, nil
			}

			data, err := addWordToSimilarGroup(
				context.Background(),
				" Affect ",
				" EFFECT ",
				ensure,
				merge,
				load,
			)
			if err != nil || data == nil || data.ID != 3 || len(data.Members) != 2 {
				t.Fatalf("addWordToSimilarGroup() = (%+v, %v)", data, err)
			}
			if len(ensured) != 2 || ensured[0] != "affect" || ensured[1] != "effect" {
				t.Fatalf("ensured = %v", ensured)
			}
		})
	}
}

func TestAddWordToSimilarGroupRejectsInvalidPair(t *testing.T) {
	called := false
	ensure := func(context.Context, string, EnsureOptions) (int64, EnsureAction, error) {
		called = true
		return 0, "", nil
	}
	data, err := addWordToSimilarGroup(
		context.Background(),
		"affect",
		" AFFECT ",
		ensure,
		nil,
		nil,
	)
	if data != nil || !errors.Is(err, ErrSimilarGroupInvalid) {
		t.Fatalf("addWordToSimilarGroup() = (%+v, %v)", data, err)
	}
	if called {
		t.Fatal("ensure called for invalid pair")
	}
}

func TestAddWordToSimilarGroupReturnsInitializationAndMergeErrors(t *testing.T) {
	initializeErr := errors.New("initialize failed")
	ensureFailure := func(context.Context, string, EnsureOptions) (int64, EnsureAction, error) {
		return 0, "", initializeErr
	}
	if _, err := addWordToSimilarGroup(
		context.Background(),
		"affect",
		"effect",
		ensureFailure,
		nil,
		nil,
	); !errors.Is(err, initializeErr) {
		t.Fatalf("initialization error = %v", err)
	}

	ensureSuccess := func(_ context.Context, word string, _ EnsureOptions) (int64, EnsureAction, error) {
		if word == "affect" {
			return 7, EnsureCached, nil
		}
		return 8, EnsureCached, nil
	}
	mergeErr := errors.New("merge failed")
	mergeFailure := func(context.Context, []int64) (int64, MergeAction, error) {
		return 0, "", mergeErr
	}
	if _, err := addWordToSimilarGroup(
		context.Background(),
		"affect",
		"effect",
		ensureSuccess,
		mergeFailure,
		nil,
	); !errors.Is(err, mergeErr) {
		t.Fatalf("merge error = %v", err)
	}
}

func TestGetSimilarGroupByWordReturnsOrderedMembers(t *testing.T) {
	gormDB, mock, cleanup := newSimilarGroupMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	now := time.Now()
	mock.ExpectQuery("SELECT members\\.\\* FROM word_group_members AS members JOIN words ON words\\.id = members\\.word_id WHERE words\\.word = .* LIMIT .*").
		WithArgs("affect", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "word_id", "added_at"}).
			AddRow(10, 3, 7, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `word_groups` WHERE id = ? LIMIT ?")).
		WithArgs(int64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "member_count", "created_at", "updated_at"}).
			AddRow(3, 2, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `word_group_members` WHERE group_id = ? ORDER BY id")).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "word_id", "added_at"}).
			AddRow(10, 3, 7, now).
			AddRow(11, 3, 8, now))
	mock.ExpectQuery("SELECT \\* FROM `words` WHERE id IN .*").
		WithArgs(int64(7), int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "word", "created_at", "updated_at"}).
			AddRow(8, "affection", now, now).
			AddRow(7, "affect", now, now))
	mock.ExpectQuery("SELECT \\* FROM `word_meanings` WHERE word_id IN .* AND kind = .* ORDER BY word_id, sort_order").
		WithArgs(int64(7), int64(8), "zh").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "word_id", "kind", "part_of_speech", "definition", "sort_order",
		}).
			AddRow(1, 7, "zh", "v.", "影响", 0).
			AddRow(2, 7, "zh", "n.", "情感", 1).
			AddRow(3, 8, "zh", "n.", "喜爱", 0))

	data, err := GetSimilarGroupByWord(context.Background(), "  AfFeCt  ")
	if err != nil {
		t.Fatalf("GetSimilarGroupByWord error = %v", err)
	}
	if data == nil {
		t.Fatal("GetSimilarGroupByWord data = nil")
	}
	if data.ID != 3 || data.MemberCount != 2 || len(data.Members) != 2 {
		t.Fatalf("unexpected group data: %+v", data)
	}
	if data.Members[0].ItemID != 7 || data.Members[0].Word != "affect" ||
		data.Members[0].TopMeaningText != "影响" {
		t.Fatalf("unexpected first member: %+v", data.Members[0])
	}
	if data.Members[1].ItemID != 8 || data.Members[1].Word != "affection" ||
		data.Members[1].TopMeaningText != "喜爱" {
		t.Fatalf("unexpected second member: %+v", data.Members[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSimilarGroupByWordNotFound(t *testing.T) {
	gormDB, mock, cleanup := newSimilarGroupMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	mock.ExpectQuery("SELECT members\\.\\* FROM word_group_members AS members JOIN words ON words\\.id = members\\.word_id WHERE words\\.word = .* LIMIT .*").
		WithArgs("missing", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "group_id", "word_id", "added_at"}))

	data, err := GetSimilarGroupByWord(context.Background(), "missing")
	if err != nil || data != nil {
		t.Fatalf("GetSimilarGroupByWord = (%+v, %v), want (nil, nil)", data, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSimilarGroupByWordReturnsDatabaseError(t *testing.T) {
	gormDB, mock, cleanup := newSimilarGroupMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	wantErr := errors.New("query failed")
	mock.ExpectQuery("SELECT members\\.\\* FROM word_group_members AS members JOIN words ON words\\.id = members\\.word_id WHERE words\\.word = .* LIMIT .*").
		WithArgs("affect", 1).
		WillReturnError(wantErr)

	data, err := GetSimilarGroupByWord(context.Background(), "affect")
	if data != nil || !errors.Is(err, wantErr) {
		t.Fatalf("GetSimilarGroupByWord = (%+v, %v), want error %v", data, err, wantErr)
	}
}

func newSimilarGroupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
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
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return gormDB, mock, func() { _ = sqlDB.Close() }
}
