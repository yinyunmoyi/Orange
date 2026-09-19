package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrFavoriteResetInvalid  = errors.New("invalid favorite reset request")
	ErrFavoriteResetNotFound = errors.New("favorite item not found")
)

// ResetFavoriteLearning 重置收藏项的学习进度：把 UserItemLearning 回到刚收藏的初始态，
// 清空该项目在所有 session 中的 UserLearningQueue，并同步维护受影响会话的 current_queue_item_id。
// 收藏关系本身、语境、备注等都不动。
func ResetFavoriteLearning(ctx context.Context, itemType string, itemID int64) error {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if (itemType != ContextItemWord && itemType != ContextItemPhrase) || itemID <= 0 {
		return ErrFavoriteResetInvalid
	}
	if !db.Enabled() {
		return db.ErrDBDisabled
	}

	learningType := learningItemType(itemType)
	now := time.Now()

	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		itemText, level, err := lockActionTarget(tx, itemType, itemID)
		if err != nil {
			log.Printf("[favorite-reset] lock favorite failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			if errors.Is(err, ErrItemActionNotFound) {
				return ErrFavoriteResetNotFound
			}
			return fmt.Errorf("lock favorite for reset: %w", err)
		}

		var queues []db.UserLearningQueue
		if err := tx.Where("item_type = ? AND item_id = ?", learningType, itemID).
			Order("session_id, id").Find(&queues).Error; err != nil {
			log.Printf("[favorite-reset] query learning queues failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("query favorite learning queues: %w", err)
		}
		if err := adjustLearningSessionsForFavoriteDelete(tx, learningType, itemID, queues, now); err != nil {
			log.Printf("[favorite-reset] adjust learning sessions failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("adjust favorite learning sessions: %w", err)
		}

		result := tx.Model(&db.UserItemLearning{}).
			Where("item_type = ? AND item_id = ?", learningType, itemID).
			Updates(map[string]any{
				"learning_status":     learningrule.StatusNotStarted,
				"memory_stage":        0,
				"review_success_days": 0,
				"last_reviewed_at":    nil,
				"next_review_at":      nil,
				"learned_at":          nil,
				"mastered_at":         nil,
				"queued_at":           now,
			})
		if result.Error != nil {
			log.Printf("[favorite-reset] update learning progress failed item_type=%q item_id=%d err=%v",
				itemType, itemID, result.Error)
			return fmt.Errorf("update favorite learning progress: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			row := newLearningRow(learningType, itemID, now)
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "item_type"}, {Name: "item_id"}},
				DoNothing: true,
			}).Create(&row).Error; err != nil {
				log.Printf("[favorite-reset] insert learning progress failed item_type=%q item_id=%d err=%v",
					itemType, itemID, err)
				return fmt.Errorf("insert favorite learning progress: %w", err)
			}
		}
		var before, after *int
		if itemType == ContextItemWord {
			before, after = &level, &level
		}
		recordActionBestEffort(tx, actionEventParams{
			EventID: serverActionEventID(model.ActionLearningReset, itemType, itemID, now.UnixNano()),
			ItemType: itemType, ItemID: itemID, ItemText: itemText,
			Action: model.ActionLearningReset, Source: model.ActionSourceSystem,
			LevelBefore: before, LevelAfter: after, OccurredAt: now,
		})
		return nil
	})
	if err != nil {
		return err
	}
	log.Printf("[favorite-reset] ok item_type=%q item_id=%d queued_at=%s", itemType, itemID, now.Format(time.RFC3339))
	return nil
}

func lockFavoriteExists(tx *gorm.DB, itemType string, itemID int64) (bool, error) {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", itemID).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	case ContextItemPhrase:
		var phrase db.Phrase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", itemID).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	default:
		return false, ErrFavoriteResetInvalid
	}
}
