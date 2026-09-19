package service

import (
	"errors"
	"testing"

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
