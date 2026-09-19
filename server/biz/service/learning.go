package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/converter"
	"aaa_word/biz/db"
	"aaa_word/biz/dict"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	LearningItemWord   = 1
	LearningItemPhrase = 2

	sessionActive     = "active"
	sessionCompleted  = "completed"
	sessionSuperseded = "superseded"

	// LearningExtraNewBatchSize 是"继续学习"每次追加的新词条数。
	// 需求固定 10 条，先常量化，待未来需可配再抽 config。
	LearningExtraNewBatchSize = 10
)

var (
	ErrLearningNotFound = errors.New("learning session not found")
	ErrLearningConflict = errors.New("learning session conflict")
	ErrLearningInvalid  = errors.New("invalid learning request")
)

type learningTraceKey struct{}

func WithLearningTrace(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, learningTraceKey{}, traceID)
}

func learningTrace(ctx context.Context) string {
	traceID, _ := ctx.Value(learningTraceKey{}).(string)
	return traceID
}

type learningItemKey struct {
	ItemType int
	ItemID   int64
}

func learningKey(itemType int, itemID int64) learningItemKey {
	return learningItemKey{ItemType: itemType, ItemID: itemID}
}

func supportedLearningItem(itemType int) bool {
	return itemType == LearningItemWord || itemType == LearningItemPhrase
}

func learningLocation() *time.Location {
	name := config.Get("LEARNING_TIMEZONE", "Asia/Shanghai")
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("[learning] load timezone failed name=%q err=%v, fallback=Asia/Shanghai", name, err)
		loc, _ = time.LoadLocation("Asia/Shanghai")
	}
	return loc
}

