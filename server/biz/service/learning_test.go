package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPickQueueUsesRulePriority(t *testing.T) {
	retryTurn := 9
	rows := []db.UserLearningQueue{
		{ID: 1, QueueType: learningrule.QueueReview, QueueSeq: 1, Status: learningrule.QueuePending},
		{ID: 2, QueueType: learningrule.QueueNew, QueueSeq: 2, Status: learningrule.QueuePending},
		{ID: 3, QueueType: learningrule.QueueRetry, QueueSeq: 3, Status: learningrule.QueueRetryWaiting, RetryAfterTurn: &retryTurn},
	}
	if got := pickQueue(rows, 8); got == nil || got.ID != 2 {
		t.Fatalf("before retry due got=%+v", got)
	}
	if got := pickQueue(rows, 9); got == nil || got.ID != 3 {
		t.Fatalf("after retry due got=%+v", got)
	}
}

func TestSummarizeSessionProgress(t *testing.T) {
	rows := []db.UserLearningQueue{
		{Status: learningrule.QueuePending, AnswerCount: 0},
		{Status: learningrule.QueueRetryWaiting, AnswerCount: 1},
		{Status: learningrule.QueueCompleted, AnswerCount: 1},
	}
	got := summarizeSessionProgress(rows)
	if got.Total != 3 || got.NotStarted != 1 || got.InProgress != 1 || got.Completed != 1 {
		t.Fatalf("progress = %+v", got)
	}
}

func TestMasterCurrentLearningItemRejectsInvalidRequest(t *testing.T) {
	requests := []*model.LearningMasteryRequest{
		nil,
		{QueueItemID: 0, TurnNo: 0},
		{QueueItemID: 1, TurnNo: -1},
	}
	for _, request := range requests {
		if _, err := MasterCurrentLearningItem(context.Background(), 1, request); !errors.Is(err, ErrLearningInvalid) {
			t.Fatalf("request=%+v err=%v", request, err)
		}
	}
}

