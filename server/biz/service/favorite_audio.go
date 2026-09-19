package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"aaa_word/biz/converter"
	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrFavoriteAudioInvalid  = errors.New("invalid favorite audio request")
	ErrFavoriteAudioNotFound = errors.New("favorite audio item not found")
	favoriteAudioMu          sync.Mutex
)

type favoriteAudioSynthFunc func(text, accent string) ([]byte, string, error)

func RegenerateFavoriteAudio(
	ctx context.Context,
	itemType string,
	itemID int64,
) (*model.FavoriteAudioData, error) {
	if err := validateFavoriteAudioRequest(itemType, itemID); err != nil {
		return nil, err
	}
	if !db.Enabled() {
		log.Printf("[favorite-audio] db disabled item_type=%q item_id=%d", itemType, itemID)
		return nil, db.ErrDBDisabled
	}

	favoriteAudioMu.Lock()
	defer favoriteAudioMu.Unlock()

	return regenerateFavoriteAudio(ctx, itemType, itemID, newFavoriteAudioVersion(), SynthesizeMP3)
}

func regenerateFavoriteAudio(
	ctx context.Context,
	itemType string,
	itemID int64,
	version string,
	synthesize favoriteAudioSynthFunc,
) (*model.FavoriteAudioData, error) {
	if err := validateFavoriteAudioRequest(itemType, itemID); err != nil {
		return nil, err
	}
	if !db.Enabled() {
		log.Printf("[favorite-audio] db disabled item_type=%q item_id=%d", itemType, itemID)
		return nil, db.ErrDBDisabled
	}

	text, oldPath, err := loadFavoriteAudioTarget(ctx, itemType, itemID)
	if err != nil {
		return nil, err
	}
	audioBytes, contentType, err := synthesize(text, ttsAccentUS)
	if err != nil {
		log.Printf("[favorite-audio] synth failed item_type=%q item_id=%d text=%q err=%v",
			itemType, itemID, text, err)
		return nil, fmt.Errorf("synthesize favorite audio: %w", err)
	}
	newPath, audioSize, err := storage.SaveFavoriteAudioVersion(
		itemType,
		itemID,
		version,
		bytes.NewReader(audioBytes),
	)
	if err != nil {
		log.Printf("[favorite-audio] save failed item_type=%q item_id=%d version=%q err=%v",
			itemType, itemID, version, err)
		return nil, fmt.Errorf("save favorite audio: %w", err)
	}

	sourceURL := BuildTTSSrc(text, ttsAccentUS)
	err = db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		switch itemType {
		case ContextItemWord:
			return switchWordAudio(tx, itemID, sourceURL, newPath, contentType, audioSize)
		case ContextItemPhrase:
			return switchPhraseAudio(tx, itemID, newPath, contentType, audioSize)
		default:
			return ErrFavoriteAudioInvalid
		}
	})
	if err != nil {
		if cleanupErr := storage.Delete(newPath); cleanupErr != nil {
			log.Printf("[favorite-audio] cleanup new file failed item_type=%q item_id=%d path=%q err=%v",
				itemType, itemID, newPath, cleanupErr)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrFavoriteAudioNotFound
		}
		log.Printf("[favorite-audio] switch failed item_type=%q item_id=%d old_path=%q new_path=%q err=%v",
			itemType, itemID, oldPath, newPath, err)
		return nil, fmt.Errorf("switch favorite audio: %w", err)
	}
	recordServerActionBestEffort(ctx, actionEventParams{
		EventID: serverActionEventID(model.ActionAudioRegenerated, itemType, itemID, version),
		ItemType: itemType, ItemID: itemID, ItemText: text,
		Action: model.ActionAudioRegenerated, Source: model.ActionSourceSystem,
		OccurredAt: time.Now(),
	})

	if oldPath != "" && oldPath != newPath {
		if deleteErr := storage.Delete(oldPath); deleteErr != nil {
			log.Printf("[favorite-audio] delete old file failed item_type=%q item_id=%d path=%q err=%v",
				itemType, itemID, oldPath, deleteErr)
		}
	}

	audioURL := fmt.Sprintf("/api/v1/phrase/%d/audio?v=%s", itemID, url.QueryEscape(version))
	if itemType == ContextItemWord {
		audioURL = converter.AudioProxyPath + "?src=" + url.QueryEscape(sourceURL) +
			"&v=" + url.QueryEscape(version)
	}
	log.Printf("[favorite-audio] committed item_type=%q item_id=%d old_path=%q new_path=%q size=%d version=%q",
		itemType, itemID, oldPath, newPath, audioSize, version)
	return &model.FavoriteAudioData{AudioURL: audioURL}, nil
}