func CreateLearningSession(ctx context.Context) (*model.LearningPlanData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	now := time.Now()
	loc := learningLocation()
	local := now.In(loc)
	studyDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	nextDate := studyDate.AddDate(0, 0, 1)
	var session db.LearningSession
	var queues []db.UserLearningQueue

	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings, err := loadEffectiveLearningSettings(tx)
		if err != nil {
			return err
		}
		var oldSessions []db.LearningSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("study_date >= ? AND study_date < ?", studyDate, nextDate).
			Find(&oldSessions).Error; err != nil {
			log.Printf("[learning-session] load day sessions failed date=%s err=%v", studyDate.Format("2006-01-02"), err)
			return err
		}
		oldIDs := make([]int64, 0, len(oldSessions))
		dayWasCompleted := false
		for _, old := range oldSessions {
			oldIDs = append(oldIDs, old.ID)
			if old.Status == sessionCompleted {
				dayWasCompleted = true
			}
		}
		var oldQueues []db.UserLearningQueue
		if len(oldIDs) > 0 {
			if err := tx.Where("session_id IN ?", oldIDs).Find(&oldQueues).Error; err != nil {
				log.Printf("[learning-session] load old queues failed sessions=%v err=%v", oldIDs, err)
				return err
			}
			if err := tx.Model(&db.LearningSession{}).
				Where("id IN ? AND status = ?", oldIDs, sessionActive).
				Update("status", sessionSuperseded).Error; err != nil {
				log.Printf("[learning-session] supersede failed sessions=%v err=%v", oldIDs, err)
				return err
			}
		}

		session = db.LearningSession{StudyDate: studyDate, Status: sessionActive}
		if err := tx.Create(&session).Error; err != nil {
			log.Printf("[learning-session] create failed date=%s err=%v", studyDate.Format("2006-01-02"), err)
			return err
		}

		latestItems := make(map[learningItemKey]db.UserLearningQueue)
		startedNew := make(map[learningItemKey]struct{})
		startedReviews := make(map[learningItemKey]struct{})
		for _, q := range oldQueues {
			if !supportedLearningItem(q.ItemType) {
				continue
			}
			key := learningKey(q.ItemType, q.ItemID)
			if q.SourceQueueType == learningrule.QueueNew &&
				(q.AnswerCount > 0 || q.Status == learningrule.QueueCompleted) {
				// 一键掌握路径不会累加 AnswerCount，但会把 queue 置为 Completed；
				// 两种情况都视为当日已消耗新词名额，避免重复分配。
				startedNew[key] = struct{}{}
			}
			if q.SourceQueueType == learningrule.QueueReview &&
				(q.AnswerCount > 0 || q.Status == learningrule.QueueCompleted) {
				startedReviews[key] = struct{}{}
			}
			if previous, ok := latestItems[key]; !ok || q.SessionID > previous.SessionID {
				latestItems[key] = q
			}
		}
		retryItems := make(map[learningItemKey]db.UserLearningQueue)
		for key, q := range latestItems {
			if q.Status == learningrule.QueueRetryWaiting {
				retryItems[key] = q
			}
		}

		added := make(map[learningItemKey]struct{})
		seq := float64(1)
		retryKeys := make([]learningItemKey, 0, len(retryItems))
		for key := range retryItems {
			retryKeys = append(retryKeys, key)
		}
		sort.Slice(retryKeys, func(i, j int) bool {
			if retryKeys[i].ItemType != retryKeys[j].ItemType {
				return retryKeys[i].ItemType < retryKeys[j].ItemType
			}
			return retryKeys[i].ItemID < retryKeys[j].ItemID
		})
		for _, key := range retryKeys {
			q := retryItems[key]
			retryTurn := 8
			queues = append(queues, db.UserLearningQueue{
				SessionID: session.ID, ItemType: q.ItemType, ItemID: q.ItemID,
				QueueType: learningrule.QueueRetry, SourceQueueType: q.SourceQueueType,
				QueueSeq: seq, Status: learningrule.QueueRetryWaiting,
				HadIncorrect: q.HadIncorrect, IncorrectCount: q.IncorrectCount,
				AnswerCount: q.AnswerCount, KnownCount: q.KnownCount, RetryAfterTurn: &retryTurn,
			})
			added[key] = struct{}{}
			seq++
		}

		var due []db.UserItemLearning
		if err := tx.Where(
			"item_type IN ? AND learning_status = ? AND next_review_at IS NOT NULL AND next_review_at < ?",
			[]int{LearningItemWord, LearningItemPhrase}, learningrule.StatusLearning, nextDate,
		).Order("next_review_at, id").Find(&due).Error; err != nil {
			log.Printf("[learning-session] load due reviews failed date=%s err=%v", studyDate.Format("2006-01-02"), err)
			return err
		}

		var notStarted []db.UserItemLearning
		if err := tx.Where("item_type IN ? AND learning_status = ?",
			[]int{LearningItemWord, LearningItemPhrase}, learningrule.StatusNotStarted).
			Order("queued_at DESC, id DESC").Find(&notStarted).Error; err != nil {
			log.Printf("[learning-session] load new candidates failed err=%v", err)
			return err
		}

		freshCandidates := make([]db.UserItemLearning, 0, len(notStarted))
		for _, row := range notStarted {
			key := learningKey(row.ItemType, row.ItemID)
			if _, started := startedNew[key]; started {
				continue
			}
			if _, alreadyAdded := added[key]; alreadyAdded {
				continue
			}
			freshCandidates = append(freshCandidates, row)
		}
		remainingSlots := settings.DailyNewLimit - len(startedNew)
		if dayWasCompleted {
			// Reopening a completed day must not refill slots released by deleting
			// an item that was already learned. Extra words remain opt-in via Extend.
			remainingSlots = 0
		}
		if remainingSlots < 0 {
			remainingSlots = 0
		}
		newRows := selectDailyNewRows(freshCandidates, remainingSlots)

		for _, row := range newRows {
			key := learningKey(row.ItemType, row.ItemID)
			if _, exists := added[key]; exists {
				continue
			}
			queues = append(queues, db.UserLearningQueue{
				SessionID: session.ID, ItemType: row.ItemType, ItemID: row.ItemID,
				QueueType: learningrule.QueueNew, SourceQueueType: learningrule.QueueNew,
				QueueSeq: 1000 + seq, Status: learningrule.QueuePending,
			})
			added[key] = struct{}{}
			seq++
		}
		reviewCandidates := make([]db.UserItemLearning, 0, len(due))
		for _, row := range due {
			key := learningKey(row.ItemType, row.ItemID)
			if _, exists := added[key]; exists {
				continue
			}
			reviewCandidates = append(reviewCandidates, row)
		}
		remainingReviewSlots := remainingDailyReviewSlots(
			settings.DailyReviewLimit,
			len(startedReviews),
		)
		for _, row := range selectDailyReviewRows(reviewCandidates, remainingReviewSlots) {
			key := learningKey(row.ItemType, row.ItemID)
			queues = append(queues, db.UserLearningQueue{
				SessionID: session.ID, ItemType: row.ItemType, ItemID: row.ItemID,
				QueueType: learningrule.QueueReview, SourceQueueType: learningrule.QueueReview,
				QueueSeq: 2000 + seq, Status: learningrule.QueuePending,
			})
			added[key] = struct{}{}
			seq++
		}
		if len(queues) > 0 {
			if err := tx.Create(&queues).Error; err != nil {
				log.Printf("[learning-session] create queue failed session=%d count=%d err=%v", session.ID, len(queues), err)
				return err
			}
		}
		next := pickQueue(queues, 1)
		if next == nil {
			session.Status = sessionCompleted
			session.CompletedAt = &now
			if err := tx.Model(&db.LearningSession{}).Where("id = ?", session.ID).
				Updates(map[string]any{"status": sessionCompleted, "completed_at": now}).Error; err != nil {
				return err
			}
		} else {
			session.CurrentQueueItemID = &next.ID
			if err := tx.Model(&db.LearningSession{}).Where("id = ?", session.ID).
				Update("current_queue_item_id", next.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create learning session: %w", err)
	}
	return buildLearningPlan(ctx, &session, queues)
}

func selectDailyNewRows(rows []db.UserItemLearning, limit int) []db.UserItemLearning {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	return append([]db.UserItemLearning(nil), rows[:limit]...)
}

func selectDailyReviewRows(rows []db.UserItemLearning, limit int) []db.UserItemLearning {
	if len(rows) == 0 || limit <= 0 {
		return nil
	}
	if limit == LearningReviewUnlimited || limit > len(rows) {
		limit = len(rows)
	}
	return append([]db.UserItemLearning(nil), rows[:limit]...)
}

func remainingDailyReviewSlots(limit, consumed int) int {
	if limit == LearningReviewUnlimited {
		return LearningReviewUnlimited
	}
	if remaining := limit - consumed; remaining > 0 {
		return remaining
	}
	return 0
}

// ExtendLearningSession 让当日已完成的 session 追加一批新词，允许用户"继续学习"。
// 会在事务内加载 session、复用已有 queue 去重、按 queued_at DESC 取新词候选，
// 追加 LearningExtraNewBatchSize 条 QueueNew 行并将 session 状态重置为 active。
func ExtendLearningSession(ctx context.Context, sessionID int64) (*model.LearningPlanData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	if sessionID <= 0 {
		return nil, ErrLearningInvalid
	}
	now := time.Now()
	loc := learningLocation()
	local := now.In(loc)
	studyDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	nextDate := studyDate.AddDate(0, 0, 1)

	var session db.LearningSession
	var allQueues []db.UserLearningQueue

	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", sessionID).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("[learning-extend] session missing session=%d err=%v", sessionID, err)
				return ErrLearningNotFound
			}
			log.Printf("[learning-extend] load session failed session=%d err=%v", sessionID, err)
			return err
		}
		// 只允许当日的 completed session 继续学习。
		if session.StudyDate.Before(studyDate) || !session.StudyDate.Before(nextDate) {
			log.Printf("[learning-extend] session not today session=%d study_date=%s today=%s",
				sessionID, session.StudyDate.Format(time.RFC3339), studyDate.Format(time.RFC3339))
			return ErrLearningConflict
		}
		if session.Status != sessionCompleted || session.CurrentQueueItemID != nil {
			log.Printf("[learning-extend] session not completed session=%d status=%s current=%v",
				sessionID, session.Status, session.CurrentQueueItemID)
			return ErrLearningConflict
		}

		var existing []db.UserLearningQueue
		if err := tx.Where("session_id = ?", sessionID).Find(&existing).Error; err != nil {
			log.Printf("[learning-extend] load existing queue failed session=%d err=%v", sessionID, err)
			return err
		}
		existed := make(map[learningItemKey]struct{}, len(existing))
		baseSeq := float64(0)
		for _, q := range existing {
			existed[learningKey(q.ItemType, q.ItemID)] = struct{}{}
			if q.QueueSeq > baseSeq {
				baseSeq = q.QueueSeq
			}
		}

		var notStarted []db.UserItemLearning
		if err := tx.Where("item_type IN ? AND learning_status = ?",
			[]int{LearningItemWord, LearningItemPhrase}, learningrule.StatusNotStarted).
			Order("queued_at DESC, id DESC").Find(&notStarted).Error; err != nil {
			log.Printf("[learning-extend] load new candidates failed session=%d err=%v", sessionID, err)
			return err
		}
		freshCandidates := make([]db.UserItemLearning, 0, len(notStarted))
		for _, row := range notStarted {
			if _, ok := existed[learningKey(row.ItemType, row.ItemID)]; ok {
				continue
			}
			freshCandidates = append(freshCandidates, row)
		}
		selected := selectDailyNewRows(freshCandidates, LearningExtraNewBatchSize)
		if len(selected) == 0 {
			log.Printf("[learning-extend] no more candidates session=%d not_started=%d", sessionID, len(notStarted))
			return ErrLearningConflict
		}

		newQueues := make([]db.UserLearningQueue, 0, len(selected))
		seq := baseSeq
		for _, row := range selected {
			seq++
			newQueues = append(newQueues, db.UserLearningQueue{
				SessionID: sessionID, ItemType: row.ItemType, ItemID: row.ItemID,
				QueueType: learningrule.QueueNew, SourceQueueType: learningrule.QueueNew,
				QueueSeq: 1000 + seq, Status: learningrule.QueuePending,
			})
		}
		if err := tx.Create(&newQueues).Error; err != nil {
			log.Printf("[learning-extend] insert new queue failed session=%d count=%d err=%v",
				sessionID, len(newQueues), err)
			return err
		}

		next := pickQueue(newQueues, session.TurnNo+1)
		if next == nil {
			log.Printf("[learning-extend] pick next failed session=%d count=%d", sessionID, len(newQueues))
			return ErrLearningConflict
		}
		updates := map[string]any{
			"status":                sessionActive,
			"completed_at":          nil,
			"current_queue_item_id": next.ID,
		}
		if err := tx.Model(&db.LearningSession{}).Where("id = ?", sessionID).
			Updates(updates).Error; err != nil {
			log.Printf("[learning-extend] update session failed session=%d err=%v", sessionID, err)
			return err
		}
		session.Status = sessionActive
		session.CompletedAt = nil
		session.CurrentQueueItemID = &next.ID

		allQueues = append(allQueues, existing...)
		allQueues = append(allQueues, newQueues...)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrLearningNotFound) || errors.Is(err, ErrLearningConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("extend learning session: %w", err)
	}
	return buildLearningPlan(ctx, &session, allQueues)
}

