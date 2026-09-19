package service

import (
	"testing"
	"time"
)

func TestBuildFavoriteGroupSummaries_BitsUT(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, loc)
	rows := []favoriteGroupCountRow{
		{BucketType: "day", BucketKey: "2026-08-10", Count: 3},
		{BucketType: "week", BucketKey: "2026-07-27", Count: 8},
		{BucketType: "month", BucketKey: "2026-07-01", Count: 20},
	}

	groups, total, err := buildFavoriteGroupSummaries(rows, now, loc)
	if err != nil {
		t.Fatalf("buildFavoriteGroupSummaries() error = %v", err)
	}
	if total != 31 || len(groups) != 3 {
		t.Fatalf("groups=%+v total=%d", groups, total)
	}
	if groups[0].Type != "day" || groups[0].RangeStart != "2026-08-10T00:00:00+08:00" {
		t.Fatalf("day group = %+v", groups[0])
	}
	if groups[1].Type != "week" ||
		groups[1].RangeStart != "2026-07-27T00:00:00+08:00" ||
		groups[1].RangeEnd != "2026-08-03T00:00:00+08:00" {
		t.Fatalf("week group = %+v", groups[1])
	}
	if groups[2].Type != "month" ||
		groups[2].RangeEnd != "2026-07-12T00:00:00+08:00" {
		t.Fatalf("month group = %+v", groups[2])
	}
}

func TestFavoritePageCursorAndOrdering_BitsUT(t *testing.T) {
	at := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	candidates := []favoritePageCandidate{
		{ItemType: ContextItemPhrase, ItemID: 9, CreatedAt: at},
		{ItemType: ContextItemWord, ItemID: 7, CreatedAt: at},
		{ItemType: ContextItemWord, ItemID: 8, CreatedAt: at},
	}
	sortFavoritePageCandidates(candidates)
	if candidates[0].ItemType != ContextItemWord || candidates[0].ItemID != 8 ||
		candidates[1].ItemType != ContextItemWord || candidates[1].ItemID != 7 ||
		candidates[2].ItemType != ContextItemPhrase {
		t.Fatalf("sortFavoritePageCandidates() = %+v", candidates)
	}

	encoded, err := encodeFavoriteCursor(candidates[1])
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeFavoriteCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ItemType != ContextItemWord || decoded.ItemID != 7 || !decoded.CreatedAt.Equal(at) {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
	if _, err := decodeFavoriteCursor("invalid"); err == nil {
		t.Fatal("decodeFavoriteCursor(invalid) error = nil")
	}
}
