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

func RegenerateFavoriteAudio(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	itemType := strings.ToLower(strings.TrimSpace(c.Param("itemType")))
	itemID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || itemID <= 0 || (itemType != service.ContextItemWord && itemType != service.ContextItemPhrase) {
		log.Printf("[favorite-audio] invalid request remote=%s item_type=%q raw_id=%q err=%v elapsed=%s",
			c.ClientIP(), itemType, c.Param("id"), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.FavoriteAudioResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid favorite audio request",
		})
		return
	}

	data, err := service.RegenerateFavoriteAudio(ctx, itemType, itemID)
	if err != nil {
		status, message := favoriteAudioErrorResponse(err)
		log.Printf("[favorite-audio] failed remote=%s item_type=%q item_id=%d status=%d err=%v elapsed=%s",
			c.ClientIP(), itemType, itemID, status, err, time.Since(started))
		c.JSON(status, model.FavoriteAudioResponse{Code: status, Msg: message})
		return
	}

	log.Printf("[favorite-audio] ok remote=%s item_type=%q item_id=%d audio_url=%q elapsed=%s",
		c.ClientIP(), itemType, itemID, data.AudioURL, time.Since(started))
	c.JSON(http.StatusOK, model.FavoriteAudioResponse{Code: 0, Data: data})
}

func favoriteAudioErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrFavoriteAudioInvalid):
		return http.StatusBadRequest, "invalid favorite audio request"
	case errors.Is(err, service.ErrFavoriteAudioNotFound):
		return http.StatusNotFound, "favorite item not found"
	case errors.Is(err, db.ErrDBDisabled), errors.Is(err, service.ErrTTSNotConfigured):
		return http.StatusServiceUnavailable, "audio regeneration unavailable"
	case errors.Is(err, service.ErrTTSUpstream):
		return http.StatusBadGateway, "audio synthesis failed"
	default:
		return http.StatusInternalServerError, "regenerate favorite audio failed"
	}
}