func buildLearningPlan(ctx context.Context, session *db.LearningSession, queues []db.UserLearningQueue) (*model.LearningPlanData, error) {
	data := &model.LearningPlanData{
		SessionID: session.ID, SessionStatus: session.Status,
		TodayTotal: len(queues), HasCurrentCard: session.CurrentQueueItemID != nil,
	}
	for _, q := range queues {
		switch q.QueueType {
		case learningrule.QueueNew:
			data.TodayNew++
		case learningrule.QueueReview:
			data.TodayReview++
		case learningrule.QueueRetry:
			data.TodayRetry++
		}
	}
	var rows []struct {
		LearningStatus string
		Count          int64
	}
	if err := db.DB.WithContext(ctx).Model(&db.UserItemLearning{}).
		Select("learning_status, COUNT(*) AS count").Group("learning_status").Scan(&rows).Error; err != nil {
		log.Printf("[learning-session] status counts failed session=%d err=%v", session.ID, err)
		return nil, err
	}
	for _, row := range rows {
		data.TotalItems += row.Count
		switch row.LearningStatus {
		case learningrule.StatusNotStarted:
			data.StatusCounts.NotStarted = row.Count
		case learningrule.StatusLearning:
			data.StatusCounts.Learning = row.Count
		case learningrule.StatusLearned:
			data.StatusCounts.Learned = row.Count
		case learningrule.StatusMastered:
			data.StatusCounts.Mastered = row.Count
		}
	}
	data.CanExtend = !data.HasCurrentCard &&
		data.SessionStatus == sessionCompleted &&
		data.StatusCounts.NotStarted > 0
	return data, nil
}

