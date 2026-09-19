package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxActionMetadataBytes = 4096

var (
	ErrItemActionInvalid  = errors.New("invalid item action")
	ErrItemActionNotFound = errors.New("favorite item not found")
	ErrItemActionConflict = errors.New("event id already belongs to another action")
)

type actionEventParams struct {
	EventID     string
	ItemType    string
	ItemID      int64
	ItemText    string
	Action      string
	Source      string
	LevelBefore *int
	LevelAfter  *int
	SessionID   *int64
	QueueItemID *int64
	ContextID   *int64
	Metadata    *string
	OccurredAt  time.Time
}

func RecordClientItemAction(
	ctx context.Context,
	itemType string,
	itemID int64,
	req *model.ItemActionRequest,
) (*model.ItemActionData, error) {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if !validClientActionRequest(itemType, itemID, req) {
		return nil, ErrItemActionInvalid
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	metadata := normalizeActionMetadata(req.Metadata)
	if metadata == nil && len(req.Metadata) > 0 {
		return nil, ErrItemActionInvalid
	}

	data := &model.ItemActionData{ItemType: itemType, ItemID: itemID}
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		itemText, level, err := lockActionTarget(tx, itemType, itemID)
		if err != nil {
			return err
		}
		data.Level = level

		var existing db.ItemActionEvent
		err = tx.Select("item_type", "item_id", "action", "source", "level_after").
			Where("event_id = ?", req.EventID).Take(&existing).Error
		if err == nil {
			if existing.ItemType != itemType || existing.ItemID != itemID ||
				existing.Action != req.Action || existing.Source != req.Source {
				return ErrItemActionConflict
			}
			if existing.LevelAfter != nil {
				data.Level = *existing.LevelAfter
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var before, after *int
		if itemType == ContextItemWord && req.Action == model.ActionLookupOpened {
			current, next, err := incrementWordLevel(tx, itemID)
			if err != nil {
				return err
			}
			before, after = &current, &next
			data.Level = next
		}
		event := db.ItemActionEvent{
			EventID: req.EventID, ItemType: itemType, ItemID: itemID, ItemText: itemText,
			Action: req.Action, Source: req.Source, LevelBefore: before, LevelAfter: after,
			Metadata: metadata, OccurredAt: time.Now(),
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("create item action: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func validClientActionRequest(itemType string, itemID int64, req *model.ItemActionRequest) bool {
	if req == nil || itemID <= 0 || (itemType != ContextItemWord && itemType != ContextItemPhrase) {
		return false
	}
	if len(req.EventID) < 8 || len(req.EventID) > 64 || strings.TrimSpace(req.EventID) != req.EventID {
		return false
	}
	switch req.Action {
	case model.ActionLookupOpened, model.ActionDetailOpened, model.ActionAudioPlayed:
	default:
		return false
	}
	switch req.Source {
	case model.ActionSourceAndroidReader, model.ActionSourceAndroidWordDetail,
		model.ActionSourceAndroidLearning, model.ActionSourceWebVideo,
		model.ActionSourceWebWordDetail:
		return true
	default:
		return false
	}
}

func normalizeActionMetadata(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if len(raw) > maxActionMetadataBytes || !json.Valid(raw) {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	text := string(normalized)
	return &text
}

func lockActionTarget(tx *gorm.DB, itemType string, itemID int64) (string, int, error) {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", itemID).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", 0, ErrItemActionNotFound
			}
			return "", 0, err
		}
		return word.Word, word.Level, nil
	case ContextItemPhrase:
		var phrase db.Phrase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", itemID).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", 0, ErrItemActionNotFound
			}
			return "", 0, err
		}
		return phrase.Phrase, 0, nil
	default:
		return "", 0, ErrItemActionInvalid
	}
}

func incrementWordLevel(tx *gorm.DB, itemID int64) (int, int, error) {
	var word db.Word
	if err := tx.Select("id", "level").Where("id = ?", itemID).Take(&word).Error; err != nil {
		return 0, 0, err
	}
	before := word.Level
	if before < 1 {
		before = 1
	}
	after := before + 1
	if err := tx.Model(&db.Word{}).Where("id = ?", itemID).Update("level", after).Error; err != nil {
		return 0, 0, err
	}
	return before, after, nil
}

func serverActionEventID(action, itemType string, itemID int64, parts ...any) string {
	var raw strings.Builder
	fmt.Fprintf(&raw, "%s\x00%s\x00%d", action, itemType, itemID)
	for _, part := range parts {
		fmt.Fprintf(&raw, "\x00%v", part)
	}
	sum := sha256.Sum256([]byte(raw.String()))
	return "srv_" + hex.EncodeToString(sum[:])[:60]
}

func recordActionBestEffort(tx *gorm.DB, params actionEventParams) {
	if params.OccurredAt.IsZero() {
		params.OccurredAt = time.Now()
	}
	event := db.ItemActionEvent{
		EventID: params.EventID, ItemType: params.ItemType, ItemID: params.ItemID,
		ItemText: params.ItemText, Action: params.Action, Source: params.Source,
		LevelBefore: params.LevelBefore, LevelAfter: params.LevelAfter,
		SessionID: params.SessionID, QueueItemID: params.QueueItemID,
		ContextID: params.ContextID, Metadata: params.Metadata, OccurredAt: params.OccurredAt,
	}
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}).Create(&event)
	if result.Error != nil {
		log.Printf("[item-action] best-effort insert failed action=%q item_type=%q item_id=%d event_id=%q err=%v",
			params.Action, params.ItemType, params.ItemID, params.EventID, result.Error)
	}
}

