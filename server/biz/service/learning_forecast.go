package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"
)

const maxReviewForecastDays = 90

func GetLearningReviewForecast(ctx context.Context, days int) (*model.LearningReviewForecastData, error) {
	if days < 1 || days > maxReviewForecastDays {
		return nil, ErrLearningInvalid
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	now := time.Now()
	loc := learningLocation()
	start, end := learningrule.ForecastWindow(now, loc, days)
	var rows []db.UserItemLearning
	if err := db.DB.WithContext(ctx).
		Select("next_review_at", "memory_stage").
		Where(
			"item_type IN ? AND learning_status = ? AND next_review_at IS NOT NULL",
			[]int{LearningItemWord, LearningItemPhrase}, learningrule.StatusLearning,
		).
		Find(&rows).Error; err != nil {
		log.Printf("[learning-forecast] query failed days=%d start=%s end=%s err=%v",
			days, start.Format(time.RFC3339), end.Format(time.RFC3339), err)
		return nil, fmt.Errorf("query learning review forecast: %w", err)
	}
	reviewTimes := make([]time.Time, 0, len(rows)*4)
	for _, row := range rows {
		if row.NextReviewAt == nil {
			continue
		}
		// 沿 stageIntervals 展开该词剩余的完整复习曲线，让预览图与实际复习计划一致
		for _, dt := range learningrule.ExpandReviewDates(row.MemoryStage, *row.NextReviewAt, loc) {
			if !dt.Before(start) && dt.Before(end) {
				reviewTimes = append(reviewTimes, dt)
			}
		}
	}
	buckets := learningrule.BuildForecast(now, loc, days, reviewTimes)
	data := &model.LearningReviewForecastData{
		Days:  days,
		Items: make([]model.LearningReviewForecastItem, 0, len(buckets)),
	}
	for _, bucket := range buckets {
		item := model.LearningReviewForecastItem{
			Date:  bucket.Date.Format("2006-01-02"),
			Count: bucket.Count,
		}
		data.Items = append(data.Items, item)
		data.Total += item.Count
	}
	if len(data.Items) > 0 {
		data.StartDate = data.Items[0].Date
		data.EndDate = data.Items[len(data.Items)-1].Date
	}
	return data, nil
}
