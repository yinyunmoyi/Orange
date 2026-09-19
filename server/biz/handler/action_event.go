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

	"github.com/cloudwego/hertz/pkg/app"
)

func RecordFavoriteAction(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	itemType := c.Param("itemType")
	itemID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || itemID <= 0 {
		c.JSON(http.StatusBadRequest, model.ItemActionResponse{Code: http.StatusBadRequest, Msg: "invalid favorite item"})
		return
	}
	var req model.ItemActionRequest
	if bindErr := c.BindJSON(&req); bindErr != nil {
		c.JSON(http.StatusBadRequest, model.ItemActionResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	data, err := service.RecordClientItemAction(ctx, itemType, itemID, &req)
	if err != nil {
		status := http.StatusInternalServerError
		message := "record item action failed"
		switch {
		case errors.Is(err, service.ErrItemActionInvalid):
			status, message = http.StatusBadRequest, err.Error()
		case errors.Is(err, service.ErrItemActionNotFound):
			status, message = http.StatusNotFound, err.Error()
		case errors.Is(err, service.ErrItemActionConflict):
			status, message = http.StatusConflict, err.Error()
		case errors.Is(err, db.ErrDBDisabled):
			status, message = http.StatusServiceUnavailable, "database not configured"
		}
		log.Printf("[item-action] failed remote=%s action=%q item_type=%q item_id=%d event_id=%q err=%v elapsed=%s",
			c.ClientIP(), req.Action, itemType, itemID, req.EventID, err, time.Since(started))
		c.JSON(status, model.ItemActionResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.ItemActionResponse{Code: 0, Data: data})
}
