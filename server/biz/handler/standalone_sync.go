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

func SyncStandaloneFavorite(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	var req model.StandaloneFavoriteSyncRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.StandaloneFavoriteSyncResponse{
			Code: http.StatusBadRequest, Msg: "invalid request body",
		})
		return
	}
	data, err := service.SyncStandaloneFavorite(ctx, &req)
	if err != nil {
		status, message := standaloneSyncHTTPError(err)
		log.Printf("[standalone-sync] failed client_id=%q item_type=%q status=%d err=%v elapsed=%s",
			req.ClientID, req.ItemType, status, err, time.Since(started))
		c.JSON(status, model.StandaloneFavoriteSyncResponse{Code: status, Msg: message})
		return
	}
	log.Printf("[standalone-sync] ok client_id=%q item_type=%q item_id=%d idempotent=%t elapsed=%s",
		data.ClientID, data.ItemType, data.ItemID, data.Idempotent, time.Since(started))
	c.JSON(http.StatusOK, model.StandaloneFavoriteSyncResponse{Code: 0, Data: data})
}

func standaloneSyncHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrStandaloneSyncInvalid):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrStandaloneSyncConflict):
		return http.StatusConflict, err.Error()
	case errors.Is(err, db.ErrDBDisabled),
		errors.Is(err, service.ErrLLMNotConfigured),
		errors.Is(err, service.ErrTTSNotConfigured):
		return http.StatusServiceUnavailable, "standalone sync unavailable"
	default:
		return http.StatusInternalServerError, "standalone sync failed"
	}
}
