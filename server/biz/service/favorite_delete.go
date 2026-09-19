package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrFavoriteDeleteInvalid = errors.New("invalid favorite delete request")

// DeleteFavorite 删除收藏项及其所有关联数据。数据库提交后再清理音频文件。
func DeleteFavorite(ctx context.Context, itemType string, itemID int64) error {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if (itemType != ContextItemWord && itemType != ContextItemPhrase) || itemID <= 0 {
		return ErrFavoriteDeleteInvalid
	}
	if !db.Enabled() {
		return db.ErrDBDisabled
	}

	audioPaths := make(map[string]struct{})
	found := false
	itemText := ""
	itemLevel := 0
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		found, itemText, itemLevel, err = lockFavoriteForDelete(tx, itemType, itemID, audioPaths)
		if err != nil {
			log.Printf("[favorite-delete] lock favorite failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("lock favorite for delete: %w", err)
		}
		if !found {
			return nil
		}

		var contexts []db.Context
		if err := tx.Select("id", "audio_file_path").
			Where("item_type = ? AND item_id = ?", itemType, itemID).
			Find(&contexts).Error; err != nil {
			log.Printf("[favorite-delete] query context audio failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("query context audio paths: %w", err)
		}
		contextIDs := make([]int64, 0, len(contexts))
		for _, row := range contexts {
			contextIDs = append(contextIDs, row.ID)
			addAudioPath(audioPaths, row.AudioFilePath)
		}
		if len(contextIDs) > 0 {
			var videos []db.ContextVideo
			if err := tx.Select("file_path").
				Where("context_id IN ?", contextIDs).
				Find(&videos).Error; err != nil {
				log.Printf("[favorite-delete] query context videos failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
				return fmt.Errorf("query context video paths: %w", err)
			}
			for _, video := range videos {
				addAudioPath(audioPaths, video.FilePath)
			}
			if err := tx.Where("context_id IN ?", contextIDs).
				Delete(&db.ContextVideo{}).Error; err != nil {
				log.Printf("[favorite-delete] delete context videos failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
				return fmt.Errorf("delete favorite context videos: %w", err)
			}
		}

		learningType := learningItemType(itemType)
		var queues []db.UserLearningQueue
		if err := tx.Where("item_type = ? AND item_id = ?", learningType, itemID).
			Order("session_id, id").Find(&queues).Error; err != nil {
			log.Printf("[favorite-delete] query learning queues failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("query favorite learning queues: %w", err)
		}
		if err := adjustLearningSessionsForFavoriteDelete(tx, learningType, itemID, queues, time.Now()); err != nil {
			log.Printf("[favorite-delete] adjust learning sessions failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("adjust favorite learning sessions: %w", err)
		}
		if err := tx.Where("item_type = ? AND item_id = ?", learningType, itemID).
			Delete(&db.UserItemLearning{}).Error; err != nil {
			log.Printf("[favorite-delete] delete learning progress failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("delete favorite learning progress: %w", err)
		}
		if err := tx.Where("item_type = ? AND item_id = ?", itemType, itemID).
			Delete(&db.ContextTask{}).Error; err != nil {
			log.Printf("[favorite-delete] delete context tasks failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("delete favorite context tasks: %w", err)
		}
		if err := tx.Where("item_type = ? AND item_id = ?", itemType, itemID).
			Delete(&db.Context{}).Error; err != nil {
			log.Printf("[favorite-delete] delete contexts failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
			return fmt.Errorf("delete favorite contexts: %w", err)
		}

		switch itemType {
		case ContextItemWord:
			if err := deleteWordFavoriteRows(tx, itemID); err != nil {
				return err
			}
		case ContextItemPhrase:
			if err := deletePhraseFavoriteRows(tx, itemID); err != nil {
				return err
			}
		}
		var before, after *int
		if itemType == ContextItemWord {
			before, after = &itemLevel, &itemLevel
		}
		recordActionBestEffort(tx, actionEventParams{
			EventID: serverActionEventID(model.ActionFavoriteDeleted, itemType, itemID),
			ItemType: itemType, ItemID: itemID, ItemText: itemText,
			Action: model.ActionFavoriteDeleted, Source: model.ActionSourceSystem,
			LevelBefore: before, LevelAfter: after, OccurredAt: time.Now(),
		})
		return nil
	})
	if err != nil {
		log.Printf("[favorite-delete] transaction failed item_type=%q item_id=%d err=%v", itemType, itemID, err)
		return err
	}
	if !found {
		log.Printf("[favorite-delete] idempotent missing item_type=%q item_id=%d", itemType, itemID)
		return nil
	}
	for path := range audioPaths {
		if err := storage.Delete(path); err != nil {
			log.Printf("[favorite-delete] delete audio failed item_type=%q item_id=%d path=%q err=%v",
				itemType, itemID, path, err)
		}
	}
	log.Printf("[favorite-delete] committed item_type=%q item_id=%d audio_paths=%d", itemType, itemID, len(audioPaths))
	return nil
}

func lockFavoriteForDelete(tx *gorm.DB, itemType string, itemID int64, audioPaths map[string]struct{}) (bool, string, int, error) {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", itemID).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, "", 0, nil
			}
			return false, "", 0, err
		}
		var audios []db.WordAudio
		if err := tx.Select("file_path").Where("word_id = ?", itemID).Find(&audios).Error; err != nil {
			return false, "", 0, fmt.Errorf("query word audio paths: %w", err)
		}
		for _, audio := range audios {
			addAudioPath(audioPaths, audio.FilePath)
		}
		return true, word.Word, word.Level, nil
	case ContextItemPhrase:
		var phrase db.Phrase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", itemID).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, "", 0, nil
			}
			return false, "", 0, err
		}
		addAudioPath(audioPaths, phrase.AudioFilePath)
		return true, phrase.Phrase, 0, nil
	default:
		return false, "", 0, ErrFavoriteDeleteInvalid
	}
}

func adjustLearningSessionsForFavoriteDelete(
	tx *gorm.DB,
	itemType int,
	itemID int64,
	queues []db.UserLearningQueue,
	now time.Time,
) error {
	sessionIDs := make([]int64, 0, len(queues))
	seenSessions := make(map[int64]struct{}, len(queues))
	removedQueueIDs := make(map[int64]struct{}, len(queues))
	for _, queue := range queues {
		removedQueueIDs[queue.ID] = struct{}{}
		if _, ok := seenSessions[queue.SessionID]; !ok {
			seenSessions[queue.SessionID] = struct{}{}
			sessionIDs = append(sessionIDs, queue.SessionID)
		}
	}
	sort.Slice(sessionIDs, func(i, j int) bool { return sessionIDs[i] < sessionIDs[j] })

	var sessions []db.LearningSession
	if len(sessionIDs) > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", sessionIDs).Order("id").Find(&sessions).Error; err != nil {
			return fmt.Errorf("lock affected learning sessions: %w", err)
		}
	}
	if err := tx.Where("item_type = ? AND item_id = ?", itemType, itemID).
		Delete(&db.UserLearningQueue{}).Error; err != nil {
		return fmt.Errorf("delete favorite learning queues: %w", err)
	}

	for i := range sessions {
		session := &sessions[i]
		if session.CurrentQueueItemID == nil {
			continue
		}
		if _, removed := removedQueueIDs[*session.CurrentQueueItemID]; !removed {
			continue
		}
		if session.Status == sessionActive {
			if err := advanceLearningSession(tx, session, session.TurnNo, now); err != nil {
				return fmt.Errorf("advance active learning session %d: %w", session.ID, err)
			}
			continue
		}
		if err := tx.Model(&db.LearningSession{}).Where("id = ?", session.ID).
			Update("current_queue_item_id", nil).Error; err != nil {
			return fmt.Errorf("clear inactive learning session %d current queue: %w", session.ID, err)
		}
	}
	return nil
}

