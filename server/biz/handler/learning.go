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

func CreateLearningSession(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	data, err := service.CreateLearningSession(ctx)
	if err != nil {
		log.Printf("[learning-session] create failed remote=%s err=%v elapsed=%s", c.ClientIP(), err, time.Since(start))
		writeLearningError(c, err)
		return
	}
	log.Printf("[learning-session] created session=%d today=%d status=%s elapsed=%s",
		data.SessionID, data.TodayTotal, data.SessionStatus, time.Since(start))
	c.JSON(http.StatusOK, model.LearningPlanResponse{Code: 0, Data: data})
}

func CurrentLearningCard(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	sessionID, err := parseLearningSessionID(c)
	if err != nil {
		log.Printf("[learning-current] invalid session raw=%q remote=%s err=%v", c.Param("id"), c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, model.LearningCurrentResponse{Code: http.StatusBadRequest, Msg: "invalid session id"})
		return
	}
	data, err := service.GetCurrentLearningCard(ctx, sessionID)
	if err != nil {
		log.Printf("[learning-current] failed session=%d remote=%s err=%v elapsed=%s",
			sessionID, c.ClientIP(), err, time.Since(start))
		writeLearningError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.LearningCurrentResponse{Code: 0, Data: data})
}

func SubmitLearningAnswer(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	traceID := string(c.Request.Header.Peek("X-Orange-Debug-Trace-Id"))
	sessionID, err := parseLearningSessionID(c)
	if err != nil {
		log.Printf("[learning-answer] invalid session raw=%q remote=%s err=%v", c.Param("id"), c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, model.LearningCurrentResponse{Code: http.StatusBadRequest, Msg: "invalid session id"})
		return
	}
	var req model.LearningAnswerRequest
	if bindErr := c.BindJSON(&req); bindErr != nil {
		log.Printf("[learning-answer] bind failed session=%d raw_body=%q remote=%s err=%v",
			sessionID, logSnippet(string(c.Request.Body()), 500), c.ClientIP(), bindErr)
		c.JSON(http.StatusBadRequest, model.LearningCurrentResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	log.Printf("[learning-trace] trace=%q phase=answer_received session=%d queue=%d turn=%d answer=%q remote=%s",
		traceID, sessionID, req.QueueItemID, req.TurnNo, req.Answer, c.ClientIP())
	ctx = service.WithLearningTrace(ctx, traceID)
	data, err := service.SubmitLearningAnswer(ctx, sessionID, &req)
	if err != nil {
		log.Printf("[learning-answer] failed session=%d queue=%d turn=%d answer=%q remote=%s err=%v elapsed=%s",
			sessionID, req.QueueItemID, req.TurnNo, req.Answer, c.ClientIP(), err, time.Since(start))
		writeLearningError(c, err)
		return
	}
	log.Printf("[learning-answer] ok session=%d queue=%d turn=%d answer=%q completed=%t remaining=%d elapsed=%s",
		sessionID, req.QueueItemID, req.TurnNo, req.Answer, data.Completed, data.Remaining, time.Since(start))
	c.JSON(http.StatusOK, model.LearningCurrentResponse{Code: 0, Data: data})
	log.Printf("[learning-trace] trace=%q phase=answer_response_completed session=%d next_queue=%d total_ms=%d",
		traceID, sessionID, data.QueueItemID, time.Since(start).Milliseconds())
}

func MasterCurrentLearningItem(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	traceID := string(c.Request.Header.Peek("X-Orange-Debug-Trace-Id"))
	sessionID, err := parseLearningSessionID(c)
	if err != nil {
		log.Printf("[learning-mastery] invalid session raw=%q remote=%s err=%v", c.Param("id"), c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, model.LearningCurrentResponse{Code: http.StatusBadRequest, Msg: "invalid session id"})
		return
	}
	var req model.LearningMasteryRequest
	if bindErr := c.BindJSON(&req); bindErr != nil {
		log.Printf("[learning-mastery] bind failed session=%d raw_body=%q remote=%s err=%v",
			sessionID, logSnippet(string(c.Request.Body()), 500), c.ClientIP(), bindErr)
		c.JSON(http.StatusBadRequest, model.LearningCurrentResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	log.Printf("[learning-trace] trace=%q phase=mastery_received session=%d queue=%d turn=%d remote=%s",
		traceID, sessionID, req.QueueItemID, req.TurnNo, c.ClientIP())
	ctx = service.WithLearningTrace(ctx, traceID)
	data, err := service.MasterCurrentLearningItem(ctx, sessionID, &req)
	if err != nil {
		log.Printf("[learning-mastery] failed session=%d queue=%d turn=%d remote=%s err=%v elapsed=%s",
			sessionID, req.QueueItemID, req.TurnNo, c.ClientIP(), err, time.Since(start))
		writeLearningError(c, err)
		return
	}
	log.Printf("[learning-mastery] ok session=%d queue=%d turn=%d next_queue=%d completed=%t remaining=%d elapsed=%s",
		sessionID, req.QueueItemID, req.TurnNo, data.QueueItemID, data.Completed, data.Remaining, time.Since(start))
	c.JSON(http.StatusOK, model.LearningCurrentResponse{Code: 0, Data: data})
	log.Printf("[learning-trace] trace=%q phase=mastery_response_completed session=%d next_queue=%d total_ms=%d",
		traceID, sessionID, data.QueueItemID, time.Since(start).Milliseconds())
}

func ExtendLearningSession(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	sessionID, err := parseLearningSessionID(c)
	if err != nil {
		log.Printf("[learning-extend] invalid session raw=%q remote=%s err=%v", c.Param("id"), c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, model.LearningPlanResponse{Code: http.StatusBadRequest, Msg: "invalid session id"})
		return
	}
	data, err := service.ExtendLearningSession(ctx, sessionID)
	if err != nil {
		log.Printf("[learning-extend] failed session=%d remote=%s err=%v elapsed=%s",
			sessionID, c.ClientIP(), err, time.Since(start))
		writeLearningError(c, err)
		return
	}
	log.Printf("[learning-extend] ok session=%d today=%d status=%s elapsed=%s",
		data.SessionID, data.TodayTotal, data.SessionStatus, time.Since(start))
	c.JSON(http.StatusOK, model.LearningPlanResponse{Code: 0, Data: data})
}

func parseLearningSessionID(c *app.RequestContext) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, service.ErrLearningInvalid
	}
	return id, nil
}

func writeLearningError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, service.ErrLearningInvalid):
		c.JSON(http.StatusBadRequest, model.LearningCurrentResponse{Code: http.StatusBadRequest, Msg: "invalid learning request"})
	case errors.Is(err, service.ErrLearningNotFound):
		c.JSON(http.StatusNotFound, model.LearningCurrentResponse{Code: http.StatusNotFound, Msg: "learning session not found"})
	case errors.Is(err, service.ErrLearningConflict):
		c.JSON(http.StatusConflict, model.LearningCurrentResponse{Code: http.StatusConflict, Msg: "learning session changed"})
	case errors.Is(err, db.ErrDBDisabled):
		c.JSON(http.StatusServiceUnavailable, model.LearningCurrentResponse{Code: http.StatusServiceUnavailable, Msg: "database not configured"})
	default:
		c.JSON(http.StatusInternalServerError, model.LearningCurrentResponse{Code: http.StatusInternalServerError, Msg: "learning operation failed"})
	}
}
