package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/storage"
	"aaa_word/biz/textsegment"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ContextItemWord   = "word"
	ContextItemPhrase = "phrase"

	ContextTaskPending    = "pending"
	ContextTaskProcessing = "processing"
	ContextTaskSucceeded  = "succeeded"
	ContextTaskFailed     = "failed"

	maxContextParagraphUTF16 = 20_000
)

var (
	ErrContextTaskConflict = errors.New("context task is already processing")
	ErrContextTaskFailed   = errors.New("context task has failed")
	ErrContextItemNotFound = errors.New("context item not found")
	ErrContextItemMismatch = textsegment.ErrSelectionMismatch
)

// NormalizeContextTaskRequest 规范化并校验语境任务输入。
func NormalizeContextTaskRequest(req *model.ContextTaskRequest) (*model.ContextTaskRequest, error) {
	if req == nil {
		return nil, errors.New("context task request is nil")
	}

	itemType := strings.ToLower(strings.TrimSpace(req.ItemType))
	if itemType != ContextItemWord && itemType != ContextItemPhrase {
		return nil, errors.New("invalid item type")
	}
	if req.ItemID <= 0 {
		return nil, errors.New("invalid item id")
	}

	if strings.TrimSpace(req.Paragraph) == "" {
		return nil, errors.New("paragraph is empty")
	}
	if textsegment.UTF16Length(req.Paragraph) > maxContextParagraphUTF16 {
		return nil, errors.New("paragraph is too long")
	}
	if err := textsegment.ValidateUTF16Range(req.Paragraph, req.SelectionStart, req.SelectionEnd); err != nil {
		return nil, errors.New("invalid selection range")
	}

	return &model.ContextTaskRequest{
		ItemType:       itemType,
		ItemID:         req.ItemID,
		Paragraph:      req.Paragraph,
		SelectionStart: req.SelectionStart,
		SelectionEnd:   req.SelectionEnd,
		Source:         normalizeContextActionSource(req.Source),
	}, nil
}

