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

func GetNote(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	itemType := c.Query("itemType")
	text := c.Query("text")
	data, err := service.GetNote(ctx, itemType, text)
	if err != nil {
		status, message := noteHTTPError(err)
		log.Printf("[note-get] failed remote=%s item_type=%q text=%q status=%d err=%v elapsed=%s",
			c.ClientIP(), itemType, text, status, err, time.Since(started))
		c.JSON(status, model.NoteResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.NoteResponse{Code: 0, Data: data})
}

func PutNote(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	var req model.NoteRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[note-save] bind failed remote=%s raw_body=%q err=%v elapsed=%s",
			c.ClientIP(), logSnippet(string(c.Request.Body()), 500), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.NoteResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	data, err := service.SaveNote(ctx, &req)
	if err != nil {
		status, message := noteHTTPError(err)
		log.Printf("[note-save] failed remote=%s item_type=%q text=%q note_length=%d status=%d err=%v elapsed=%s",
			c.ClientIP(), req.ItemType, req.Text, len([]rune(req.Note)), status, err, time.Since(started))
		c.JSON(status, model.NoteResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.NoteResponse{Code: 0, Data: data})
}

func noteHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrNoteInvalid):
		return http.StatusBadRequest, service.ErrNoteInvalid.Error()
	case errors.Is(err, service.ErrNoteTooLong):
		return http.StatusBadRequest, service.ErrNoteTooLong.Error()
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "note operation failed"
	}
}