func deleteWordFavoriteRows(tx *gorm.DB, itemID int64) error {
	var memberships []db.WordGroupMember
	if err := tx.Where("word_id = ?", itemID).Find(&memberships).Error; err != nil {
		log.Printf("[favorite-delete] query word groups failed item_type=%q item_id=%d err=%v", ContextItemWord, itemID, err)
		return fmt.Errorf("query favorite word groups: %w", err)
	}
	if err := tx.Where("word_id = ?", itemID).Delete(&db.WordGroupMember{}).Error; err != nil {
		log.Printf("[favorite-delete] delete word group members failed item_type=%q item_id=%d err=%v", ContextItemWord, itemID, err)
		return fmt.Errorf("delete favorite word group members: %w", err)
	}
	groupIDs := make(map[int64]struct{}, len(memberships))
	for _, membership := range memberships {
		groupIDs[membership.GroupID] = struct{}{}
	}
	for groupID := range groupIDs {
		var count int64
		if err := tx.Model(&db.WordGroupMember{}).Where("group_id = ?", groupID).Count(&count).Error; err != nil {
			return fmt.Errorf("count favorite word group %d members: %w", groupID, err)
		}
		if count == 0 {
			if err := tx.Where("id = ?", groupID).Delete(&db.WordGroup{}).Error; err != nil {
				return fmt.Errorf("delete empty favorite word group %d: %w", groupID, err)
			}
		} else if err := recountGroupMembers(tx, groupID); err != nil {
			return fmt.Errorf("recount favorite word group %d: %w", groupID, err)
		}
	}
	if err := tx.Where("word_id = ?", itemID).Delete(&db.WordMeaning{}).Error; err != nil {
		return fmt.Errorf("delete favorite word meanings: %w", err)
	}
	if err := tx.Where("word_id = ?", itemID).Delete(&db.WordAudio{}).Error; err != nil {
		return fmt.Errorf("delete favorite word audios: %w", err)
	}
	if err := tx.Where("id = ?", itemID).Delete(&db.Word{}).Error; err != nil {
		return fmt.Errorf("delete favorite word: %w", err)
	}
	return nil
}

func deletePhraseFavoriteRows(tx *gorm.DB, itemID int64) error {
	if err := tx.Where("phrase_id = ?", itemID).Delete(&db.PhraseMeaning{}).Error; err != nil {
		return fmt.Errorf("delete favorite phrase meanings: %w", err)
	}
	if err := tx.Where("id = ?", itemID).Delete(&db.Phrase{}).Error; err != nil {
		return fmt.Errorf("delete favorite phrase: %w", err)
	}
	return nil
}

func learningItemType(itemType string) int {
	if itemType == ContextItemPhrase {
		return LearningItemPhrase
	}
	return LearningItemWord
}

func addAudioPath(paths map[string]struct{}, path string) {
	path = strings.TrimSpace(path)
	if path != "" {
		paths[path] = struct{}{}
	}
}