func recordServerActionBestEffort(ctx context.Context, params actionEventParams) {
	if err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		recordActionBestEffort(tx, params)
		return nil
	}); err != nil {
		log.Printf("[item-action] best-effort transaction failed action=%q item_type=%q item_id=%d err=%v",
			params.Action, params.ItemType, params.ItemID, err)
	}
}

func recordLearningAnswerAction(
	tx *gorm.DB,
	queue *db.UserLearningQueue,
	sessionID int64,
	turnNo int,
	known bool,
	now time.Time,
) error {
	itemType := ContextItemWord
	if queue.ItemType == LearningItemPhrase {
		itemType = ContextItemPhrase
	}
	action := model.ActionLearningKnown
	if !known {
		action = model.ActionLearningUnknown
	}

	itemText, level, err := lockActionTarget(tx, itemType, queue.ItemID)
	if err != nil {
		if itemType == ContextItemWord && !known {
			return err
		}
		log.Printf("[item-action] learning snapshot skipped action=%q item_type=%q item_id=%d err=%v",
			action, itemType, queue.ItemID, err)
		return nil
	}
	var before, after *int
	if itemType == ContextItemWord {
		current := level
		next := current
		if !known {
			var incrementErr error
			current, next, incrementErr = incrementWordLevel(tx, queue.ItemID)
			if incrementErr != nil {
				return incrementErr
			}
		}
		before, after = &current, &next
	}
	recordActionBestEffort(tx, actionEventParams{
		EventID:  serverActionEventID(action, itemType, queue.ItemID, sessionID, queue.ID, turnNo),
		ItemType: itemType, ItemID: queue.ItemID, ItemText: itemText,
		Action: action, Source: model.ActionSourceAndroidLearning,
		LevelBefore: before, LevelAfter: after, SessionID: &sessionID,
		QueueItemID: &queue.ID, OccurredAt: now,
	})
	return nil
}

func recordLearningMasteryAction(
	tx *gorm.DB,
	queue *db.UserLearningQueue,
	sessionID int64,
	turnNo int,
	now time.Time,
) {
	itemType := ContextItemWord
	if queue.ItemType == LearningItemPhrase {
		itemType = ContextItemPhrase
	}
	itemText, level, err := lockActionTarget(tx, itemType, queue.ItemID)
	if err != nil {
		log.Printf("[item-action] mastery snapshot skipped item_type=%q item_id=%d err=%v",
			itemType, queue.ItemID, err)
		return
	}
	var before, after *int
	if itemType == ContextItemWord {
		before, after = &level, &level
	}
	recordActionBestEffort(tx, actionEventParams{
		EventID:  serverActionEventID(model.ActionLearningMastered, itemType, queue.ItemID, sessionID, queue.ID, turnNo),
		ItemType: itemType, ItemID: queue.ItemID, ItemText: itemText,
		Action: model.ActionLearningMastered, Source: model.ActionSourceAndroidLearning,
		LevelBefore: before, LevelAfter: after, SessionID: &sessionID,
		QueueItemID: &queue.ID, OccurredAt: now,
	})
}

func recordContextSavedBestEffort(
	ctx context.Context,
	itemType string,
	itemID int64,
	itemText string,
	source string,
	contextID int64,
) {
	recordServerActionBestEffort(ctx, actionEventParams{
		EventID:  serverActionEventID(model.ActionContextSaved, itemType, itemID, contextID),
		ItemType: itemType, ItemID: itemID, ItemText: itemText,
		Action: model.ActionContextSaved, Source: source,
		ContextID: &contextID, OccurredAt: time.Now(),
	})
}
