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

func SaveSentenceFavorite(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	var req model.SentenceFavoriteRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.SentenceFavoriteResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid request body",
		})
		return
	}
	data, err := service.SaveSentenceFavorite(ctx, &req)
	if err != nil {
		status, message := sentenceFavoriteHTTPError(err)
		log.Printf("[sentence-favorite-save] failed remote=%s sentence=%q status=%d err=%v elapsed=%s",
			c.ClientIP(), logSnippet(req.Sentence, 200), status, err, time.Since(started))
		c.JSON(status, model.SentenceFavoriteResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceFavoriteResponse{Code: 0, Data: data})
}

func SentenceFavoriteStatus(ctx context.Context, c *app.RequestContext) {
	data, err := service.GetSentenceFavoriteStatus(ctx, c.Query("sentence"))
	if err != nil {
		status, message := sentenceFavoriteHTTPError(err)
		c.JSON(status, model.SentenceFavoriteStatusResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceFavoriteStatusResponse{Code: 0, Data: data})
}

func ListSentenceFavorites(ctx context.Context, c *app.RequestContext) {
	tagIDs, err := parseSentenceTagIDs(c.Query("tagIds"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.SentenceFavoriteListResponse{
			Code: http.StatusBadRequest,
			Msg:  err.Error(),
		})
		return
	}
	data, err := service.ListSentenceFavorites(ctx, tagIDs...)
	if err != nil {
		status, message := sentenceFavoriteHTTPError(err)
		c.JSON(status, model.SentenceFavoriteListResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceFavoriteListResponse{Code: 0, Data: data})
}

func GetSentenceFavorite(ctx context.Context, c *app.RequestContext) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.SentenceFavoriteResponse{
			Code: http.StatusBadRequest,
			Msg:  service.ErrSentenceFavoriteInvalid.Error(),
		})
		return
	}
	data, err := service.GetSentenceFavorite(ctx, id)
	if err != nil {
		status, message := sentenceFavoriteHTTPError(err)
		c.JSON(status, model.SentenceFavoriteResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceFavoriteResponse{Code: 0, Data: data})
}

func sentenceFavoriteHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrSentenceFavoriteInvalid),
		errors.Is(err, service.ErrSentenceTranslationLong):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrSentenceFavoriteNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "sentence favorite operation failed"
	}
}
