package service

import (
	"context"
	"errors"
	"testing"
	"time"

	learningrule "aaa_word/biz/learning"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetLearningReviewForecastRejectsInvalidDays(t *testing.T) {
	for _, days := range []int{0, -1, 91} {
		if _, err := GetLearningReviewForecast(context.Background(), days); !errors.Is(err, ErrLearningInvalid) {
			t.Fatalf("days=%d err=%v", days, err)
		}
	}
}

func TestGetLearningReviewForecastExpandsFullCurve_BitsUT(t *testing.T) {
	// 一个 stage=1 的词，next_review_at=明天，
	// 按 stageIntervals 从 stage=1 起后续复习偏移 3、6、13、29、59 天。
	// forecast 应把这 6 次全部落到对应日期桶里。
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now().In(learningLocation())
	tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, learningLocation()).AddDate(0, 0, 1)

	mock.ExpectQuery("SELECT `next_review_at`,`memory_stage` FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? AND next_review_at IS NOT NULL").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusLearning).
		WillReturnRows(sqlmock.NewRows([]string{"next_review_at", "memory_stage"}).
			AddRow(tomorrow, 1))

	data, err := GetLearningReviewForecast(context.Background(), 90)
	if err != nil {
		t.Fatalf("GetLearningReviewForecast() error = %v", err)
	}
	if data.Total != 6 {
		t.Fatalf("total = %d, want 6 (完整复习曲线：明天 + 后续 5 次复习)", data.Total)
	}
	nonZero := make(map[string]int64)
	for _, item := range data.Items {
		if item.Count > 0 {
			nonZero[item.Date] = item.Count
		}
	}
	// stage=1 起始日期偏移：0、3、6、13、29、59
	wantOffsets := []int{0, 3, 6, 13, 29, 59}
	if len(nonZero) != len(wantOffsets) {
		t.Fatalf("非零桶数 = %d, want %d, nonZero=%v", len(nonZero), len(wantOffsets), nonZero)
	}
	for _, off := range wantOffsets {
		date := tomorrow.AddDate(0, 0, off).Format("2006-01-02")
		if nonZero[date] != 1 {
			t.Fatalf("日期 %s (偏移 %d) 计数 = %d, want 1, all=%v", date, off, nonZero[date], nonZero)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
