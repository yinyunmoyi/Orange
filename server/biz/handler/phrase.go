package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
	"gorm.io/gorm"
)

func PhraseFavorite(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	var req model.PhraseFavoriteRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[phrase-favorite] bind failed remote=%s raw_body=%q err=%v elapsed=%s",
			c.ClientIP(), logSnippet(string(c.Request.Body()), 500), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.PhraseFavoriteResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	itemID, err := service.AddPhraseFavorite(ctx, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPhraseInvalid):
			log.Printf("[phrase-favorite] invalid remote=%s phrase=%q err=%v elapsed=%s",
				c.ClientIP(), req.Phrase, err, time.Since(started))
			c.JSON(http.StatusBadRequest, model.PhraseFavoriteResponse{Code: http.StatusBadRequest, Msg: err.Error()})
		case errors.Is(err, db.ErrDBDisabled), errors.Is(err, service.ErrTTSNotConfigured):
			log.Printf("[phrase-favorite] unavailable remote=%s phrase=%q err=%v elapsed=%s",
				c.ClientIP(), req.Phrase, err, time.Since(started))
			c.JSON(http.StatusServiceUnavailable, model.PhraseFavoriteResponse{Code: http.StatusServiceUnavailable, Msg: err.Error()})
		default:
			log.Printf("[phrase-favorite] failed remote=%s phrase=%q err=%v elapsed=%s",
				c.ClientIP(), req.Phrase, err, time.Since(started))
			c.JSON(http.StatusInternalServerError, model.PhraseFavoriteResponse{Code: http.StatusInternalServerError, Msg: "favorite phrase failed"})
		}
		return
	}
	log.Printf("[phrase-favorite] ok remote=%s phrase=%q item_id=%d elapsed=%s",
		c.ClientIP(), req.Phrase, itemID, time.Since(started))
	c.JSON(http.StatusOK, model.PhraseFavoriteResponse{
		Code: 0,
		Data: &model.PhraseFavoriteData{ItemID: itemID},
	})
}

func PhraseFavoriteStatus(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	raw := c.Query("phrase")
	favorited, itemID, err := service.IsPhraseFavorited(ctx, raw)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPhraseInvalid):
			log.Printf("[phrase-favorite-status] invalid remote=%s phrase=%q err=%v", c.ClientIP(), raw, err)
			c.JSON(http.StatusBadRequest, model.PhraseFavoriteStatusResponse{Code: http.StatusBadRequest, Msg: err.Error()})
		case errors.Is(err, db.ErrDBDisabled):
			log.Printf("[phrase-favorite-status] db disabled remote=%s phrase=%q", c.ClientIP(), raw)
			c.JSON(http.StatusOK, model.PhraseFavoriteStatusResponse{
				Code: 0,
				Data: &model.PhraseFavoriteStatusData{Favorited: false},
			})
		default:
			log.Printf("[phrase-favorite-status] failed remote=%s phrase=%q err=%v elapsed=%s",
				c.ClientIP(), raw, err, time.Since(started))
			c.JSON(http.StatusInternalServerError, model.PhraseFavoriteStatusResponse{Code: http.StatusInternalServerError, Msg: "get phrase favorite status failed"})
		}
		return
	}
	log.Printf("[phrase-favorite-status] ok remote=%s phrase=%q favorited=%t item_id=%d elapsed=%s",
		c.ClientIP(), raw, favorited, itemID, time.Since(started))
	c.JSON(http.StatusOK, model.PhraseFavoriteStatusResponse{
		Code: 0,
		Data: &model.PhraseFavoriteStatusData{Favorited: favorited, ItemID: itemID},
	})
}

func PhraseDetail(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		log.Printf("[phrase-detail] invalid id remote=%s raw=%q err=%v", c.ClientIP(), c.Param("id"), err)
		c.JSON(http.StatusBadRequest, model.PhraseDetailResponse{Code: http.StatusBadRequest, Msg: "invalid phrase id"})
		return
	}
	data, err := service.GetPhraseDetail(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPhraseNotFound):
			c.JSON(http.StatusNotFound, model.PhraseDetailResponse{Code: http.StatusNotFound, Msg: err.Error()})
		case errors.Is(err, db.ErrDBDisabled):
			c.JSON(http.StatusServiceUnavailable, model.PhraseDetailResponse{Code: http.StatusServiceUnavailable, Msg: "database not configured"})
		default:
			log.Printf("[phrase-detail] failed remote=%s phrase_id=%d err=%v elapsed=%s",
				c.ClientIP(), id, err, time.Since(started))
			c.JSON(http.StatusInternalServerError, model.PhraseDetailResponse{Code: http.StatusInternalServerError, Msg: "get phrase detail failed"})
		}
		return
	}
	c.JSON(http.StatusOK, model.PhraseDetailResponse{Code: 0, Data: data})
}

func PhraseAudio(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		log.Printf("[phrase-audio] invalid id remote=%s raw=%q err=%v", c.ClientIP(), c.Param("id"), err)
		c.JSON(http.StatusBadRequest, model.PhraseDetailResponse{Code: http.StatusBadRequest, Msg: "invalid phrase id"})
		return
	}
	path, contentType, err := service.LookupPhraseAudio(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, service.ErrPhraseNotFound):
			c.JSON(http.StatusNotFound, model.PhraseDetailResponse{Code: http.StatusNotFound, Msg: "phrase audio not found"})
		case errors.Is(err, db.ErrDBDisabled):
			c.JSON(http.StatusServiceUnavailable, model.PhraseDetailResponse{Code: http.StatusServiceUnavailable, Msg: "database not configured"})
		default:
			log.Printf("[phrase-audio] lookup failed remote=%s phrase_id=%d err=%v elapsed=%s",
				c.ClientIP(), id, err, time.Since(started))
			c.JSON(http.StatusInternalServerError, model.PhraseDetailResponse{Code: http.StatusInternalServerError, Msg: "get phrase audio failed"})
		}
		return
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, model.PhraseDetailResponse{Code: http.StatusNotFound, Msg: "phrase audio not found"})
			return
		}
		log.Printf("[phrase-audio] stat failed remote=%s phrase_id=%d path=%q err=%v elapsed=%s",
			c.ClientIP(), id, path, err, time.Since(started))
		c.JSON(http.StatusInternalServerError, model.PhraseDetailResponse{Code: http.StatusInternalServerError, Msg: "get phrase audio failed"})
		return
	}
	log.Printf("[phrase-audio] ok remote=%s phrase_id=%d path=%q elapsed=%s",
		c.ClientIP(), id, path, time.Since(started))
	c.SetContentType(contentType)
	c.File(path)
}
