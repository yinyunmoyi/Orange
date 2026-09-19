package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
)

func UploadContextVideo(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	contextID, err := parsePositiveInt64(c.Param("contextId"))
	if err != nil {
		writeContextVideoError(c, http.StatusBadRequest, "invalid context id")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader.Size <= 0 || fileHeader.Size > service.MaxContextVideoBytes {
		writeContextVideoError(c, http.StatusBadRequest, "invalid video file")
		return
	}
	clipStartMS, startErr := parsePositiveOrZeroInt64(c.PostForm("clipStartMs"))
	clipEndMS, endErr := parsePositiveInt64(c.PostForm("clipEndMs"))
	durationMS, durationErr := parsePositiveInt64(c.PostForm("durationMs"))
	var subtitles []model.ContextVideoSubtitle
	subtitleErr := json.Unmarshal([]byte(c.PostForm("subtitles")), &subtitles)
	if startErr != nil || endErr != nil || durationErr != nil || subtitleErr != nil {
		writeContextVideoError(c, http.StatusBadRequest, "invalid video metadata")
		return
	}
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/webm"
	}
	var audioFile multipart.File
	audioContentType := ""
	audioFileSize := int64(0)
	audioHeader, audioHeaderErr := c.FormFile("audioFile")
	if audioHeaderErr == nil {
		if audioHeader.Size <= 0 || audioHeader.Size > service.MaxContextAudioBytes {
			writeContextVideoError(c, http.StatusBadRequest, "invalid audio file")
			return
		}
		audioContentType = audioHeader.Header.Get("Content-Type")
		if audioContentType == "" {
			audioContentType = "audio/webm"
		}
		audioFileSize = audioHeader.Size
		audioFile, err = audioHeader.Open()
		if err != nil {
			writeContextVideoError(c, http.StatusBadRequest, "invalid audio file")
			return
		}
		defer audioFile.Close()
	}
	req := &model.ContextVideoUploadRequest{
		ContextID:         contextID,
		SourceFingerprint: c.PostForm("sourceFingerprint"),
		ClipStartMS:       clipStartMS,
		ClipEndMS:         clipEndMS,
		DurationMS:        durationMS,
		ContentType:       contentType,
		FileSize:          fileHeader.Size,
		AudioContentType:  audioContentType,
		AudioFileSize:     audioFileSize,
		Subtitles:         subtitles,
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeContextVideoError(c, http.StatusBadRequest, "invalid video file")
		return
	}
	defer file.Close()

	data, err := service.SaveContextVideo(ctx, req, file, audioFile)
	if err != nil {
		status, message := contextVideoError(err)
		log.Printf("[context-video] upload failed context_id=%d remote=%s err=%v elapsed=%s",
			contextID, c.ClientIP(), err, time.Since(started))
		writeContextVideoError(c, status, message)
		return
	}
	log.Printf("[context-video] upload ok context_id=%d video_id=%d bytes=%d remote=%s elapsed=%s",
		contextID, data.ID, fileHeader.Size, c.ClientIP(), time.Since(started))
	c.JSON(http.StatusOK, model.ContextVideoResponse{Code: 0, Data: data})
}

func ContextVideoFile(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	videoID, err := parsePositiveInt64(c.Param("videoId"))
	if err != nil {
		writeContextVideoError(c, http.StatusBadRequest, "invalid video id")
		return
	}
	row, err := service.LookupContextVideo(ctx, videoID)
	if err != nil {
		status, message := contextVideoError(err)
		writeContextVideoError(c, status, message)
		return
	}
	if _, err := os.Stat(row.FilePath); err != nil {
		writeContextVideoError(c, http.StatusNotFound, "context video not found")
		return
	}
	if contextVideoNotModified(c, row.DedupeKey) {
		return
	}
	log.Printf("[context-video] stream video_id=%d path=%q remote=%s elapsed=%s",
		videoID, row.FilePath, c.ClientIP(), time.Since(started))
	c.SetContentType(row.ContentType)
	c.File(row.FilePath)
}

func ContextVideoSubtitles(ctx context.Context, c *app.RequestContext) {
	videoID, err := parsePositiveInt64(c.Param("videoId"))
	if err != nil {
		writeContextVideoError(c, http.StatusBadRequest, "invalid video id")
		return
	}
	row, err := service.LookupContextVideo(ctx, videoID)
	if err != nil {
		status, message := contextVideoError(err)
		writeContextVideoError(c, status, message)
		return
	}
	vtt, err := service.ContextVideoVTT(row)
	if err != nil {
		log.Printf("[context-video] build subtitles failed video_id=%d err=%v", videoID, err)
		writeContextVideoError(c, http.StatusInternalServerError, "get context video subtitles failed")
		return
	}
	if contextVideoNotModified(c, row.DedupeKey) {
		return
	}
	c.Data(http.StatusOK, "text/vtt; charset=utf-8", []byte(vtt))
}

func contextVideoNotModified(c *app.RequestContext, version string) bool {
	etag := `"` + version + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	if string(c.Request.Header.Peek("If-None-Match")) == etag {
		c.Status(http.StatusNotModified)
		return true
	}
	return false
}

func parsePositiveInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, service.ErrContextVideoInvalid
	}
	return value, nil
}

func parsePositiveOrZeroInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, service.ErrContextVideoInvalid
	}
	return value, nil
}

func contextVideoError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrContextVideoInvalid):
		return http.StatusBadRequest, "invalid context video"
	case errors.Is(err, service.ErrContextItemNotFound),
		errors.Is(err, service.ErrContextVideoNotFound),
		errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound, "context video not found"
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "process context video failed"
	}
}

func writeContextVideoError(c *app.RequestContext, status int, message string) {
	c.JSON(status, model.ContextVideoResponse{Code: status, Msg: message})
}
