package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	learningSettingsID        = 1
	LearningReviewUnlimited   = 9999
	minDailyNewLimit          = 5
	maxDailyNewLimit          = 500
	minDailyReviewLimit       = 5
	maxFiniteDailyReviewLimit = 1000
	learningSettingsLimitStep = 5
)

var ErrLearningSettingsInvalid = errors.New("invalid learning settings")

func GetLearningSettings(ctx context.Context) (*model.LearningSettingsData, error) {
	if !db.Enabled() {
		log.Printf("[learning-settings] get failed err=%v", db.ErrDBDisabled)
		return nil, db.ErrDBDisabled
	}
	settings, err := loadEffectiveLearningSettings(db.DB.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return buildLearningSettingsData(ctx, db.DB.WithContext(ctx), settings)
}

func UpdateLearningSettings(
	ctx context.Context,
	req *model.LearningSettingsRequest,
) (*model.LearningSettingsData, error) {
	if req == nil || !validDailyNewLimit(req.DailyNewLimit) ||
		!validDailyReviewLimit(req.DailyReviewLimit) {
		return nil, ErrLearningSettingsInvalid
	}
	if !db.Enabled() {
		log.Printf("[learning-settings] update failed new_limit=%d review_limit=%d err=%v",
			req.DailyNewLimit, req.DailyReviewLimit, db.ErrDBDisabled)
		return nil, db.ErrDBDisabled
	}

	now := time.Now()
	row := db.LearningSettings{
		ID:               learningSettingsID,
		DailyNewLimit:    req.DailyNewLimit,
		DailyReviewLimit: req.DailyReviewLimit,
		UpdatedAt:        now,
	}
	if err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"daily_new_limit":    req.DailyNewLimit,
			"daily_review_limit": req.DailyReviewLimit,
			"updated_at":         now,
		}),
	}).Create(&row).Error; err != nil {
		log.Printf("[learning-settings] upsert failed new_limit=%d review_limit=%d err=%v",
			req.DailyNewLimit, req.DailyReviewLimit, err)
		return nil, fmt.Errorf("update learning settings: %w", err)
	}
	return buildLearningSettingsData(ctx, db.DB.WithContext(ctx), row)
}

func validDailyNewLimit(value int) bool {
	return value >= minDailyNewLimit &&
		value <= maxDailyNewLimit &&
		value%learningSettingsLimitStep == 0
}

func validDailyReviewLimit(value int) bool {
	return value == LearningReviewUnlimited ||
		(value >= minDailyReviewLimit &&
			value <= maxFiniteDailyReviewLimit &&
			value%learningSettingsLimitStep == 0)
}

func loadEffectiveLearningSettings(query *gorm.DB) (db.LearningSettings, error) {
	var row db.LearningSettings
	if err := query.Where("id = ?", learningSettingsID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return db.LearningSettings{
				ID:               learningSettingsID,
				DailyNewLimit:    config.LearningDailyNewLimit(),
				DailyReviewLimit: LearningReviewUnlimited,
			}, nil
		}
		log.Printf("[learning-settings] query failed id=%d err=%v", learningSettingsID, err)
		return db.LearningSettings{}, fmt.Errorf("query learning settings: %w", err)
	}
	return row, nil
}

func buildLearningSettingsData(
	ctx context.Context,
	query *gorm.DB,
	settings db.LearningSettings,
) (*model.LearningSettingsData, error) {
	var remaining int64
	if err := query.WithContext(ctx).Model(&db.UserItemLearning{}).
		Where(
			"item_type IN ? AND learning_status = ?",
			[]int{LearningItemWord, LearningItemPhrase},
			learningrule.StatusNotStarted,
		).
		Count(&remaining).Error; err != nil {
		log.Printf("[learning-settings] count remaining failed err=%v", err)
		return nil, fmt.Errorf("count remaining new items: %w", err)
	}
	now := time.Now().In(learningLocation())
	return &model.LearningSettingsData{
		DailyNewLimit:     settings.DailyNewLimit,
		DailyReviewLimit:  settings.DailyReviewLimit,
		RemainingNewCount: remaining,
		StudyDate:         now.Format("2006-01-02"),
	}, nil
}
