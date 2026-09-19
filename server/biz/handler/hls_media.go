package handler

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aaa_word/biz/service"
	"aaa_word/biz/storage"

	"github.com/cloudwego/hertz/pkg/app"
)

var (
	hlsFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{16,128}$`)
	hlsSegmentPattern     = regexp.MustCompile(`^(seg[0-9]+\.(ts|m4s)|init\.mp4)$`)
)

func hlsMediaError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrHLSMediaInvalid):
		return http.StatusBadRequest, "invalid hls media"
	case errors.Is(err, service.ErrHLSMediaNotFound), errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound, "hls media not found"
	default:
		return http.StatusInternalServerError, "process hls media failed"
	}
}

func writeHLSError(c *app.RequestContext, status int, message string) {
	c.JSON(status, map[string]any{"code": status, "msg": message})
}

// ImportHLSMedia 上传原始视频文件并触发 HLS 转封装。
// 请求：POST /api/v1/media/hls/import?fingerprint=<fp>，Body 为二进制视频数据（application/octet-stream）。
func ImportHLSMedia(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	fingerprint := string(c.Query("fingerprint"))
	if !hlsFingerprintPattern.MatchString(fingerprint) {
		writeHLSError(c, http.StatusBadRequest, "invalid fingerprint")
		return
	}

	body := c.RequestBodyStream()
	if body == nil {
		writeHLSError(c, http.StatusBadRequest, "empty body")
		return
	}

	durationSeconds, _ := strconv.ParseFloat(string(c.Query("duration")), 64)
	if durationSeconds < 0 || math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) {
		durationSeconds = 0
	}
	result, err := service.ImportHLSMediaWithDuration(ctx, fingerprint, body, durationSeconds)
	if err != nil {
		status, message := hlsMediaError(err)
		log.Printf("[hls-media] import failed fp=%s remote=%s err=%v elapsed=%s",
			fingerprint, c.ClientIP(), err, time.Since(started))
		writeHLSError(c, status, message)
		return
	}
	log.Printf("[hls-media] import ok fp=%s cached=%t size=%d remote=%s elapsed=%s",
		fingerprint, result.CachedHit, result.SourceSize, c.ClientIP(), time.Since(started))
	c.JSON(http.StatusOK, map[string]any{
		"code": 0,
		"data": result,
	})
}

// HLSMediaStatus 查询指定 fingerprint 是否已完成切片。
func HLSMediaStatus(_ context.Context, c *app.RequestContext) {
	fingerprint := c.Param("fingerprint")
	if !hlsFingerprintPattern.MatchString(fingerprint) {
		writeHLSError(c, http.StatusBadRequest, "invalid fingerprint")
		return
	}
	status, err := service.LookupHLSMediaStatus(fingerprint)
	if err != nil {
		httpStatus, message := hlsMediaError(err)
		writeHLSError(c, httpStatus, message)
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"code": 0,
		"data": status,
	})
}

// HLSMediaPlaylist 返回 master.m3u8。
func HLSMediaPlaylist(_ context.Context, c *app.RequestContext) {
	fingerprint := c.Param("fingerprint")
	if !hlsFingerprintPattern.MatchString(fingerprint) {
		writeHLSError(c, http.StatusBadRequest, "invalid fingerprint")
		return
	}
	path, err := storage.HLSPlaylistPath(fingerprint)
	if err != nil {
		writeHLSError(c, http.StatusBadRequest, "invalid fingerprint")
		return
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeHLSError(c, http.StatusNotFound, "hls media not found")
			return
		}
		writeHLSError(c, http.StatusInternalServerError, "read playlist failed")
		return
	}
	_ = storage.TouchHLSMedia(fingerprint)
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-cache")
	c.File(path)
}

// HLSMediaSegment 返回单个 .ts 分片文件。
func HLSMediaSegment(_ context.Context, c *app.RequestContext) {
	fingerprint := c.Param("fingerprint")
	segment := c.Param("segment")
	if !hlsFingerprintPattern.MatchString(fingerprint) || !hlsSegmentPattern.MatchString(segment) {
		writeHLSError(c, http.StatusBadRequest, "invalid segment")
		return
	}
	path, err := storage.HLSSegmentPath(fingerprint, segment)
	if err != nil {
		writeHLSError(c, http.StatusBadRequest, "invalid segment")
		return
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeHLSError(c, http.StatusNotFound, "hls segment not found")
			return
		}
		writeHLSError(c, http.StatusInternalServerError, "read segment failed")
		return
	}
	c.Header("Content-Type", hlsSegmentContentType(segment))
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.File(path)
}

func hlsSegmentContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".m4s"), name == "init.mp4":
		return "video/mp4"
	default:
		return "video/mp2t"
	}
}
