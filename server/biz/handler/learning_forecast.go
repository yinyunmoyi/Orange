package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
)

func LearningReviewForecast(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	rawDays := strings.TrimSpace(c.Query("days"))
	days, err := parseForecastDays(rawDays)
	if err != nil {
		log.Printf("[learning-forecast] invalid days raw=%q remote=%s err=%v", rawDays, c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, model.LearningReviewForecastResponse{
			Code: http.StatusBadRequest,
			Msg:  "days must be between 1 and 90",
		})
		return
	}
	data, err := service.GetLearningReviewForecast(ctx, days)
	if err != nil {
		log.Printf("[learning-forecast] failed days=%d remote=%s err=%v elapsed=%s",
			days, c.ClientIP(), err, time.Since(start))
		switch {
		case errors.Is(err, db.ErrDBDisabled):
			c.JSON(http.StatusServiceUnavailable, model.LearningReviewForecastResponse{
				Code: http.StatusServiceUnavailable,
				Msg:  "database not configured",
			})
		case errors.Is(err, service.ErrLearningInvalid):
			c.JSON(http.StatusBadRequest, model.LearningReviewForecastResponse{
				Code: http.StatusBadRequest,
				Msg:  "invalid forecast range",
			})
		default:
			c.JSON(http.StatusInternalServerError, model.LearningReviewForecastResponse{
				Code: http.StatusInternalServerError,
				Msg:  "get review forecast failed",
			})
		}
		return
	}
	log.Printf("[learning-forecast] ok days=%d total=%d start=%s end=%s elapsed=%s",
		days, data.Total, data.StartDate, data.EndDate, time.Since(start))
	c.JSON(http.StatusOK, model.LearningReviewForecastResponse{Code: 0, Data: data})
}

func parseForecastDays(raw string) (int, error) {
	if raw == "" {
		return 14, nil
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > 90 {
		return 0, fmt.Errorf("days must be between 1 and 90")
	}
	return days, nil
}
