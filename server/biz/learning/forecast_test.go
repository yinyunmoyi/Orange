package learning

import (
	"testing"
	"time"
)

func TestBuildForecast(t *testing.T) {
	loc := time.FixedZone("study", 8*60*60)
	now := time.Date(2026, 12, 30, 20, 0, 0, 0, loc)
	reviews := []time.Time{
		time.Date(2026, 12, 30, 23, 0, 0, 0, loc),
		time.Date(2026, 12, 31, 0, 0, 0, 0, loc),
		time.Date(2026, 12, 31, 12, 0, 0, 0, loc),
		time.Date(2027, 1, 13, 23, 0, 0, 0, loc),
		time.Date(2027, 1, 14, 0, 0, 0, 0, loc),
	}
	got := BuildForecast(now, loc, 14, reviews)
	if len(got) != 14 {
		t.Fatalf("len = %d, want 14", len(got))
	}
	if date := got[0].Date.Format("2006-01-02"); date != "2026-12-31" {
		t.Fatalf("first date = %s", date)
	}
	if got[0].Count != 2 {
		t.Fatalf("first count = %d, want 2", got[0].Count)
	}
	if date := got[13].Date.Format("2006-01-02"); date != "2027-01-13" {
		t.Fatalf("last date = %s", date)
	}
	if got[13].Count != 1 {
		t.Fatalf("last count = %d, want 1", got[13].Count)
	}
}

func TestBuildForecastUsesStudyTimezone(t *testing.T) {
	loc := time.FixedZone("study", 8*60*60)
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, loc)
	reviewUTC := time.Date(2026, 8, 8, 16, 30, 0, 0, time.UTC)
	got := BuildForecast(now, loc, 1, []time.Time{reviewUTC})
	if len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("forecast = %+v", got)
	}
}

func TestBuildForecastEmptyStillReturnsBuckets(t *testing.T) {
	got := BuildForecast(time.Now(), time.UTC, 14, nil)
	if len(got) != 14 {
		t.Fatalf("len = %d, want 14", len(got))
	}
	for _, bucket := range got {
		if bucket.Count != 0 {
			t.Fatalf("bucket = %+v", bucket)
		}
	}
}
