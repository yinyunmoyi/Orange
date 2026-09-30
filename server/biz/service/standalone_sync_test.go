package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"
)

func TestNormalizeStandaloneSyncRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     *model.StandaloneFavoriteSyncRequest
		wantType    string
		wantText    string
		wantInvalid bool
	}{
		{
			name: "word",
			request: &model.StandaloneFavoriteSyncRequest{
				ClientID: "12345678-abcd", ItemType: " WORD ", Text: "  Creed ",
			},
			wantType: "word", wantText: "creed",
		},
		{
			name: "phrase",
			request: &model.StandaloneFavoriteSyncRequest{
				ClientID: "12345678-abce", ItemType: "phrase", Text: " Look   Up ",
			},
			wantType: "phrase", wantText: "look up",
		},
		{
			name: "sentence",
			request: &model.StandaloneFavoriteSyncRequest{
				ClientID: "12345678-abcf", ItemType: "sentence",
				Text: " We\t test this. ", Translation: " 测试。 ",
			},
			wantType: "sentence", wantText: "We test this.",
		},
		{
			name: "missing translation",
			request: &model.StandaloneFavoriteSyncRequest{
				ClientID: "12345678-abcg", ItemType: "sentence", Text: "We test this.",
			},
			wantInvalid: true,
		},
		{
			name: "invalid client id",
			request: &model.StandaloneFavoriteSyncRequest{
				ClientID: "bad", ItemType: "word", Text: "test",
			},
			wantInvalid: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hash, err := normalizeStandaloneSyncRequest(tt.request)
			if tt.wantInvalid {
				if !errors.Is(err, ErrStandaloneSyncInvalid) {
					t.Fatalf("err=%v, want ErrStandaloneSyncInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize error: %v", err)
			}
			if got.ItemType != tt.wantType || got.Text != tt.wantText || len(hash) != 64 {
				t.Fatalf("got=%+v hash=%q", got, hash)
			}
		})
	}
}

func TestStandaloneSyncPayloadHashIsStableAndCoversPayload(t *testing.T) {
	base := &model.StandaloneFavoriteSyncRequest{
		ClientID: "12345678-abcd", ItemType: "sentence",
		Text: "We test this.", Translation: "我们测试。",
	}
	_, first, err := normalizeStandaloneSyncRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := normalizeStandaloneSyncRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := *base
	changed.Translation = "另一个翻译。"
	_, third, err := normalizeStandaloneSyncRequest(&changed)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == third {
		t.Fatalf("hashes first=%q second=%q changed=%q", first, second, third)
	}
}

func TestStandaloneFavoriteAlreadyExists(t *testing.T) {
	gdb := installActionEventTestDB(t)
	if err := gdb.Create(&db.Word{Word: "existing"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&db.Phrase{Phrase: "look up"}).Error; err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		req  model.StandaloneFavoriteSyncRequest
		want bool
	}{
		{name: "existing word", req: model.StandaloneFavoriteSyncRequest{ItemType: ContextItemWord, Text: "existing"}, want: true},
		{name: "new word", req: model.StandaloneFavoriteSyncRequest{ItemType: ContextItemWord, Text: "new"}, want: false},
		{name: "existing phrase", req: model.StandaloneFavoriteSyncRequest{ItemType: ContextItemPhrase, Text: "look up"}, want: true},
		{name: "sentence does not reset learning", req: model.StandaloneFavoriteSyncRequest{ItemType: "sentence", Text: "Existing."}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := standaloneFavoriteAlreadyExists(context.Background(), &tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("exists = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestResetExistingStandaloneLearningForWordAndPhrase(t *testing.T) {
	gdb := installActionEventTestDB(t)
	word := db.Word{Word: "existing"}
	phrase := db.Phrase{Phrase: "look up"}
	if err := gdb.Create(&word).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&phrase).Error; err != nil {
		t.Fatal(err)
	}
	reviewedAt := time.Now().Add(-time.Hour)
	nextReviewAt := time.Now().Add(time.Hour)
	rows := []db.UserItemLearning{
		{
			ItemType: LearningItemWord, ItemID: word.ID, QueuedAt: reviewedAt,
			LearningStatus: learningrule.StatusLearning, MemoryStage: 3,
			ReviewSuccessDays: 2, LastReviewedAt: &reviewedAt, NextReviewAt: &nextReviewAt,
		},
		{
			ItemType: LearningItemPhrase, ItemID: phrase.ID, QueuedAt: reviewedAt,
			LearningStatus: learningrule.StatusMastered, MemoryStage: 5,
			ReviewSuccessDays: 4, LastReviewedAt: &reviewedAt, NextReviewAt: &nextReviewAt,
		},
	}
	if err := gdb.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	if err := resetExistingStandaloneLearning(context.Background(), ContextItemWord, word.ID); err != nil {
		t.Fatal(err)
	}
	if err := resetExistingStandaloneLearning(context.Background(), ContextItemPhrase, phrase.ID); err != nil {
		t.Fatal(err)
	}

	var resetCount int64
	if err := gdb.Model(&db.UserItemLearning{}).
		Where(
			"learning_status = ? AND memory_stage = 0 AND review_success_days = 0 AND "+
				"last_reviewed_at IS NULL AND next_review_at IS NULL",
			learningrule.StatusNotStarted,
		).
		Count(&resetCount).Error; err != nil {
		t.Fatal(err)
	}
	if resetCount != 2 {
		t.Fatalf("reset learning rows = %d, want 2", resetCount)
	}
}
