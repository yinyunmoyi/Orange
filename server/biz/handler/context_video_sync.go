package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"
	"aaa_word/biz/syncidentity"

	"github.com/cloudwego/hertz/pkg/app"
)

func ContextVideoSyncPing(_ context.Context, c *app.RequestContext) {
	serverID, err := syncidentity.Current()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ContextVideoSyncPingResponse{
			Code: http.StatusInternalServerError,
			Msg:  "context video sync unavailable",
		})
		return
	}
	c.JSON(http.StatusOK, model.ContextVideoSyncPingResponse{
		Code: 0,
		Data: &model.ContextVideoSyncPingData{
			ServerID:   serverID,
			APIVersion: 1,
		},
	})
}

func ContextVideoSyncManifest(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	days, err := syncPositiveQueryInt(
		string(c.Query("days")),
		service.DefaultContextVideoSyncDays,
		service.MaxContextVideoSyncDays,
	)
	if err != nil {
		writeContextVideoSyncError(c, http.StatusBadRequest, "invalid days")
		return
	}
	limit, err := syncPositiveQueryInt(
		string(c.Query("limit")),
		service.DefaultContextVideoSyncLimit,
		service.MaxContextVideoSyncLimit,
	)
	if err != nil {
		writeContextVideoSyncError(c, http.StatusBadRequest, "invalid limit")
		return
	}
	serverID, err := syncidentity.Current()
	if err != nil {
		writeContextVideoSyncError(c, http.StatusInternalServerError, "context video sync unavailable")
		return
	}
	data, err := service.GetContextVideoSyncManifest(ctx, serverID, days, limit)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrLearningInvalid):
			writeContextVideoSyncError(c, http.StatusBadRequest, "invalid sync request")
		case errors.Is(err, db.ErrDBDisabled):
			writeContextVideoSyncError(c, http.StatusServiceUnavailable, "database not configured")
		default:
			log.Printf("[context-video-sync] manifest failed remote=%s days=%d limit=%d err=%v elapsed=%s",
				c.ClientIP(), days, limit, err, time.Since(started))
			writeContextVideoSyncError(c, http.StatusInternalServerError, "get sync manifest failed")
		}
		return
	}
	log.Printf("[context-video-sync] manifest ok remote=%s count=%d days=%d elapsed=%s",
		c.ClientIP(), len(data.Items), days, time.Since(started))
	c.JSON(http.StatusOK, model.ContextVideoSyncResponse{Code: 0, Data: data})
}

func syncPositiveQueryInt(raw string, fallback, max int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > max {
		return 0, service.ErrLearningInvalid
	}
	return value, nil
}

func writeContextVideoSyncError(c *app.RequestContext, status int, message string) {
	c.JSON(status, model.ContextVideoSyncResponse{Code: status, Msg: message})
}
