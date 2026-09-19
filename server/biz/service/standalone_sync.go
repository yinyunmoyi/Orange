package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrStandaloneSyncInvalid  = errors.New("invalid standalone sync request")
	ErrStandaloneSyncConflict = errors.New("standalone sync client id conflict")
	standaloneSyncClientID    = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	standaloneSyncMu          sync.Mutex
)

func SyncStandaloneFavorite(
	ctx context.Context,
	req *model.StandaloneFavoriteSyncRequest,
) (*model.StandaloneFavoriteSyncData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	normalized, payloadHash, err := normalizeStandaloneSyncRequest(req)
	if err != nil {
		return nil, err
	}

	// Serialization keeps the existing favorite initialization routines from
	// racing while the database uniqueness constraints provide durable dedupe.
	standaloneSyncMu.Lock()
	defer standaloneSyncMu.Unlock()

	if data, found, err := loadStandaloneSyncReceipt(ctx, normalized.ClientID, payloadHash); err != nil {
		return nil, err
	} else if found {
		return data, nil
	}

	itemID, err := createStandaloneFavorite(ctx, normalized)
	if err != nil {
		return nil, err
	}
	receipt := db.StandaloneSyncReceipt{
		ClientID:    normalized.ClientID,
		PayloadHash: payloadHash,
		ItemType:    normalized.ItemType,
		ItemID:      itemID,
	}
	if err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoNothing: true,
	}).Create(&receipt).Error; err != nil {
		return nil, fmt.Errorf("create standalone sync receipt: %w", err)
	}
	data, found, err := loadStandaloneSyncReceipt(ctx, normalized.ClientID, payloadHash)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("standalone sync receipt missing after create")
	}
	data.Idempotent = receipt.ID == 0
	return data, nil
}

func normalizeStandaloneSyncRequest(
	req *model.StandaloneFavoriteSyncRequest,
) (*model.StandaloneFavoriteSyncRequest, string, error) {
	if req == nil || !standaloneSyncClientID.MatchString(req.ClientID) {
		return nil, "", ErrStandaloneSyncInvalid
	}
	itemType := strings.ToLower(strings.TrimSpace(req.ItemType))
	var text, translation string
	var err error
	switch itemType {
	case ContextItemWord:
		text = strings.ToLower(strings.TrimSpace(req.Text))
		if _, _, err = NormalizeNoteTarget(ContextItemWord, text); err != nil {
			return nil, "", ErrStandaloneSyncInvalid
		}
	case ContextItemPhrase:
		text, err = NormalizePhrase(req.Text)
		if err != nil {
			return nil, "", ErrStandaloneSyncInvalid
		}
	case "sentence":
		text, err = NormalizeFavoriteSentence(req.Text)
		translation = strings.TrimSpace(req.Translation)
		if err != nil || translation == "" {
			return nil, "", ErrStandaloneSyncInvalid
		}
	default:
		return nil, "", ErrStandaloneSyncInvalid
	}
	normalized := &model.StandaloneFavoriteSyncRequest{
		ClientID: req.ClientID, ItemType: itemType, Text: text, Translation: translation,
	}
	sum := sha256.Sum256([]byte(itemType + "\x00" + text + "\x00" + translation))
	return normalized, hex.EncodeToString(sum[:]), nil
}

func loadStandaloneSyncReceipt(
	ctx context.Context,
	clientID string,
	payloadHash string,
) (*model.StandaloneFavoriteSyncData, bool, error) {
	var receipt db.StandaloneSyncReceipt
	err := db.DB.WithContext(ctx).Where("client_id = ?", clientID).Take(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load standalone sync receipt: %w", err)
	}
	if receipt.PayloadHash != payloadHash {
		return nil, false, ErrStandaloneSyncConflict
	}
	return &model.StandaloneFavoriteSyncData{
		ClientID: clientID, ItemType: receipt.ItemType, ItemID: receipt.ItemID, Idempotent: true,
	}, true, nil
}

func createStandaloneFavorite(
	ctx context.Context,
	req *model.StandaloneFavoriteSyncRequest,
) (int64, error) {
	switch req.ItemType {
	case ContextItemWord:
		wordData, lookupErr := LookupWithFallback(req.Text)
		if lookupErr != nil && wordData == nil {
			return 0, fmt.Errorf("standalone word lookup: %w", lookupErr)
		}
		meaning, meaningErr := GetMeaningContext(ctx, req.Text)
		if meaningErr != nil && !errors.Is(meaningErr, ErrNotFound) &&
			!errors.Is(meaningErr, ErrLLMNotConfigured) {
			return 0, fmt.Errorf("standalone word meaning: %w", meaningErr)
		}
		itemID, _, err := AddFavorite(ctx, &model.FavoriteRequest{
			Word: req.Text, WordData: wordData, Meaning: meaning,
		})
		return itemID, err
	case ContextItemPhrase:
		meaning, err := GetChinesePhraseMeaningContext(ctx, req.Text)
		if err != nil {
			return 0, fmt.Errorf("standalone phrase meaning: %w", err)
		}
		return AddPhraseFavorite(ctx, &model.PhraseFavoriteRequest{
			Phrase: req.Text, Meaning: meaning,
		})
	case "sentence":
		data, err := SaveSentenceFavorite(ctx, &model.SentenceFavoriteRequest{
			Sentence: req.Text, Translation: req.Translation,
		})
		if err != nil {
			return 0, err
		}
		return data.ID, nil
	default:
		return 0, ErrStandaloneSyncInvalid
	}
}
