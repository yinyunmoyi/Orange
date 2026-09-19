package service

import (
	"context"
	"errors"
	"testing"

	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetLearningSettingsFallsBackToExistingDefaults_BitsUT(t *testing.T) {
	t.Setenv("LEARNING_DAILY_NEW_LIMIT", "35")
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `learning_settings` WHERE id = \\? LIMIT \\?").
		WithArgs(learningSettingsID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "daily_new_limit", "daily_review_limit",
		}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\?").
		WithArgs(LearningItemWord, LearningItemPhrase, "not_started").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(774))

	data, err := GetLearningSettings(context.Background())
	if err != nil {
		t.Fatalf("GetLearningSettings() error = %v", err)
	}
	if data.DailyNewLimit != 35 || data.DailyReviewLimit != LearningReviewUnlimited {
		t.Fatalf("limits = %d/%d, want 35/%d",
			data.DailyNewLimit, data.DailyReviewLimit, LearningReviewUnlimited)
	}
	if data.RemainingNewCount != 774 || data.StudyDate == "" {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateLearningSettingsRejectsInvalidValues_BitsUT(t *testing.T) {
	tests := []*model.LearningSettingsRequest{
		nil,
		{DailyNewLimit: 4, DailyReviewLimit: 100},
		{DailyNewLimit: 31, DailyReviewLimit: 100},
		{DailyNewLimit: 30, DailyReviewLimit: 1001},
		{DailyNewLimit: 30, DailyReviewLimit: 101},
	}
	for _, request := range tests {
		if _, err := UpdateLearningSettings(context.Background(), request); !errors.Is(err, ErrLearningSettingsInvalid) {
			t.Fatalf("request=%+v err=%v, want ErrLearningSettingsInvalid", request, err)
		}
	}
}

func TestUpdateLearningSettingsUpsertsAndReturnsCount_BitsUT(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `learning_settings`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\?").
		WithArgs(LearningItemWord, LearningItemPhrase, "not_started").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))

	data, err := UpdateLearningSettings(context.Background(), &model.LearningSettingsRequest{
		DailyNewLimit:    30,
		DailyReviewLimit: LearningReviewUnlimited,
	})
	if err != nil {
		t.Fatalf("UpdateLearningSettings() error = %v", err)
	}
	if data.DailyNewLimit != 30 || data.DailyReviewLimit != LearningReviewUnlimited ||
		data.RemainingNewCount != 20 {
		t.Fatalf("unexpected data: %+v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