func GetCurrentLearningCard(ctx context.Context, sessionID int64) (*model.LearningCurrentData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	var session db.LearningSession
	if err := db.DB.WithContext(ctx).Where("id = ?", sessionID).Take(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLearningNotFound
		}
		log.Printf("[learning-current] load session failed session=%d err=%v", sessionID, err)
		return nil, err
	}
	return currentData(ctx, &session)
}

func SubmitLearningAnswer(ctx context.Context, sessionID int64, req *model.LearningAnswerRequest) (*model.LearningCurrentData, error) {
	if req == nil || req.QueueItemID <= 0 || req.TurnNo < 0 || (req.Answer != "known" && req.Answer != "unknown") {
		return nil, ErrLearningInvalid
	}
	now := time.Now()
	loc := learningLocation()
	var session db.LearningSession
	transactionStarted := time.Now()
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLearningNotFound
			}
			return err
		}
		if session.Status != sessionActive || session.CurrentQueueItemID == nil ||
			*session.CurrentQueueItemID != req.QueueItemID || session.TurnNo != req.TurnNo {
			return ErrLearningConflict
		}
		var queue db.UserLearningQueue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND session_id = ?", req.QueueItemID, sessionID).Take(&queue).Error; err != nil {
			return err
		}
		var item db.UserItemLearning
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("item_type = ? AND item_id = ?", queue.ItemType, queue.ItemID).Take(&item).Error; err != nil {
			return err
		}
		progress := learningrule.Progress{
			Status: item.LearningStatus, Stage: item.MemoryStage,
			ReviewSuccessDays: item.ReviewSuccessDays, NextReviewAt: item.NextReviewAt,
			LastReviewedAt: item.LastReviewedAt, LearnedAt: item.LearnedAt, MasteredAt: item.MasteredAt,
		}
		answerTurn := session.TurnNo + 1
		known := req.Answer == "known"
		outcome := learningrule.ApplySessionAnswer(queue.SourceQueueType, queue.KnownCount, known)
		queue.AnswerCount++
		queue.KnownCount = outcome.KnownCount
		if !known {
			progress = learningrule.ApplyUnknown(progress, now, loc)
			queue.QueueType = learningrule.QueueRetry
			queue.HadIncorrect = true
			queue.IncorrectCount++
		}
		if err := recordLearningAnswerAction(tx, &queue, sessionID, answerTurn, known, now); err != nil {
			log.Printf("[learning-answer] update word level failed session=%d queue=%d item=%d err=%v",
				sessionID, queue.ID, queue.ItemID, err)
			return err
		}
		if outcome.Completed {
			if queue.HadIncorrect {
				progress.LastReviewedAt = &now
			} else {
				var err error
				progress, err = learningrule.ApplyKnown(progress, now, loc)
				if err != nil {
					return err
				}
			}
			queue.Status = learningrule.QueueCompleted
			queue.CompletedAt = &now
			queue.RetryAfterTurn = nil
		} else {
			retryTurn := learningrule.RetryTurn(answerTurn)
			queue.Status = learningrule.QueueRetryWaiting
			queue.CompletedAt = nil
			queue.RetryAfterTurn = &retryTurn
		}
		if err := tx.Model(&db.UserItemLearning{}).Where("id = ?", item.ID).Updates(map[string]any{
			"learning_status": progress.Status, "memory_stage": progress.Stage,
			"review_success_days": progress.ReviewSuccessDays, "last_reviewed_at": progress.LastReviewedAt,
			"next_review_at": progress.NextReviewAt, "learned_at": progress.LearnedAt,
		}).Error; err != nil {
			log.Printf("[learning-answer] update progress failed session=%d item=%d err=%v", sessionID, queue.ItemID, err)
			return err
		}
		if err := tx.Save(&queue).Error; err != nil {
			return err
		}
		return advanceLearningSession(tx, &session, answerTurn, now)
	})
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=answer_transaction_completed session=%d queue=%d duration_ms=%d success=%t",
			traceID, sessionID, req.QueueItemID, time.Since(transactionStarted).Milliseconds(), err == nil)
	}
	if err != nil {
		log.Printf("[learning-answer] failed session=%d queue=%d turn=%d answer=%q err=%v",
			sessionID, req.QueueItemID, req.TurnNo, req.Answer, err)
		return nil, err
	}
	currentStarted := time.Now()
	data, err := currentData(ctx, &session)
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=answer_next_card_completed session=%d duration_ms=%d success=%t",
			traceID, sessionID, time.Since(currentStarted).Milliseconds(), err == nil)
	}
	return data, err
}

