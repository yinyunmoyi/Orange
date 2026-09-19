package learning

import (
	"testing"
	"time"
)

func TestKnownSchedule(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, 8, 8, 15, 0, 0, 0, loc)
	p := Progress{Status: StatusNotStarted}
	wantDays := []int{1, 3, 3, 7, 16, 30}
	for stage, days := range wantDays {
		var err error
		p, err = ApplyKnown(p, now, loc)
		if err != nil {
			t.Fatal(err)
		}
		if p.Stage != stage+1 || p.Status != StatusLearning {
			t.Fatalf("stage %d: got %+v", stage, p)
		}
		want := NextStudyDay(now, days, loc)
		if p.NextReviewAt == nil || !p.NextReviewAt.Equal(want) {
			t.Fatalf("stage %d next=%v want=%v", stage, p.NextReviewAt, want)
		}
		now = want
	}
	p, err := ApplyKnown(p, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusLearned || p.NextReviewAt != nil || p.LearnedAt == nil {
		t.Fatalf("final progress = %+v", p)
	}
}

func TestUnknownResetsProgress(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 8, 8, 23, 0, 0, 0, loc)
	p := ApplyUnknown(Progress{
		Status: StatusLearning, Stage: 5, ReviewSuccessDays: 5,
	}, now, loc)
	if p.Stage != 0 || p.ReviewSuccessDays != 0 || p.Status != StatusLearning {
		t.Fatalf("progress = %+v", p)
	}
	want := time.Date(2026, 8, 9, 0, 0, 0, 0, loc)
	if p.NextReviewAt == nil || !p.NextReviewAt.Equal(want) {
		t.Fatalf("next=%v want=%v", p.NextReviewAt, want)
	}
}

func TestExpandReviewDatesFromStageZero_BitsUT(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	nextReview := time.Date(2026, 8, 9, 0, 0, 0, 0, loc)
	got := ExpandReviewDates(0, nextReview, loc)
	wantOffsets := []int{0, 1, 4, 7, 14, 30, 60}
	if len(got) != len(wantOffsets) {
		t.Fatalf("len=%d, want=%d, got=%v", len(got), len(wantOffsets), got)
	}
	for i, off := range wantOffsets {
		want := nextReview.AddDate(0, 0, off)
		if !got[i].Equal(want) {
			t.Fatalf("index=%d got=%v want=%v (offset=%d days)", i, got[i], want, off)
		}
	}
}

func TestExpandReviewDatesFromMidStage_BitsUT(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	nextReview := time.Date(2026, 8, 9, 0, 0, 0, 0, loc)
	// stage=3 起：应向后展开 stage 3→4 (7d)、4→5 (16d)、5→6 (30d)，再加最后一次 stage=6 复习
	got := ExpandReviewDates(3, nextReview, loc)
	wantOffsets := []int{0, 7, 23, 53}
	if len(got) != len(wantOffsets) {
		t.Fatalf("len=%d, want=%d, got=%v", len(got), len(wantOffsets), got)
	}
	for i, off := range wantOffsets {
		want := nextReview.AddDate(0, 0, off)
		if !got[i].Equal(want) {
			t.Fatalf("index=%d got=%v want=%v", i, got[i], want)
		}
	}
}

func TestExpandReviewDatesRejectsInvalidStage_BitsUT(t *testing.T) {
	loc := time.UTC
	now := time.Now()
	if got := ExpandReviewDates(-1, now, loc); got != nil {
		t.Fatalf("stage=-1 got=%v, want nil", got)
	}
	if got := ExpandReviewDates(7, now, loc); got != nil {
		t.Fatalf("stage=7 got=%v, want nil", got)
	}
}

func TestMasteredPreservesLearningHistory(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	learnedAt := now.AddDate(0, 0, -1)
	nextReview := now.AddDate(0, 0, 7)
	for _, status := range []string{StatusNotStarted, StatusLearning, StatusLearned} {
		t.Run(status, func(t *testing.T) {
			got := ApplyMastered(Progress{
				Status: status, Stage: 4, ReviewSuccessDays: 3,
				NextReviewAt: &nextReview, LearnedAt: &learnedAt,
			}, now)
			if got.Status != StatusMastered || got.NextReviewAt != nil {
				t.Fatalf("progress = %+v", got)
			}
			if got.LastReviewedAt == nil || !got.LastReviewedAt.Equal(now) ||
				got.MasteredAt == nil || !got.MasteredAt.Equal(now) {
				t.Fatalf("audit times = %+v", got)
			}
			if got.Stage != 4 || got.ReviewSuccessDays != 3 ||
				got.LearnedAt == nil || !got.LearnedAt.Equal(learnedAt) {
				t.Fatalf("history changed = %+v", got)
			}
		})
	}
}
