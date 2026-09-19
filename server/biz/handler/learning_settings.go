package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
)

func GetLearningSettings(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	data, err := service.GetLearningSettings(ctx)
	if err != nil {
		status, message := learningSettingsHTTPError(err)
		log.Printf("[learning-settings-get] failed remote=%s status=%d err=%v elapsed=%s",
			c.ClientIP(), status, err, time.Since(started))
		c.JSON(status, model.LearningSettingsResponse{Code: status, Msg: message})
		return
	}
	log.Printf("[learning-settings-get] ok remote=%s new_limit=%d review_limit=%d remaining_new=%d study_date=%s elapsed=%s",
		c.ClientIP(), data.DailyNewLimit, data.DailyReviewLimit, data.RemainingNewCount,
		data.StudyDate, time.Since(started))
	c.JSON(http.StatusOK, model.LearningSettingsResponse{Code: 0, Data: data})
}

func PutLearningSettings(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	var req model.LearningSettingsRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[learning-settings-put] bind failed remote=%s raw_body=%q err=%v elapsed=%s",
			c.ClientIP(), logSnippet(string(c.Request.Body()), 500), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.LearningSettingsResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid request body",
		})
		return
	}
	data, err := service.UpdateLearningSettings(ctx, &req)
	if err != nil {
		status, message := learningSettingsHTTPError(err)
		log.Printf("[learning-settings-put] failed remote=%s new_limit=%d review_limit=%d status=%d err=%v elapsed=%s",
			c.ClientIP(), req.DailyNewLimit, req.DailyReviewLimit, status, err, time.Since(started))
		c.JSON(status, model.LearningSettingsResponse{Code: status, Msg: message})
		return
	}
	log.Printf("[learning-settings-put] ok remote=%s new_limit=%d review_limit=%d remaining_new=%d study_date=%s elapsed=%s",
		c.ClientIP(), data.DailyNewLimit, data.DailyReviewLimit, data.RemainingNewCount,
		data.StudyDate, time.Since(started))
	c.JSON(http.StatusOK, model.LearningSettingsResponse{Code: 0, Data: data})
}

func learningSettingsHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrLearningSettingsInvalid):
		return http.StatusBadRequest, "invalid learning settings"
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "learning settings operation failed"
	}
}