func MasterCurrentLearningItem(ctx context.Context, sessionID int64, req *model.LearningMasteryRequest) (*model.LearningCurrentData, error) {
	if req == nil || req.QueueItemID <= 0 || req.TurnNo < 0 {
		return nil, ErrLearningInvalid
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	now := time.Now()
	var session db.LearningSession
	transactionStarted := time.Now()
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLearningNotFound
			}
			return err
		}
		if session.Status != sessionActive || session.CurrentQueueItemID == nil ||
			*session.CurrentQueueItemID != req.QueueItemID || session.TurnNo != req.TurnNo {
			return ErrLearningConflict
		}
		var queue db.UserLearningQueue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND session_id = ?", req.QueueItemID, sessionID).Take(&queue).Error; err != nil {
			return err
		}
		var item db.UserItemLearning
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("item_type = ? AND item_id = ?", queue.ItemType, queue.ItemID).Take(&item).Error; err != nil {
			return err
		}
		progress := learningrule.ApplyMastered(learningrule.Progress{
			Status: item.LearningStatus, Stage: item.MemoryStage,
			ReviewSuccessDays: item.ReviewSuccessDays, NextReviewAt: item.NextReviewAt,
			LastReviewedAt: item.LastReviewedAt, LearnedAt: item.LearnedAt, MasteredAt: item.MasteredAt,
		}, now)
		if err := tx.Model(&db.UserItemLearning{}).Where("id = ?", item.ID).Updates(map[string]any{
			"learning_status":  progress.Status,
			"next_review_at":   progress.NextReviewAt,
			"last_reviewed_at": progress.LastReviewedAt,
			"mastered_at":      progress.MasteredAt,
		}).Error; err != nil {
			log.Printf("[learning-mastery] update progress failed session=%d queue=%d item=%d err=%v",
				sessionID, queue.ID, queue.ItemID, err)
			return err
		}
		queue.Status = learningrule.QueueCompleted
		queue.CompletedAt = &now
		queue.RetryAfterTurn = nil
		if err := tx.Save(&queue).Error; err != nil {
			log.Printf("[learning-mastery] update queue failed session=%d queue=%d item=%d err=%v",
				sessionID, queue.ID, queue.ItemID, err)
			return err
		}
		recordLearningMasteryAction(tx, &queue, sessionID, session.TurnNo+1, now)
		return advanceLearningSession(tx, &session, session.TurnNo+1, now)
	})
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=mastery_transaction_completed session=%d queue=%d duration_ms=%d success=%t",
			traceID, sessionID, req.QueueItemID, time.Since(transactionStarted).Milliseconds(), err == nil)
	}
	if err != nil {
		log.Printf("[learning-mastery] failed session=%d queue=%d turn=%d err=%v",
			sessionID, req.QueueItemID, req.TurnNo, err)
		return nil, err
	}
	currentStarted := time.Now()
	data, err := currentData(ctx, &session)
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=mastery_next_card_completed session=%d duration_ms=%d success=%t",
			traceID, sessionID, time.Since(currentStarted).Milliseconds(), err == nil)
	}
	return data, err
}

