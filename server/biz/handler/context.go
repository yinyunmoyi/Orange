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

// CreateContextTask 创建并同步执行语境任务。
func CreateContextTask(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	remote := c.ClientIP()
	var req model.ContextTaskRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[context-task] bind failed remote=%s raw_body=%q err=%v elapsed=%s",
			remote, logSnippet(string(c.Request.Body()), 500), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.ContextTaskResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	normalized, err := service.NormalizeContextTaskRequest(&req)
	if err != nil {
		log.Printf("[context-task] validate failed remote=%s item_type=%q item_id=%d paragraph=%q selection_start=%d selection_end=%d err=%v elapsed=%s",
			remote, req.ItemType, req.ItemID, logSnippet(req.Paragraph, 200),
			req.SelectionStart, req.SelectionEnd, err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.ContextTaskResponse{Code: http.StatusBadRequest, Msg: err.Error()})
		return
	}

	// 任务已经由本次同步请求启动；即使客户端提前断开，也要尽力写入最终任务状态。
	data, err := service.ProcessContextTask(context.Background(), normalized)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrDBDisabled):
			log.Printf("[context-task] db disabled remote=%s item_type=%q item_id=%d elapsed=%s",
				remote, normalized.ItemType, normalized.ItemID, time.Since(started))
			c.JSON(http.StatusServiceUnavailable, model.ContextTaskResponse{
				Code: http.StatusServiceUnavailable, Msg: "database not configured", Data: data,
			})
		case errors.Is(err, service.ErrContextItemNotFound):
			log.Printf("[context-task] item not found remote=%s item_type=%q item_id=%d elapsed=%s",
				remote, normalized.ItemType, normalized.ItemID, time.Since(started))
			c.JSON(http.StatusNotFound, model.ContextTaskResponse{
				Code: http.StatusNotFound, Msg: err.Error(), Data: data,
			})
		case errors.Is(err, service.ErrContextItemMismatch):
			log.Printf("[context-task] item mismatch remote=%s item_type=%q item_id=%d err=%v elapsed=%s",
				remote, normalized.ItemType, normalized.ItemID, err, time.Since(started))
			c.JSON(http.StatusBadRequest, model.ContextTaskResponse{
				Code: http.StatusBadRequest, Msg: err.Error(), Data: data,
			})
		case errors.Is(err, service.ErrContextTaskConflict), errors.Is(err, service.ErrContextTaskFailed):
			log.Printf("[context-task] conflict remote=%s item_type=%q item_id=%d task=%+v err=%v elapsed=%s",
				remote, normalized.ItemType, normalized.ItemID, data, err, time.Since(started))
			c.JSON(http.StatusConflict, model.ContextTaskResponse{
				Code: http.StatusConflict, Msg: err.Error(), Data: data,
			})
		default:
			log.Printf("[context-task] process failed remote=%s item_type=%q item_id=%d task=%+v err=%v elapsed=%s",
				remote, normalized.ItemType, normalized.ItemID, data, err, time.Since(started))
			c.JSON(http.StatusInternalServerError, model.ContextTaskResponse{
				Code: http.StatusInternalServerError, Msg: "process context failed", Data: data,
			})
		}
		return
	}

	log.Printf("[context-task] ok remote=%s item_type=%q item_id=%d task_id=%d context_id=%v elapsed=%s",
		remote, normalized.ItemType, normalized.ItemID, data.TaskID, data.ContextID, time.Since(started))
	c.JSON(http.StatusOK, model.ContextTaskResponse{Code: 0, Data: data})
}

// ContextAudio 返回已生成的语境句子音频。
func ContextAudio(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	rawID := c.Param("id")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		log.Printf("[context-audio] invalid id raw=%q remote=%s err=%v", rawID, c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, model.ContextTaskResponse{Code: http.StatusBadRequest, Msg: "invalid context id"})
		return
	}

	path, contentType, err := service.LookupContextAudio(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, os.ErrNotExist):
			log.Printf("[context-audio] not found context_id=%d remote=%s elapsed=%s",
				id, c.ClientIP(), time.Since(started))
			c.JSON(http.StatusNotFound, model.ContextTaskResponse{Code: http.StatusNotFound, Msg: "context audio not found"})
		case errors.Is(err, db.ErrDBDisabled):
			log.Printf("[context-audio] db disabled context_id=%d elapsed=%s", id, time.Since(started))
			c.JSON(http.StatusServiceUnavailable, model.ContextTaskResponse{Code: http.StatusServiceUnavailable, Msg: "database not configured"})
		default:
			log.Printf("[context-audio] lookup failed context_id=%d remote=%s err=%v elapsed=%s",
				id, c.ClientIP(), err, time.Since(started))
			c.JSON(http.StatusInternalServerError, model.ContextTaskResponse{Code: http.StatusInternalServerError, Msg: "get context audio failed"})
		}
		return
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			log.Printf("[context-audio] file missing context_id=%d path=%q elapsed=%s", id, path, time.Since(started))
			c.JSON(http.StatusNotFound, model.ContextTaskResponse{Code: http.StatusNotFound, Msg: "context audio not found"})
			return
		}
		log.Printf("[context-audio] stat failed context_id=%d path=%q err=%v elapsed=%s",
			id, path, err, time.Since(started))
		c.JSON(http.StatusInternalServerError, model.ContextTaskResponse{Code: http.StatusInternalServerError, Msg: "get context audio failed"})
		return
	}

	log.Printf("[context-audio] ok context_id=%d path=%q content_type=%q remote=%s elapsed=%s",
		id, path, contentType, c.ClientIP(), time.Since(started))
	c.SetContentType(contentType)
	c.File(path)
}
