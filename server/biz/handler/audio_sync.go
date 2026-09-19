package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"
	"aaa_word/biz/syncidentity"

	"github.com/cloudwego/hertz/pkg/app"
)

func AudioSyncManifest(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	serverID, err := syncidentity.Current()
	if err != nil {
		writeAudioSyncError(c, http.StatusInternalServerError, "audio sync unavailable")
		return
	}
	data, err := service.GetAudioSyncManifest(ctx, serverID, time.Now())
	if err != nil {
		status, message := audioSyncError(err)
		log.Printf("[audio-sync] manifest failed remote=%s status=%d err=%v elapsed=%s",
			c.ClientIP(), status, err, time.Since(started))
		writeAudioSyncError(c, status, message)
		return
	}
	log.Printf("[audio-sync] manifest ok remote=%s count=%d elapsed=%s",
		c.ClientIP(), len(data.Items), time.Since(started))
	c.JSON(http.StatusOK, model.AudioSyncResponse{Code: 0, Data: data})
}

func AudioSyncFile(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	itemType := strings.ToLower(strings.TrimSpace(c.Param("itemType")))
	audioKey, err := strconv.ParseInt(c.Param("audioKey"), 10, 64)
	version := strings.TrimSpace(c.Query("v"))
	if err != nil || audioKey <= 0 || version == "" {
		writeAudioSyncError(c, http.StatusBadRequest, "invalid audio sync request")
		return
	}
	path, contentType, currentVersion, err := service.LookupAudioSyncFile(
		ctx,
		itemType,
		audioKey,
		version,
	)
	if err != nil {
		status, message := audioSyncError(err)
		writeAudioSyncError(c, status, message)
		return
	}
	if audioSyncNotModified(c, currentVersion) {
		return
	}
	stat, err := os.Stat(path)
	if err != nil {
		writeAudioSyncError(c, http.StatusNotFound, "audio sync item not found")
		return
	}
	c.Header("Content-Length", strconv.FormatInt(stat.Size(), 10))
	c.SetContentType(contentType)
	log.Printf("[audio-sync] file ok type=%s key=%d bytes=%d remote=%s elapsed=%s",
		itemType, audioKey, stat.Size(), c.ClientIP(), time.Since(started))
	c.File(path)
}

func audioSyncNotModified(c *app.RequestContext, version string) bool {
	etag := `"` + version + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	if string(c.Request.Header.Peek("If-None-Match")) == etag {
		c.Status(http.StatusNotModified)
		return true
	}
	return false
}

func audioSyncError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrAudioSyncInvalid):
		return http.StatusBadRequest, "invalid audio sync request"
	case errors.Is(err, service.ErrAudioSyncNotFound), errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound, "audio sync item not found"
	case errors.Is(err, db.ErrDBDisabled):
		return http.StatusServiceUnavailable, "database not configured"
	default:
		return http.StatusInternalServerError, "audio sync failed"
	}
}

func writeAudioSyncError(c *app.RequestContext, status int, message string) {
	c.JSON(status, model.AudioSyncResponse{Code: status, Msg: message})
}