func advanceLearningSession(tx *gorm.DB, session *db.LearningSession, nextTurn int, now time.Time) error {
	var candidates []db.UserLearningQueue
	if err := tx.Where("session_id = ? AND status <> ?", session.ID, learningrule.QueueCompleted).
		Find(&candidates).Error; err != nil {
		log.Printf("[learning-session] load next candidates failed session=%d turn=%d err=%v",
			session.ID, nextTurn, err)
		return err
	}
	next := pickQueue(candidates, nextTurn+1)
	updates := map[string]any{"turn_no": nextTurn}
	session.TurnNo = nextTurn
	if next == nil {
		updates["status"] = sessionCompleted
		updates["completed_at"] = now
		updates["current_queue_item_id"] = nil
		session.Status = sessionCompleted
		session.CompletedAt = &now
		session.CurrentQueueItemID = nil
	} else {
		updates["current_queue_item_id"] = next.ID
		session.CurrentQueueItemID = &next.ID
	}
	if err := tx.Model(&db.LearningSession{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
		log.Printf("[learning-session] advance failed session=%d turn=%d err=%v", session.ID, nextTurn, err)
		return err
	}
	return nil
}

func pickQueue(rows []db.UserLearningQueue, nextTurn int) *db.UserLearningQueue {
	candidates := make([]learningrule.Candidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, learningrule.Candidate{
			ID: row.ID, QueueType: row.QueueType, QueueSeq: row.QueueSeq,
			Status: row.Status, RetryAfterTurn: row.RetryAfterTurn,
		})
	}
	picked := learningrule.PickNext(candidates, nextTurn)
	if picked == nil {
		return nil
	}
	for i := range rows {
		if rows[i].ID == picked.ID {
			return &rows[i]
		}
	}
	return nil
}