func TestLoadLearningNoteSeparatesWordAndPhrase(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `item_notes` WHERE item_type = \\? AND item_text = \\?.*LIMIT \\?").
		WithArgs("word", "look", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_text", "note", "created_at", "updated_at",
		}).AddRow(1, "word", "look", "word note", nil, nil))
	mock.ExpectQuery("SELECT \\* FROM `item_notes` WHERE item_type = \\? AND item_text = \\?.*LIMIT \\?").
		WithArgs("phrase", "look up", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_text", "note", "created_at", "updated_at",
		}).AddRow(2, "phrase", "look up", "phrase note", nil, nil))

	wordNote, err := loadLearningNote(context.Background(), ContextItemWord, "Look")
	if err != nil || wordNote != "word note" {
		t.Fatalf("word note=%q err=%v", wordNote, err)
	}
	phraseNote, err := loadLearningNote(context.Background(), ContextItemPhrase, "Look   Up")
	if err != nil || phraseNote != "phrase note" {
		t.Fatalf("phrase note=%q err=%v", phraseNote, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLearningNoteMissingReturnsEmpty(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `item_notes` WHERE item_type = \\? AND item_text = \\?.*LIMIT \\?").
		WithArgs("word", "hello", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_text", "note", "created_at", "updated_at",
		}))

	note, err := loadLearningNote(context.Background(), ContextItemWord, "hello")
	if err != nil || note != "" {
		t.Fatalf("note=%q err=%v", note, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLearningSessionPrefersLaterQueuedNewItem_BitsUT(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectBegin()
	expectLearningSettings(mock, 50, LearningReviewUnlimited)
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE study_date >= \\? AND study_date < \\? FOR UPDATE").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO `learning_sessions`").
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? AND next_review_at IS NOT NULL AND next_review_at < \\? ORDER BY next_review_at, id").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusLearning, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_id", "queued_at", "learning_status",
		}).
			AddRow(2, LearningItemWord, 102, now, learningrule.StatusNotStarted).
			AddRow(1, LearningItemWord, 101, now.Add(-time.Hour), learningrule.StatusNotStarted))
	mock.ExpectExec("INSERT INTO `user_learning_queue`").
		WillReturnResult(sqlmock.NewResult(100, 2))
	mock.ExpectExec("UPDATE `learning_sessions` SET .*current_queue_item_id.* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT learning_status, COUNT\\(\\*\\) AS count FROM `user_item_learning` GROUP BY `learning_status`").
		WillReturnRows(sqlmock.NewRows([]string{"learning_status", "count"}).
			AddRow(learningrule.StatusNotStarted, 2))

	plan, err := CreateLearningSession(context.Background())
	if err != nil {
		t.Fatalf("CreateLearningSession() error = %v", err)
	}
	if plan.SessionID != 10 || !plan.HasCurrentCard || plan.TodayNew != 2 {
		t.Fatalf("CreateLearningSession() plan = %+v", plan)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLearningSessionMasteryCompletedConsumesDailyQuota_BitsUT(t *testing.T) {
	// Bad case 复现：今日旧 session 中，通过"一键掌握"完成的新词（AnswerCount=0, Status=completed）
	// 也必须占用当日新词名额，否则重建 session 时会重复分配新词。
	t.Setenv("LEARNING_DAILY_NEW_LIMIT", "1")
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectBegin()
	expectLearningSettings(mock, 1, LearningReviewUnlimited)
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE study_date >= \\? AND study_date < \\? FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id",
		}).AddRow(9, now, "superseded", 4, nil))
	mock.ExpectQuery("SELECT \\* FROM `user_learning_queue` WHERE session_id IN \\(\\?\\)").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "had_incorrect", "incorrect_count", "answer_count", "known_count",
			"retry_after_turn", "completed_at",
		}).AddRow(701, 9, LearningItemWord, 501,
			learningrule.QueueNew, learningrule.QueueNew,
			1.0, learningrule.QueueCompleted, false, 0, 0, 0, nil, now))
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id IN \\(\\?\\) AND status = \\?").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `learning_sessions`").
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? AND next_review_at IS NOT NULL AND next_review_at < \\? ORDER BY next_review_at, id").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusLearning, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_id", "queued_at", "learning_status",
		}).AddRow(3, LearningItemWord, 103, now, learningrule.StatusNotStarted))
	// 修复后：mastery-completed 的 item 501 计入 startedNew，remainingSlots=0，
	// 不会再往 user_learning_queue 插入任何新词，session 立即置为 completed。
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT learning_status, COUNT\\(\\*\\) AS count FROM `user_item_learning` GROUP BY `learning_status`").
		WillReturnRows(sqlmock.NewRows([]string{"learning_status", "count"}).
			AddRow(learningrule.StatusMastered, 1).
			AddRow(learningrule.StatusNotStarted, 1))

	plan, err := CreateLearningSession(context.Background())
	if err != nil {
		t.Fatalf("CreateLearningSession() error = %v", err)
	}
	if plan.TodayNew != 0 {
		t.Fatalf("todayNew = %d, want 0 (mastery-completed 新词应当占用当日名额)", plan.TodayNew)
	}
	if plan.TodayTotal != 0 {
		t.Fatalf("todayTotal = %d, want 0", plan.TodayTotal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLearningSessionDoesNotRefillAfterCompletedItemDeleted_BitsUT(t *testing.T) {
	t.Setenv("LEARNING_DAILY_NEW_LIMIT", "1")
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectBegin()
	expectLearningSettings(mock, 1, LearningReviewUnlimited)
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE study_date >= \\? AND study_date < \\? FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id",
		}).AddRow(9, now, sessionCompleted, 4, nil))
	// The completed item's queue was removed together with the favorite.
	mock.ExpectQuery("SELECT \\* FROM `user_learning_queue` WHERE session_id IN \\(\\?\\)").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id IN \\(\\?\\) AND status = \\?").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `learning_sessions`").
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? AND next_review_at IS NOT NULL AND next_review_at < \\? ORDER BY next_review_at, id").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusLearning, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_id", "queued_at", "learning_status",
		}).AddRow(3, LearningItemWord, 103, now, learningrule.StatusNotStarted))
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT learning_status, COUNT\\(\\*\\) AS count FROM `user_item_learning` GROUP BY `learning_status`").
		WillReturnRows(sqlmock.NewRows([]string{"learning_status", "count"}).
			AddRow(learningrule.StatusNotStarted, 1))

	plan, err := CreateLearningSession(context.Background())
	if err != nil {
		t.Fatalf("CreateLearningSession() error = %v", err)
	}
	if plan.TodayTotal != 0 || plan.TodayNew != 0 || plan.HasCurrentCard {
		t.Fatalf("CreateLearningSession() plan = %+v, want completed day with no refill", plan)
	}
	if !plan.CanExtend {
		t.Fatal("plan.CanExtend = false, want explicit continue-learning option")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectDailyNewRowsUsesGloballyNewestCandidates_BitsUT(t *testing.T) {
	now := time.Now()
	rows := []db.UserItemLearning{
		{ItemType: LearningItemWord, ItemID: 202, QueuedAt: now},
		{ItemType: LearningItemWord, ItemID: 201, QueuedAt: now.Add(-time.Hour)},
	}

	selected := selectDailyNewRows(rows, 1)
	if len(selected) != 1 || selected[0].ItemID != 202 {
		t.Fatalf("selectDailyNewRows() = %+v, want latest item 202", selected)
	}
}

func TestSelectDailyReviewRowsHonorsFiniteAndUnlimitedLimits_BitsUT(t *testing.T) {
	rows := []db.UserItemLearning{{ID: 1}, {ID: 2}, {ID: 3}}

	finite := selectDailyReviewRows(rows, 2)
	if len(finite) != 2 || finite[0].ID != 1 || finite[1].ID != 2 {
		t.Fatalf("finite selection = %+v, want first two rows", finite)
	}
	unlimited := selectDailyReviewRows(rows, LearningReviewUnlimited)
	if len(unlimited) != 3 {
		t.Fatalf("unlimited selection count = %d, want 3", len(unlimited))
	}
	if got := selectDailyReviewRows(rows, 0); got != nil {
		t.Fatalf("zero-limit selection = %+v, want nil", got)
	}
}

func TestRemainingDailyReviewSlotsCountsConsumedReviews_BitsUT(t *testing.T) {
	if got := remainingDailyReviewSlots(20, 7); got != 13 {
		t.Fatalf("remaining slots = %d, want 13", got)
	}
	if got := remainingDailyReviewSlots(5, 7); got != 0 {
		t.Fatalf("remaining slots below zero = %d, want 0", got)
	}
	if got := remainingDailyReviewSlots(LearningReviewUnlimited, 500); got != LearningReviewUnlimited {
		t.Fatalf("unlimited remaining slots = %d, want %d", got, LearningReviewUnlimited)
	}
}

func TestCreateLearningSessionPreservesReviewRetryAndFillsRemainingCap_BitsUT(t *testing.T) {
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	retryTurn := 8
	mock.ExpectBegin()
	expectLearningSettings(mock, 50, 5)
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE study_date >= \\? AND study_date < \\? FOR UPDATE").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id",
		}).AddRow(9, now, sessionActive, 3, 701))
	mock.ExpectQuery("SELECT \\* FROM `user_learning_queue` WHERE session_id IN \\(\\?\\)").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "had_incorrect", "incorrect_count", "answer_count", "known_count",
			"retry_after_turn",
		}).AddRow(
			701, 9, LearningItemWord, 501,
			learningrule.QueueRetry, learningrule.QueueReview,
			1.0, learningrule.QueueRetryWaiting, true, 1, 1, 0, retryTurn,
		))
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id IN \\(\\?\\) AND status = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `learning_sessions`").
		WillReturnResult(sqlmock.NewResult(10, 1))

	dueRows := sqlmock.NewRows([]string{
		"id", "item_type", "item_id", "learning_status", "next_review_at",
	}).AddRow(1, LearningItemWord, 501, learningrule.StatusLearning, now)
	for i := 0; i < 6; i++ {
		dueRows.AddRow(2+i, LearningItemWord, 600+i, learningrule.StatusLearning, now)
	}
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? AND next_review_at IS NOT NULL AND next_review_at < \\? ORDER BY next_review_at, id").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusLearning, sqlmock.AnyArg()).
		WillReturnRows(dueRows)
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO `user_learning_queue`").
		WillReturnResult(sqlmock.NewResult(800, 5))
	mock.ExpectExec("UPDATE `learning_sessions` SET .*current_queue_item_id.* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT learning_status, COUNT\\(\\*\\) AS count FROM `user_item_learning` GROUP BY `learning_status`").
		WillReturnRows(sqlmock.NewRows([]string{"learning_status", "count"}).
			AddRow(learningrule.StatusLearning, 7))

	plan, err := CreateLearningSession(context.Background())
	if err != nil {
		t.Fatalf("CreateLearningSession() error = %v", err)
	}
	if plan.TodayRetry != 1 || plan.TodayReview != 4 || plan.TodayTotal != 5 {
		t.Fatalf("plan = %+v, want retry=1 review=4 total=5", plan)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectLearningSettings(mock sqlmock.Sqlmock, dailyNewLimit, dailyReviewLimit int) {
	mock.ExpectQuery("SELECT \\* FROM `learning_settings` WHERE id = \\? LIMIT \\?").
		WithArgs(learningSettingsID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "daily_new_limit", "daily_review_limit",
		}).AddRow(learningSettingsID, dailyNewLimit, dailyReviewLimit))
}

