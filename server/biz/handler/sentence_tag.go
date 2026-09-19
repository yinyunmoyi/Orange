package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
)

func CreateSentenceTag(ctx context.Context, c *app.RequestContext) {
	var req model.SentenceTagCreateRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.SentenceTagResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid request body",
		})
		return
	}
	data, err := service.CreateSentenceTag(ctx, &req)
	if err != nil {
		status, message := sentenceTagHTTPError(err)
		c.JSON(status, model.SentenceTagResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceTagResponse{Code: 0, Data: data})
}

func ListSentenceTags(ctx context.Context, c *app.RequestContext) {
	data, err := service.ListSentenceTags(ctx)
	if err != nil {
		status, message := sentenceTagHTTPError(err)
		c.JSON(status, model.SentenceTagListResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceTagListResponse{Code: 0, Data: data})
}

func ReplaceSentenceFavoriteTags(ctx context.Context, c *app.RequestContext) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.SentenceFavoriteResponse{
			Code: http.StatusBadRequest,
			Msg:  service.ErrSentenceTagInvalid.Error(),
		})
		return
	}
	var req model.SentenceFavoriteTagsUpdateRequest
	if bindErr := c.BindJSON(&req); bindErr != nil {
		c.JSON(http.StatusBadRequest, model.SentenceFavoriteResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid request body",
		})
		return
	}
	data, err := service.ReplaceSentenceFavoriteTags(ctx, id, req.TagIDs, req.Note)
	if err != nil {
		status, message := sentenceTagHTTPError(err)
		c.JSON(status, model.SentenceFavoriteResponse{Code: status, Msg: message})
		return
	}
	c.JSON(http.StatusOK, model.SentenceFavoriteResponse{Code: 0, Data: data})
}

func parseSentenceTagIDs(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			return nil, service.ErrSentenceTagInvalid
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) > service.MaxSentenceTagsPerItem {
			return nil, service.ErrSentenceTagInvalid
		}
	}
	return ids, nil
}

func sentenceTagHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrSentenceTagInvalid),
		errors.Is(err, service.ErrSentenceTagDuplicate):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrSentenceFavoriteNotFound),
		errors.Is(err, service.ErrSentenceTagNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, service.ErrSentenceTagConflict):
		return http.StatusConflict, err.Error()
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "sentence tag operation failed"
	}
}