func currentData(ctx context.Context, session *db.LearningSession) (*model.LearningCurrentData, error) {
	started := time.Now()
	data := &model.LearningCurrentData{
		SessionID: session.ID, TurnNo: session.TurnNo,
		Completed: session.Status == sessionCompleted,
	}
	var queues []db.UserLearningQueue
	if err := db.DB.WithContext(ctx).Where("session_id = ?", session.ID).Find(&queues).Error; err != nil {
		log.Printf("[learning-current] load progress failed session=%d err=%v", session.ID, err)
		return nil, err
	}
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=progress_query_completed session=%d rows=%d duration_ms=%d",
			traceID, session.ID, len(queues), time.Since(started).Milliseconds())
	}
	data.Progress = summarizeSessionProgress(queues)
	data.Remaining = data.Progress.NotStarted + data.Progress.InProgress
	if session.CurrentQueueItemID == nil {
		return data, nil
	}
	var queue db.UserLearningQueue
	queueStarted := time.Now()
	if err := db.DB.WithContext(ctx).Where("id = ? AND session_id = ?", *session.CurrentQueueItemID, session.ID).Take(&queue).Error; err != nil {
		log.Printf("[learning-current] load queue failed session=%d queue=%d err=%v", session.ID, *session.CurrentQueueItemID, err)
		return nil, err
	}
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=queue_query_completed session=%d queue=%d duration_ms=%d",
			traceID, session.ID, queue.ID, time.Since(queueStarted).Milliseconds())
	}
	cardStarted := time.Now()
	card, err := loadLearningCard(ctx, queue.ItemType, queue.ItemID)
	if err != nil {
		return nil, err
	}
	if traceID := learningTrace(ctx); traceID != "" {
		log.Printf("[learning-trace] trace=%q phase=card_query_completed session=%d queue=%d item_type=%d item=%d duration_ms=%d",
			traceID, session.ID, queue.ID, queue.ItemType, queue.ItemID, time.Since(cardStarted).Milliseconds())
	}
	data.QueueItemID = queue.ID
	data.QueueType = queue.QueueType
	data.Card = card
	return data, nil
}

func summarizeSessionProgress(queues []db.UserLearningQueue) model.LearningSessionProgress {
	progress := model.LearningSessionProgress{Total: int64(len(queues))}
	for _, queue := range queues {
		if queue.Status == learningrule.QueueCompleted {
			progress.Completed++
		} else if queue.AnswerCount > 0 {
			progress.InProgress++
		} else {
			progress.NotStarted++
		}
	}
	return progress
}

func loadLearningCard(ctx context.Context, itemType int, itemID int64) (*model.LearningCard, error) {
	switch itemType {
	case LearningItemWord:
		return loadWordLearningCard(ctx, itemID)
	case LearningItemPhrase:
		return loadPhraseLearningCard(ctx, itemID)
	default:
		log.Printf("[learning-card] unsupported item item_type=%d item_id=%d", itemType, itemID)
		return nil, ErrLearningInvalid
	}
}