func TestExtendLearningSessionAddsExtraBatchAndReactivates_BitsUT(t *testing.T) {
	// Success 路径：当日 session 已完成，仍有 15 条未开始新词候选，
	// 调用 ExtendLearningSession 应追加 10 条新词 queue，把 session 状态复位为 active。
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	loc := learningLocation()
	studyDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE id = \\? .*FOR UPDATE").
		WithArgs(int64(20), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id",
		}).AddRow(20, studyDate, "completed", 100, nil))
	mock.ExpectQuery("SELECT \\* FROM `user_learning_queue` WHERE session_id = \\?").
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "had_incorrect", "incorrect_count", "answer_count", "known_count",
		}))
	itemRows := sqlmock.NewRows([]string{
		"id", "item_type", "item_id", "queued_at", "learning_status",
	})
	for i := 0; i < 15; i++ {
		itemRows = itemRows.AddRow(int64(1000+i), LearningItemWord, int64(2000+i),
			now.Add(-time.Duration(i)*time.Minute), learningrule.StatusNotStarted)
	}
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(itemRows)
	mock.ExpectExec("INSERT INTO `user_learning_queue`").
		WillReturnResult(sqlmock.NewResult(9001, 10))
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT learning_status, COUNT\\(\\*\\) AS count FROM `user_item_learning` GROUP BY `learning_status`").
		WillReturnRows(sqlmock.NewRows([]string{"learning_status", "count"}).
			AddRow(learningrule.StatusNotStarted, 5).
			AddRow(learningrule.StatusLearning, 10))

	plan, err := ExtendLearningSession(context.Background(), 20)
	if err != nil {
		t.Fatalf("ExtendLearningSession() error = %v", err)
	}
	if plan.SessionID != 20 {
		t.Fatalf("session id = %d, want 20", plan.SessionID)
	}
	if plan.SessionStatus != "active" {
		t.Fatalf("session status = %q, want active", plan.SessionStatus)
	}
	if !plan.HasCurrentCard {
		t.Fatalf("plan.HasCurrentCard = false, want true")
	}
	if plan.TodayNew != 10 || plan.TodayTotal != 10 {
		t.Fatalf("todayNew=%d todayTotal=%d, want 10/10", plan.TodayNew, plan.TodayTotal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExtendLearningSessionReturnsConflictWhenNoMoreCandidates_BitsUT(t *testing.T) {
	// Bad case：session 已完成但 not_started 已耗尽，应返回 ErrLearningConflict 并回滚事务。
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	loc := learningLocation()
	studyDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE id = \\? .*FOR UPDATE").
		WithArgs(int64(21), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id",
		}).AddRow(21, studyDate, "completed", 50, nil))
	mock.ExpectQuery("SELECT \\* FROM `user_learning_queue` WHERE session_id = \\?").
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "had_incorrect", "incorrect_count", "answer_count", "known_count",
		}))
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_id", "queued_at", "learning_status",
		}))
	mock.ExpectRollback()

	_, err := ExtendLearningSession(context.Background(), 21)
	if !errors.Is(err, ErrLearningConflict) {
		t.Fatalf("err = %v, want ErrLearningConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExtendLearningSessionSkipsAlreadyQueuedItems_BitsUT(t *testing.T) {
	// Bad case：not_started 里恰好有一条 item 已经存在于当前 session queue（异常态），
	// 追加时必须跳过该条，避免 uk_session_item 唯一索引冲突。
	mock, cleanup := installNoteMockDB(t)
	defer cleanup()

	now := time.Now()
	loc := learningLocation()
	studyDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `learning_sessions` WHERE id = \\? .*FOR UPDATE").
		WithArgs(int64(22), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "study_date", "status", "turn_no", "current_queue_item_id",
		}).AddRow(22, studyDate, "completed", 12, nil))
	mock.ExpectQuery("SELECT \\* FROM `user_learning_queue` WHERE session_id = \\?").
		WithArgs(int64(22)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "item_type", "item_id", "queue_type", "source_queue_type",
			"queue_seq", "status", "had_incorrect", "incorrect_count", "answer_count", "known_count",
		}).AddRow(701, 22, LearningItemWord, int64(3007),
			learningrule.QueueNew, learningrule.QueueNew,
			1.0, learningrule.QueueCompleted, false, 0, 1, 1))
	itemRows := sqlmock.NewRows([]string{
		"id", "item_type", "item_id", "queued_at", "learning_status",
	})
	// 插入 12 条 not_started，其中一条 ItemID=3007 已在当前 session queue。
	// 期望 selectDailyNewRows 后剩余 11 条候选，取前 10 条插入。
	for i := 0; i < 12; i++ {
		itemID := int64(3000 + i)
		itemRows = itemRows.AddRow(int64(4000+i), LearningItemWord, itemID,
			now.Add(-time.Duration(i)*time.Minute), learningrule.StatusNotStarted)
	}
	mock.ExpectQuery("SELECT \\* FROM `user_item_learning` WHERE item_type IN \\(\\?,\\?\\) AND learning_status = \\? ORDER BY queued_at DESC, id DESC").
		WithArgs(LearningItemWord, LearningItemPhrase, learningrule.StatusNotStarted).
		WillReturnRows(itemRows)
	mock.ExpectExec("INSERT INTO `user_learning_queue`").
		WillReturnResult(sqlmock.NewResult(9101, 10))
	mock.ExpectExec("UPDATE `learning_sessions` SET .* WHERE id = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT learning_status, COUNT\\(\\*\\) AS count FROM `user_item_learning` GROUP BY `learning_status`").
		WillReturnRows(sqlmock.NewRows([]string{"learning_status", "count"}).
			AddRow(learningrule.StatusNotStarted, 2))

	plan, err := ExtendLearningSession(context.Background(), 22)
	if err != nil {
		t.Fatalf("ExtendLearningSession() error = %v", err)
	}
	// 已有 queue(1) + 新增 10 = 11 条 total；TodayNew 应为 11（1 条旧 QueueNew + 10 条新 QueueNew）。
	if plan.TodayTotal != 11 || plan.TodayNew != 11 {
		t.Fatalf("todayTotal=%d todayNew=%d, want 11/11", plan.TodayTotal, plan.TodayNew)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
