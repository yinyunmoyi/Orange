package handler

import (
	"context"
	"errors"
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

// ResetFavoriteLearning 处理 POST /api/v1/favorites/:itemType/:id/learning/reset。
// 把收藏项的学习进度重置为刚收藏时的状态，收藏关系与语境、备注保持不变。
func ResetFavoriteLearning(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	itemType := strings.ToLower(strings.TrimSpace(c.Param("itemType")))
	itemID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || itemID <= 0 || (itemType != service.ContextItemWord && itemType != service.ContextItemPhrase) {
		log.Printf("[favorite-reset] invalid request remote=%s item_type=%q raw_id=%q err=%v elapsed=%s",
			c.ClientIP(), itemType, c.Param("id"), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.FavoriteResetResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid favorite reset request",
		})
		return
	}

	if err := service.ResetFavoriteLearning(ctx, itemType, itemID); err != nil {
		status, message := favoriteResetErrorResponse(err)
		log.Printf("[favorite-reset] failed remote=%s item_type=%q item_id=%d status=%d err=%v elapsed=%s",
			c.ClientIP(), itemType, itemID, status, err, time.Since(started))
		c.JSON(status, model.FavoriteResetResponse{Code: status, Msg: message})
		return
	}
	log.Printf("[favorite-reset] ok remote=%s item_type=%q item_id=%d elapsed=%s",
		c.ClientIP(), itemType, itemID, time.Since(started))
	c.JSON(http.StatusOK, model.FavoriteResetResponse{Code: 0})
}

func favoriteResetErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrFavoriteResetInvalid):
		return http.StatusBadRequest, "invalid favorite reset request"
	case errors.Is(err, service.ErrFavoriteResetNotFound):
		return http.StatusNotFound, "favorite item not found"
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "reset favorite learning failed"
	}
}