func loadWordLearningCard(ctx context.Context, itemID int64) (*model.LearningCard, error) {
	var word db.Word
	if err := db.DB.WithContext(ctx).Where("id = ?", itemID).Take(&word).Error; err != nil {
		log.Printf("[learning-card] load word failed item_type=%d item_id=%d err=%v", LearningItemWord, itemID, err)
		return nil, err
	}
	card := &model.LearningCard{
		ItemType: LearningItemWord, ItemID: itemID, Word: word.Word, Level: word.Level,
		Chinese: []model.LearningMeaning{}, English: []model.LearningMeaning{}, Contexts: []model.Context{},
	}
	note, err := loadLearningNote(ctx, ContextItemWord, word.Word)
	if err != nil {
		log.Printf("[learning-card] load note failed item_type=%d item_id=%d text=%q err=%v",
			LearningItemWord, itemID, word.Word, err)
		return nil, err
	}
	card.Note = note
	var audios []db.WordAudio
	if err := db.DB.WithContext(ctx).Where("word_id = ?", itemID).Order("id").Find(&audios).Error; err != nil {
		return nil, err
	}
	var chosen *db.WordAudio
	for i := range audios {
		if audios[i].Accent == "US" {
			chosen = &audios[i]
			break
		}
	}
	if chosen == nil && len(audios) > 0 {
		chosen = &audios[0]
	}
	if chosen != nil {
		card.Phonetic = chosen.PhoneticText
		card.AudioURL = converter.AudioProxyPath + "?src=" + url.QueryEscape(chosen.SourceURL)
	}
	var meanings []db.WordMeaning
	if err := db.DB.WithContext(ctx).Where("word_id = ?", itemID).Order("kind, sort_order").Find(&meanings).Error; err != nil {
		return nil, err
	}
	for _, meaning := range meanings {
		if meaning.Kind == "zh" {
			entry := model.LearningMeaning{
				PartOfSpeech: meaning.PartOfSpeech,
				Text:         meaning.Definition,
			}
			card.Chinese = append(card.Chinese, entry)
		} else if meaning.Kind == "en" {
			partOfSpeech, definition := dict.NormalizeDefinition(meaning.PartOfSpeech, meaning.Definition)
			entry := model.LearningMeaning{
				PartOfSpeech: partOfSpeech,
				Text:         definition,
			}
			card.English = append(card.English, entry)
		}
	}
	contexts, err := ListContexts(ctx, ContextItemWord, itemID)
	if err == nil {
		card.Contexts = contexts.Contexts
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	enrichLearningCardWithEcdict(ctx, card, word.Word)
	return card, nil
}

// enrichLearningCardWithEcdict 在学习卡片上叠加 ECDICT 元数据（pos/collins/oxford/tags）。
// ECDICT 未收录该词时保持零值，静默继续。
func enrichLearningCardWithEcdict(ctx context.Context, card *model.LearningCard, word string) {
	if !dict.Enabled() {
		return
	}
	entry, err := dict.Lookup(ctx, word)
	if err != nil {
		return
	}
	card.Pos = dict.NormalizePosField(entry.Pos)
	card.Collins = entry.Collins
	card.Oxford = entry.Oxford
	card.Tags = dict.SplitTags(entry.Tag)
}

func loadPhraseLearningCard(ctx context.Context, itemID int64) (*model.LearningCard, error) {
	var phrase db.Phrase
	if err := db.DB.WithContext(ctx).Where("id = ?", itemID).Take(&phrase).Error; err != nil {
		log.Printf("[learning-card] load phrase failed item_type=%d item_id=%d err=%v", LearningItemPhrase, itemID, err)
		return nil, err
	}
	card := &model.LearningCard{
		ItemType: LearningItemPhrase,
		ItemID:   itemID,
		Word:     phrase.Phrase,
		AudioURL: fmt.Sprintf("/api/v1/phrase/%d/audio", itemID),
		Chinese:  []model.LearningMeaning{},
		English:  []model.LearningMeaning{},
		Contexts: []model.Context{},
	}
	note, err := loadLearningNote(ctx, ContextItemPhrase, phrase.Phrase)
	if err != nil {
		log.Printf("[learning-card] load note failed item_type=%d item_id=%d text=%q err=%v",
			LearningItemPhrase, itemID, phrase.Phrase, err)
		return nil, err
	}
	card.Note = note
	var meanings []db.PhraseMeaning
	if err := db.DB.WithContext(ctx).Where("phrase_id = ?", itemID).
		Order("sort_order, id").Find(&meanings).Error; err != nil {
		log.Printf("[learning-card] load phrase meanings failed item_type=%d item_id=%d err=%v",
			LearningItemPhrase, itemID, err)
		return nil, err
	}
	for _, meaning := range meanings {
		card.Chinese = append(card.Chinese, model.LearningMeaning{
			PartOfSpeech: meaning.PartOfSpeech,
			Text:         meaning.Definition,
		})
	}
	contexts, err := ListContexts(ctx, ContextItemPhrase, itemID)
	if err == nil {
		card.Contexts = contexts.Contexts
	} else if !errors.Is(err, ErrNotFound) {
		log.Printf("[learning-card] load phrase contexts failed item_type=%d item_id=%d err=%v",
			LearningItemPhrase, itemID, err)
		return nil, err
	}
	return card, nil
}

func loadLearningNote(ctx context.Context, itemType, text string) (string, error) {
	data, err := GetNote(ctx, itemType, text)
	if err != nil {
		return "", err
	}
	return data.Note, nil
}