// ContextDedupeKey 返回同一目标、句子和高亮位置的稳定幂等键。
func ContextTaskDedupeKey(req *model.ContextTaskRequest) string {
	raw := fmt.Sprintf("%s\x00%d\x00%s\x00%d\x00%d",
		req.ItemType, req.ItemID, req.Paragraph, req.SelectionStart, req.SelectionEnd)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ContextDedupeKey 返回最终语境内容的稳定幂等键。
func ContextDedupeKey(itemType string, itemID int64, sentence string, highlightStart, highlightEnd int) string {
	raw := fmt.Sprintf("%s\x00%d\x00%s\x00%d\x00%d",
		itemType, itemID, sentence, highlightStart, highlightEnd)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ProcessContextTask 创建并同步执行一个语境任务。
func ProcessContextTask(ctx context.Context, rawReq *model.ContextTaskRequest) (*model.ContextTaskData, error) {
	if !db.Enabled() {
		log.Printf("[context-task] db disabled")
		return nil, db.ErrDBDisabled
	}
	req, err := NormalizeContextTaskRequest(rawReq)
	if err != nil {
		log.Printf("[context-task] invalid request item_type=%q item_id=%d paragraph=%q selection_start=%d selection_end=%d err=%v",
			rawReqValue(rawReq, func(r *model.ContextTaskRequest) string { return r.ItemType }),
			rawReqInt64(rawReq, func(r *model.ContextTaskRequest) int64 { return r.ItemID }),
			truncate(rawReqValue(rawReq, func(r *model.ContextTaskRequest) string { return r.Paragraph }), 200),
			rawReqInt(rawReq, func(r *model.ContextTaskRequest) int { return r.SelectionStart }),
			rawReqInt(rawReq, func(r *model.ContextTaskRequest) int { return r.SelectionEnd }), err)
		return nil, err
	}
	itemText, err := loadContextItemText(ctx, req.ItemType, req.ItemID)
	if err != nil {
		return nil, err
	}
	highlight, err := textsegment.ValidateSelection(
		req.Paragraph,
		req.SelectionStart,
		req.SelectionEnd,
		itemText,
	)
	if err != nil {
		if errors.Is(err, textsegment.ErrSelectionMismatch) {
			log.Printf("[context-task] item mismatch item_type=%q item_id=%d item_text=%q highlight=%q",
				req.ItemType, req.ItemID, itemText, highlight)
			return nil, ErrContextItemMismatch
		}
		log.Printf("[context-task] validate selection failed item_type=%q item_id=%d err=%v",
			req.ItemType, req.ItemID, err)
		return nil, err
	}
	taskDedupeKey := ContextTaskDedupeKey(req)

	task := db.ContextTask{
		DedupeKey:         taskDedupeKey,
		ItemType:          req.ItemType,
		ItemID:            req.ItemID,
		RawParagraph:      req.Paragraph,
		RawSelectionStart: req.SelectionStart,
		RawSelectionEnd:   req.SelectionEnd,
		SplitterVersion:   textsegment.Version,
		Status:            ContextTaskPending,
		LastError:         "",
	}
	createResult := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dedupe_key"}},
		DoNothing: true,
	}).Create(&task)
	if createResult.Error != nil {
		log.Printf("[context-task] create failed item_type=%q item_id=%d item_text=%q key=%s err=%v",
			req.ItemType, req.ItemID, itemText, taskDedupeKey, createResult.Error)
		return nil, fmt.Errorf("create context task: %w", createResult.Error)
	}
	if createResult.RowsAffected == 0 {
		if err := db.DB.WithContext(ctx).Where("dedupe_key = ?", taskDedupeKey).Take(&task).Error; err != nil {
			log.Printf("[context-task] load duplicate failed item_type=%q item_id=%d key=%s err=%v",
				req.ItemType, req.ItemID, taskDedupeKey, err)
			return nil, fmt.Errorf("load context task: %w", err)
		}
		switch task.Status {
		case ContextTaskSucceeded:
			log.Printf("[context-task] duplicate succeeded task_id=%d context_id=%v key=%s",
				task.ID, task.ResultContextID, taskDedupeKey)
			if task.ResultContextID != nil {
				recordContextSavedBestEffort(ctx, req.ItemType, req.ItemID, itemText, req.Source, *task.ResultContextID)
			}
			return contextTaskData(&task), nil
		case ContextTaskPending:
			// A previous process may have stopped after inserting the task but before claiming it.
		case ContextTaskProcessing:
			log.Printf("[context-task] duplicate processing task_id=%d key=%s", task.ID, taskDedupeKey)
			return contextTaskData(&task), ErrContextTaskConflict
		case ContextTaskFailed:
			log.Printf("[context-task] duplicate failed task_id=%d key=%s last_error=%q",
				task.ID, taskDedupeKey, truncate(task.LastError, 300))
			return contextTaskData(&task), ErrContextTaskFailed
		default:
			log.Printf("[context-task] invalid existing status task_id=%d status=%q key=%s",
				task.ID, task.Status, taskDedupeKey)
			return contextTaskData(&task), ErrContextTaskConflict
		}
	}

	now := time.Now()
	claim := db.DB.WithContext(ctx).Model(&db.ContextTask{}).
		Where("id = ? AND status = ?", task.ID, ContextTaskPending).
		Updates(map[string]any{
			"status":     ContextTaskProcessing,
			"attempts":   gorm.Expr("attempts + 1"),
			"started_at": now,
			"last_error": "",
		})
	if claim.Error != nil {
		log.Printf("[context-task] claim failed task_id=%d key=%s err=%v", task.ID, taskDedupeKey, claim.Error)
		return contextTaskData(&task), fmt.Errorf("claim context task: %w", claim.Error)
	}
	if claim.RowsAffected != 1 {
		if err := db.DB.WithContext(ctx).Where("id = ?", task.ID).Take(&task).Error; err != nil {
			log.Printf("[context-task] reload after claim miss failed task_id=%d err=%v", task.ID, err)
			return nil, fmt.Errorf("reload context task: %w", err)
		}
		log.Printf("[context-task] claim conflict task_id=%d status=%q key=%s", task.ID, task.Status, taskDedupeKey)
		return contextTaskData(&task), ErrContextTaskConflict
	}
	task.Status = ContextTaskProcessing
	task.Attempts++
	task.StartedAt = &now

	extracted, err := textsegment.ExtractSentence(req.Paragraph, req.SelectionStart, req.SelectionEnd)
	if err != nil {
		return failContextTask(ctx, &task, fmt.Errorf("extract sentence: %w", err))
	}
	if err := saveTaskExtraction(ctx, task.ID, extracted); err != nil {
		return failContextTask(ctx, &task, err)
	}
	task.ExtractedSentence = &extracted.Sentence
	task.ExtractedSentenceStart = &extracted.SentenceStart
	task.ExtractedSentenceEnd = &extracted.SentenceEnd
	task.ExtractedHighlightStart = &extracted.HighlightStart
	task.ExtractedHighlightEnd = &extracted.HighlightEnd

	contextDedupeKey := ContextDedupeKey(
		req.ItemType,
		req.ItemID,
		extracted.Sentence,
		extracted.HighlightStart,
		extracted.HighlightEnd,
	)
	if existing, err := findContextByDedupeKey(ctx, contextDedupeKey); err == nil {
		if err := markContextTaskSucceeded(ctx, task.ID, existing.ID); err != nil {
			log.Printf("[context-task] recover existing context update failed task_id=%d context_id=%d err=%v",
				task.ID, existing.ID, err)
			return failContextTask(ctx, &task, err)
		}
		task.Status = ContextTaskSucceeded
		task.ResultContextID = &existing.ID
		recordContextSavedBestEffort(ctx, req.ItemType, req.ItemID, itemText, req.Source, existing.ID)
		return contextTaskData(&task), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return failContextTask(ctx, &task, fmt.Errorf("check existing context: %w", err))
	}

	translation, err := TranslateSentence(extracted.Sentence)
	if err != nil {
		return failContextTask(ctx, &task, fmt.Errorf("translate sentence: %w", err))
	}
	audioBytes, contentType, err := SynthesizeSentenceMP3(extracted.Sentence)
	if err != nil {
		return failContextTask(ctx, &task, fmt.Errorf("synthesize sentence audio: %w", err))
	}
	audioPath, audioSize, err := storage.SaveContextAudio(contextDedupeKey, bytes.NewReader(audioBytes))
	if err != nil {
		return failContextTask(ctx, &task, fmt.Errorf("save sentence audio: %w", err))
	}

	contextRow := db.Context{
		ItemType:         req.ItemType,
		ItemID:           req.ItemID,
		Sentence:         extracted.Sentence,
		Translation:      translation,
		HighlightStart:   extracted.HighlightStart,
		HighlightEnd:     extracted.HighlightEnd,
		AudioFilePath:    audioPath,
		AudioContentType: contentType,
		AudioFileSize:    audioSize,
		DedupeKey:        contextDedupeKey,
	}
	contextCreated := false
	preserveExistingAudio := false
	err = db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockContextItem(tx, req.ItemType, req.ItemID); err != nil {
			log.Printf("[context-task] final item check failed task_id=%d item_type=%q item_id=%d err=%v",
				task.ID, req.ItemType, req.ItemID, err)
			return err
		}
		createContext := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "dedupe_key"}},
			DoNothing: true,
		}).Create(&contextRow)
		if createContext.Error != nil {
			return fmt.Errorf("create context: %w", createContext.Error)
		}
		if createContext.RowsAffected == 0 {
			preserveExistingAudio = true
			if err := tx.Where("dedupe_key = ?", contextDedupeKey).Take(&contextRow).Error; err != nil {
				return fmt.Errorf("load existing context: %w", err)
			}
		} else {
			contextCreated = true
		}
		if contextRow.ID == 0 {
			return errors.New("create context: missing id")
		}
		finishedAt := time.Now()
		result := tx.Model(&db.ContextTask{}).
			Where("id = ? AND status = ?", task.ID, ContextTaskProcessing).
			Updates(map[string]any{
				"status":            ContextTaskSucceeded,
				"result_context_id": contextRow.ID,
				"finished_at":       finishedAt,
				"last_error":        "",
			})
		if result.Error != nil {
			return fmt.Errorf("finish context task: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errors.New("finish context task: status changed")
		}
		return nil
	})
	if err != nil {
		if contextCreated || !preserveExistingAudio {
			if deleteErr := storage.Delete(audioPath); deleteErr != nil {
				log.Printf("[context-task] cleanup audio failed task_id=%d path=%q err=%v", task.ID, audioPath, deleteErr)
			}
		}
		return failContextTask(ctx, &task, err)
	}

	task.Status = ContextTaskSucceeded
	task.ResultContextID = &contextRow.ID
	recordContextSavedBestEffort(ctx, req.ItemType, req.ItemID, itemText, req.Source, contextRow.ID)
	log.Printf("[context-task] succeeded task_id=%d context_id=%d item_type=%q item_id=%d item_text=%q splitter=%q sentence=%q audio_size=%d",
		task.ID, contextRow.ID, req.ItemType, req.ItemID, itemText, extracted.Version, truncate(extracted.Sentence, 200), audioSize)
	return contextTaskData(&task), nil
}