func validateFavoriteAudioRequest(itemType string, itemID int64) error {
	if itemID <= 0 || (itemType != ContextItemWord && itemType != ContextItemPhrase) {
		return ErrFavoriteAudioInvalid
	}
	return nil
}

func loadFavoriteAudioTarget(ctx context.Context, itemType string, itemID int64) (text, oldPath string, err error) {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err = db.DB.WithContext(ctx).Where("id = ?", itemID).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", "", ErrFavoriteAudioNotFound
			}
			log.Printf("[favorite-audio] query word failed item_id=%d err=%v", itemID, err)
			return "", "", fmt.Errorf("query favorite word: %w", err)
		}
		var audios []db.WordAudio
		if err = db.DB.WithContext(ctx).
			Where("word_id = ? AND UPPER(accent) = ?", itemID, "US").
			Limit(1).
			Find(&audios).Error; err != nil {
			log.Printf("[favorite-audio] query word audio failed item_id=%d err=%v", itemID, err)
			return "", "", fmt.Errorf("query favorite word audio: %w", err)
		}
		if len(audios) > 0 {
			oldPath = audios[0].FilePath
		}
		return word.Word, oldPath, nil
	case ContextItemPhrase:
		var phrase db.Phrase
		if err = db.DB.WithContext(ctx).Where("id = ?", itemID).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", "", ErrFavoriteAudioNotFound
			}
			log.Printf("[favorite-audio] query phrase failed item_id=%d err=%v", itemID, err)
			return "", "", fmt.Errorf("query favorite phrase: %w", err)
		}
		return phrase.Phrase, phrase.AudioFilePath, nil
	default:
		return "", "", ErrFavoriteAudioInvalid
	}
}

func switchWordAudio(
	tx *gorm.DB,
	itemID int64,
	sourceURL string,
	newPath string,
	contentType string,
	audioSize int64,
) error {
	var word db.Word
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", itemID).
		Take(&word).Error; err != nil {
		log.Printf("[favorite-audio] lock word failed item_id=%d err=%v", itemID, err)
		return err
	}

	var audios []db.WordAudio
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("word_id = ? AND UPPER(accent) = ?", itemID, "US").
		Limit(1).
		Find(&audios).Error; err != nil {
		log.Printf("[favorite-audio] lock word audio failed item_id=%d err=%v", itemID, err)
		return err
	}
	if len(audios) == 0 {
		row := db.WordAudio{
			WordID:      itemID,
			Accent:      "US",
			SourceURL:   sourceURL,
			FilePath:    newPath,
			ContentType: contentType,
			FileSize:    audioSize,
		}
		if err := tx.Create(&row).Error; err != nil {
			log.Printf("[favorite-audio] insert word audio failed item_id=%d err=%v", itemID, err)
			return fmt.Errorf("insert word audio: %w", err)
		}
		return nil
	}

	if err := tx.Model(&db.WordAudio{}).
		Where("id = ?", audios[0].ID).
		Updates(map[string]any{
			"source_url":   sourceURL,
			"file_path":    newPath,
			"content_type": contentType,
			"file_size":    audioSize,
		}).Error; err != nil {
		log.Printf("[favorite-audio] update word audio failed item_id=%d audio_id=%d err=%v",
			itemID, audios[0].ID, err)
		return fmt.Errorf("update word audio: %w", err)
	}
	return nil
}

func switchPhraseAudio(
	tx *gorm.DB,
	itemID int64,
	newPath string,
	contentType string,
	audioSize int64,
) error {
	var phrase db.Phrase
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", itemID).
		Take(&phrase).Error; err != nil {
		log.Printf("[favorite-audio] lock phrase failed item_id=%d err=%v", itemID, err)
		return err
	}
	if err := tx.Model(&db.Phrase{}).
		Where("id = ?", itemID).
		Updates(map[string]any{
			"audio_file_path":    newPath,
			"audio_content_type": contentType,
			"audio_file_size":    audioSize,
			"updated_at":         time.Now(),
		}).Error; err != nil {
		log.Printf("[favorite-audio] update phrase failed item_id=%d err=%v", itemID, err)
		return fmt.Errorf("update phrase audio: %w", err)
	}
	return nil
}

func newFavoriteAudioVersion() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
