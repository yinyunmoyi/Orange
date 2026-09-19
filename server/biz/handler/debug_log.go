package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aaa_word/biz/config"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

var (
	debugLogMu sync.Mutex
)

const (
	maxClientLogBodyBytes = 512 * 1024
	maxClientLogFileBytes = 20 * 1024 * 1024
	clientLogBackups      = 4
	maxBatchEvents        = 100
)

// DebugLog 是给前端调试用的落地日志接口：POST 一段 JSON 上来，追加写到 ./data/debug/frontend.log。
func DebugLog(_ context.Context, ctx *app.RequestContext) {
	writeDebugLog(ctx, config.WebLogPath(), "web")
}

// AndroidDebugLog 单独落盘 Android 客户端日志，避免与 Web 日志混写。
func AndroidDebugLog(_ context.Context, ctx *app.RequestContext) {
	writeDebugLog(ctx, config.AndroidLogPath(), "android")
}

func writeDebugLog(ctx *app.RequestContext, logPath, source string) {
	body := ctx.Request.Body()
	if len(body) > maxClientLogBodyBytes {
		ctx.JSON(http.StatusRequestEntityTooLarge, utils.H{"code": -1, "msg": "log payload too large"})
		return
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		payload = map[string]any{"raw": string(body)}
	}
	payloads := unpackLogPayloads(payload)

	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o755); mkErr != nil {
		ctx.JSON(500, utils.H{"code": -1, "msg": mkErr.Error()})
		return
	}
	lines := make([][]byte, 0, len(payloads))
	incomingBytes := 0
	for _, item := range payloads {
		entry := map[string]any{
			"receivedAt": time.Now().Format("2006-01-02 15:04:05.000"),
			"source":     source,
			"remote":     ctx.ClientIP(),
			"payload":    item,
		}
		line, _ := json.Marshal(entry)
		lines = append(lines, line)
		incomingBytes += len(line) + 1
	}
	if rotateErr := rotateClientLog(logPath, int64(incomingBytes)); rotateErr != nil {
		ctx.JSON(500, utils.H{"code": -1, "msg": rotateErr.Error()})
		return
	}
	f, openErr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		ctx.JSON(500, utils.H{"code": -1, "msg": openErr.Error()})
		return
	}
	defer f.Close()
	for _, line := range lines {
		if _, writeErr := fmt.Fprintln(f, string(line)); writeErr != nil {
			ctx.JSON(500, utils.H{"code": -1, "msg": writeErr.Error()})
			return
		}
	}
	ctx.JSON(200, utils.H{"code": 0, "accepted": len(lines)})
}

func unpackLogPayloads(payload any) []any {
	root, ok := payload.(map[string]any)
	if !ok {
		return []any{payload}
	}
	events, ok := root["events"].([]any)
	if !ok {
		return []any{payload}
	}
	if len(events) > maxBatchEvents {
		events = events[:maxBatchEvents]
	}
	return events
}

func rotateClientLog(path string, incomingBytes int64) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size()+incomingBytes <= maxClientLogFileBytes {
		return nil
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, clientLogBackups))
	for index := clientLogBackups - 1; index >= 1; index-- {
		older := fmt.Sprintf("%s.%d", path, index)
		newer := fmt.Sprintf("%s.%d", path, index+1)
		if renameErr := os.Rename(older, newer); renameErr != nil && !errors.Is(renameErr, os.ErrNotExist) {
			return renameErr
		}
	}
	return os.Rename(path, path+".1")
}