func normalizeContextActionSource(source string) string {
	switch strings.TrimSpace(source) {
	case model.ActionSourceAndroidReader:
		return model.ActionSourceAndroidReader
	case model.ActionSourceWebVideo:
		return model.ActionSourceWebVideo
	default:
		return model.ActionSourceSystem
	}
}

func rawReqValue(req *model.ContextTaskRequest, get func(*model.ContextTaskRequest) string) string {
	if req == nil {
		return ""
	}
	return get(req)
}

func rawReqInt(req *model.ContextTaskRequest, get func(*model.ContextTaskRequest) int) int {
	if req == nil {
		return 0
	}
	return get(req)
}

func rawReqInt64(req *model.ContextTaskRequest, get func(*model.ContextTaskRequest) int64) int64 {
	if req == nil {
		return 0
	}
	return get(req)
}

func contextTaskData(task *db.ContextTask) *model.ContextTaskData {
	return &model.ContextTaskData{
		TaskID:        task.ID,
		Status:        task.Status,
		ContextID:     task.ResultContextID,
		SentenceStart: task.ExtractedSentenceStart,
		SentenceEnd:   task.ExtractedSentenceEnd,
		LastError:     task.LastError,
	}
}

func failContextTask(ctx context.Context, task *db.ContextTask, cause error) (*model.ContextTaskData, error) {
	lastError := truncate(cause.Error(), 2000)
	finishedAt := time.Now()
	result := db.DB.WithContext(ctx).Model(&db.ContextTask{}).
		Where("id = ? AND status = ?", task.ID, ContextTaskProcessing).
		Updates(map[string]any{
			"status":      ContextTaskFailed,
			"last_error":  lastError,
			"finished_at": finishedAt,
		})
	if result.Error != nil {
		log.Printf("[context-task] mark failed error task_id=%d cause=%v update_err=%v", task.ID, cause, result.Error)
		return contextTaskData(task), fmt.Errorf("%w; update failed task: %v", cause, result.Error)
	}
	if result.RowsAffected != 1 {
		log.Printf("[context-task] mark failed status changed task_id=%d cause=%v", task.ID, cause)
		return contextTaskData(task), cause
	}
	task.Status = ContextTaskFailed
	task.LastError = lastError
	task.FinishedAt = &finishedAt
	log.Printf("[context-task] failed task_id=%d item_type=%q item_id=%d attempts=%d paragraph=%q err=%v",
		task.ID, task.ItemType, task.ItemID, task.Attempts, truncate(task.RawParagraph, 200), cause)
	return contextTaskData(task), cause
}

func findContextByDedupeKey(ctx context.Context, dedupeKey string) (*db.Context, error) {
	var row db.Context
	if err := db.DB.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).Take(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[context] find by dedupe failed key=%s err=%v", dedupeKey, err)
		}
		return nil, err
	}
	return &row, nil
}

func markContextTaskSucceeded(ctx context.Context, taskID, contextID int64) error {
	now := time.Now()
	result := db.DB.WithContext(ctx).Model(&db.ContextTask{}).
		Where("id = ? AND status = ?", taskID, ContextTaskProcessing).
		Updates(map[string]any{
			"status":            ContextTaskSucceeded,
			"result_context_id": contextID,
			"finished_at":       now,
			"last_error":        "",
		})
	if result.Error != nil {
		return fmt.Errorf("mark context task succeeded: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("mark context task succeeded: status changed")
	}
	return nil
}

func saveTaskExtraction(ctx context.Context, taskID int64, extracted textsegment.Result) error {
	result := db.DB.WithContext(ctx).Model(&db.ContextTask{}).
		Where("id = ? AND status = ?", taskID, ContextTaskProcessing).
		Updates(map[string]any{
			"splitter_version":          extracted.Version,
			"extracted_sentence":        extracted.Sentence,
			"extracted_sentence_start":  extracted.SentenceStart,
			"extracted_sentence_end":    extracted.SentenceEnd,
			"extracted_highlight_start": extracted.HighlightStart,
			"extracted_highlight_end":   extracted.HighlightEnd,
		})
	if result.Error != nil {
		log.Printf("[context-task] save extraction failed task_id=%d splitter=%q err=%v",
			taskID, extracted.Version, result.Error)
		return fmt.Errorf("save context extraction: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		log.Printf("[context-task] save extraction status changed task_id=%d splitter=%q",
			taskID, extracted.Version)
		return errors.New("save context extraction: status changed")
	}
	return nil
}

func loadContextItemText(ctx context.Context, itemType string, itemID int64) (string, error) {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err := db.DB.WithContext(ctx).Where("id = ?", itemID).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("[context-item] word not found item_id=%d", itemID)
				return "", ErrContextItemNotFound
			}
			log.Printf("[context-item] query word failed item_id=%d err=%v", itemID, err)
			return "", err
		}
		return strings.ToLower(strings.TrimSpace(word.Word)), nil
	case ContextItemPhrase:
		var phrase db.Phrase
		if err := db.DB.WithContext(ctx).Where("id = ?", itemID).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("[context-item] phrase not found item_id=%d", itemID)
				return "", ErrContextItemNotFound
			}
			log.Printf("[context-item] query phrase failed item_id=%d err=%v", itemID, err)
			return "", err
		}
		return strings.ToLower(strings.Join(strings.Fields(phrase.Phrase), " ")), nil
	default:
		return "", errors.New("unsupported context item type")
	}
}

func lockContextItem(tx *gorm.DB, itemType string, itemID int64) error {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").Where("id = ?", itemID).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrContextItemNotFound
			}
			return fmt.Errorf("lock context word: %w", err)
		}
	case ContextItemPhrase:
		var phrase db.Phrase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").Where("id = ?", itemID).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrContextItemNotFound
			}
			return fmt.Errorf("lock context phrase: %w", err)
		}
	default:
		return errors.New("unsupported context item type")
	}
	return nil
}

// ListContexts 查询指定收藏项的全部成功语境。
func ListContexts(ctx context.Context, itemType string, itemID int64) (*model.ContextsData, error) {
	if !db.Enabled() {
		return nil, ErrNotFound
	}
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	itemText, err := loadContextItemText(ctx, itemType, itemID)
	if err != nil {
		if errors.Is(err, ErrContextItemNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var rows []db.Context
	if err := db.DB.WithContext(ctx).
		Where("item_type = ? AND item_id = ?", itemType, itemID).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		log.Printf("[context] list failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}

	contextIDs := make([]int64, 0, len(rows))
	for i := range rows {
		contextIDs = append(contextIDs, rows[i].ID)
	}
	videosByContext, err := LoadContextVideos(ctx, contextIDs)
	if err != nil {
		log.Printf("[context] list videos failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
		return nil, err
	}

	data := &model.ContextsData{Word: itemText, Contexts: make([]model.Context, 0, len(rows))}
	for _, row := range rows {
		highlight, err := textsegment.SliceUTF16(row.Sentence, row.HighlightStart, row.HighlightEnd)
		if err != nil {
			log.Printf("[context] invalid stored highlight context_id=%d start=%d end=%d err=%v",
				row.ID, row.HighlightStart, row.HighlightEnd, err)
			highlight = itemText
		}
		data.Contexts = append(data.Contexts, model.Context{
			ID:             row.ID,
			ItemType:       row.ItemType,
			ItemID:         row.ItemID,
			Sentence:       row.Sentence,
			Translation:    row.Translation,
			Highlight:      highlight,
			HighlightStart: row.HighlightStart,
			HighlightEnd:   row.HighlightEnd,
			AudioURL:       fmt.Sprintf("/api/v1/contexts/%d/audio", row.ID),
			ContextType:    "text",
			Videos:         videosByContext[row.ID],
		})
		if len(data.Contexts[len(data.Contexts)-1].Videos) > 0 {
			data.Contexts[len(data.Contexts)-1].ContextType = "video"
		}
	}
	return data, nil
}

// LookupContextAudio 返回语境本地音频元数据。
func LookupContextAudio(ctx context.Context, id int64) (string, string, error) {
	if !db.Enabled() {
		return "", "", db.ErrDBDisabled
	}
	var row db.Context
	if err := db.DB.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[context-audio] query failed context_id=%d err=%v", id, err)
		}
		return "", "", err
	}
	if row.AudioFilePath == "" {
		log.Printf("[context-audio] empty path context_id=%d", id)
		return "", "", errors.New("context audio path is empty")
	}
	return row.AudioFilePath, row.AudioContentType, nil
}
