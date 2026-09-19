package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"aaa_word/biz/db"
	learningrule "aaa_word/biz/learning"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	FavoriteBatchCreated  = "created"
	FavoriteBatchExisting = "existing"
	FavoriteBatchRepaired = "repaired"

	favoriteBatchMeaningTimeout = 30 * time.Second
)

type FavoriteBatchOptions struct {
	SleepBetweenCalls time.Duration
}

type FavoriteImportItem struct {
	Text string
	Note string
}

type FavoriteImportResult struct {
	ItemType string
	ItemID   int64
	Text     string
	Action   string
}

type FavoriteBatchError struct {
	Stage string
	Err   error
}

func (e *FavoriteBatchError) Error() string {
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e *FavoriteBatchError) Unwrap() error {
	return e.Err
}

func FavoriteBatchErrorStage(err error) string {
	var batchErr *FavoriteBatchError
	if errors.As(err, &batchErr) {
		return batchErr.Stage
	}
	return "unknown"
}

type FavoriteCleanupFailure struct {
	ItemType string
	ItemID   int64
	Text     string
	Stage    string
	Err      error
}

type FavoriteCleanupReport struct {
	Scanned          int
	Healthy          int
	AudioRepaired    int
	LearningInserted int64
	Failures         []FavoriteCleanupFailure
}

type favoriteBatchTarget struct {
	ItemType     string
	LearningType int
	ItemID       int64
	Text         string
	AudioPath    string
	AudioSize    int64
}

func NormalizeFavoriteImportText(raw string) (string, string, error) {
	normalized := strings.Join(strings.Fields(raw), " ")
	if normalized == "" {
		return "", "", ErrNoteInvalid
	}
	itemType := ContextItemWord
	if len(strings.Fields(normalized)) > 1 {
		itemType = ContextItemPhrase
	}
	return NormalizeNoteTarget(itemType, normalized)
}

func ImportFavoriteItem(
	ctx context.Context,
	item FavoriteImportItem,
	queuedAt time.Time,
	opts FavoriteBatchOptions,
) (*FavoriteImportResult, error) {
	if !db.Enabled() {
		return nil, batchError("favorite", db.ErrDBDisabled)
	}
	itemType, text, err := NormalizeFavoriteImportText(item.Text)
	if err != nil {
		return nil, batchError("validate", err)
	}

	target, found, err := findFavoriteBatchTarget(ctx, itemType, text)
	if err != nil {
		return nil, batchError("lookup", err)
	}
	action := FavoriteBatchExisting
	if found {
		healthy, _ := favoriteAudioHealthy(target.AudioPath, target.AudioSize)
		if !healthy {
			if _, err := RegenerateFavoriteAudio(ctx, itemType, target.ItemID); err != nil {
				return nil, batchError("audio", err)
			}
			action = FavoriteBatchRepaired
			sleepFavoriteBatch(opts.SleepBetweenCalls)
		}
	} else {
		target, err = createImportedFavorite(ctx, itemType, text, opts)
		if err != nil {
			return nil, err
		}
		action = FavoriteBatchCreated
	}

	if strings.TrimSpace(item.Note) != "" {
		if _, err := SaveNote(ctx, &model.NoteRequest{
			ItemType: itemType,
			Text:     text,
			Note:     item.Note,
		}); err != nil {
			return nil, batchError("note", err)
		}
	}
	if err := refreshImportedLearning(ctx, target.LearningType, target.ItemID, queuedAt); err != nil {
		return nil, batchError("learning", err)
	}
	return &FavoriteImportResult{
		ItemType: itemType,
		ItemID:   target.ItemID,
		Text:     text,
		Action:   action,
	}, nil
}

func createImportedFavorite(
	ctx context.Context,
	itemType string,
	text string,
	opts FavoriteBatchOptions,
) (*favoriteBatchTarget, error) {
	switch itemType {
	case ContextItemWord:
		wordData, err := LookupWithFallback(text)
		if err != nil && wordData == nil {
			return nil, batchError("lookup", err)
		}
		sleepFavoriteBatch(opts.SleepBetweenCalls)

		meaningCtx, cancelMeaning := context.WithTimeout(ctx, favoriteBatchMeaningTimeout)
		meaning, err := GetMeaningContext(meaningCtx, text)
		cancelMeaning()
		if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrLLMNotConfigured) {
			return nil, batchError("meaning", err)
		}
		sleepFavoriteBatch(opts.SleepBetweenCalls)

		itemID, _, err := AddFavorite(ctx, &model.FavoriteRequest{
			Word:     text,
			WordData: wordData,
			Meaning:  meaning,
		})
		if err != nil {
			return nil, batchError("favorite", err)
		}
		return &favoriteBatchTarget{
			ItemType: ContextItemWord, LearningType: LearningItemWord,
			ItemID: itemID, Text: text,
		}, nil

	case ContextItemPhrase:
		meaningCtx, cancelMeaning := context.WithTimeout(ctx, favoriteBatchMeaningTimeout)
		meaning, err := GetChinesePhraseMeaningContext(meaningCtx, text)
		cancelMeaning()
		if err != nil {
			return nil, batchError("meaning", err)
		}
		sleepFavoriteBatch(opts.SleepBetweenCalls)

		itemID, err := AddPhraseFavorite(ctx, &model.PhraseFavoriteRequest{
			Phrase:  text,
			Meaning: meaning,
		})
		if err != nil {
			return nil, batchError("favorite", err)
		}
		return &favoriteBatchTarget{
			ItemType: ContextItemPhrase, LearningType: LearningItemPhrase,
			ItemID: itemID, Text: text,
		}, nil

	default:
		return nil, batchError("validate", ErrNoteInvalid)
	}
}

func findFavoriteBatchTarget(ctx context.Context, itemType string, text string) (*favoriteBatchTarget, bool, error) {
	switch itemType {
	case ContextItemWord:
		var word db.Word
		if err := db.DB.WithContext(ctx).Where("word = ?", text).Take(&word).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, nil
			}
			log.Printf("[favorite-batch] query word failed text=%q err=%v", text, err)
			return nil, false, err
		}
		target := &favoriteBatchTarget{
			ItemType: ContextItemWord, LearningType: LearningItemWord,
			ItemID: word.ID, Text: word.Word,
		}
		var audio db.WordAudio
		err := db.DB.WithContext(ctx).
			Where("word_id = ? AND UPPER(accent) = ?", word.ID, "US").
			Order("id").Take(&audio).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[favorite-batch] query word audio failed item_id=%d text=%q err=%v", word.ID, word.Word, err)
			return nil, false, err
		}
		if err == nil {
			target.AudioPath = audio.FilePath
			target.AudioSize = audio.FileSize
		}
		return target, true, nil

	case ContextItemPhrase:
		var phrase db.Phrase
		if err := db.DB.WithContext(ctx).Where("phrase = ?", text).Take(&phrase).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, nil
			}
			log.Printf("[favorite-batch] query phrase failed text=%q err=%v", text, err)
			return nil, false, err
		}
		return &favoriteBatchTarget{
			ItemType: ContextItemPhrase, LearningType: LearningItemPhrase,
			ItemID: phrase.ID, Text: phrase.Phrase,
			AudioPath: phrase.AudioFilePath, AudioSize: phrase.AudioFileSize,
		}, true, nil
	default:
		return nil, false, ErrNoteInvalid
	}
}

func favoriteAudioHealthy(path string, recordedSize int64) (bool, string) {
	if strings.TrimSpace(path) == "" {
		return false, "audio path is empty"
	}
	if recordedSize <= 0 {
		return false, "audio size is empty"
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "audio file is missing"
		}
		return false, "audio file cannot be inspected"
	}
	if !info.Mode().IsRegular() {
		return false, "audio path is not a regular file"
	}
	if info.Size() <= 0 {
		return false, "audio file is empty"
	}
	return true, ""
}

func initializeItemLearning(tx *gorm.DB, itemType int, itemID int64, queuedAt time.Time) error {
	row := newLearningRow(itemType, itemID, queuedAt)
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "item_type"}, {Name: "item_id"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		log.Printf("[learning-enroll] create failed item_type=%d item_id=%d err=%v", itemType, itemID, err)
		return fmt.Errorf("initialize item learning: %w", err)
	}
	return nil
}

func refreshImportedLearning(ctx context.Context, itemType int, itemID int64, queuedAt time.Time) error {
	return db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&db.UserItemLearning{}).
			Where("item_type = ? AND item_id = ? AND learning_status = ?",
				itemType, itemID, learningrule.StatusNotStarted).
			Update("queued_at", queuedAt)
		if result.Error != nil {
			log.Printf("[learning-enroll] refresh failed item_type=%d item_id=%d err=%v", itemType, itemID, result.Error)
			return fmt.Errorf("refresh item learning: %w", result.Error)
		}
		return initializeItemLearning(tx, itemType, itemID, queuedAt)
	})
}

func newLearningRow(itemType int, itemID int64, queuedAt time.Time) db.UserItemLearning {
	return db.UserItemLearning{
		ItemType:          itemType,
		ItemID:            itemID,
		QueuedAt:          queuedAt,
		LearningStatus:    learningrule.StatusNotStarted,
		MemoryStage:       0,
		ReviewSuccessDays: 0,
		ScheduleVersion:   1,
	}
}

func CleanFavoriteCollection(ctx context.Context, opts FavoriteBatchOptions) (*FavoriteCleanupReport, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	targets, err := loadFavoriteCleanupTargets(ctx)
	if err != nil {
		return nil, err
	}
	learningKeys, err := loadFavoriteLearningKeys(ctx)
	if err != nil {
		return nil, err
	}

	report, pendingLearning := collectFavoriteCleanupChanges(
		ctx, targets, learningKeys, opts, RegenerateFavoriteAudio,
	)
	if len(pendingLearning) == 0 {
		return report, nil
	}
	result := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "item_type"}, {Name: "item_id"}},
		DoNothing: true,
	}).CreateInBatches(&pendingLearning, 500)
	if result.Error != nil {
		log.Printf("[favorite-clean] batch learning insert failed count=%d err=%v", len(pendingLearning), result.Error)
		return report, fmt.Errorf("batch initialize learning: %w", result.Error)
	}
	report.LearningInserted = result.RowsAffected
	return report, nil
}

type regenerateFavoriteAudioFunc func(
	ctx context.Context,
	itemType string,
	itemID int64,
) (*model.FavoriteAudioData, error)

func collectFavoriteCleanupChanges(
	ctx context.Context,
	targets []favoriteBatchTarget,
	learningKeys map[learningItemKey]struct{},
	opts FavoriteBatchOptions,
	regenerate regenerateFavoriteAudioFunc,
) (*FavoriteCleanupReport, []db.UserItemLearning) {
	report := &FavoriteCleanupReport{Failures: []FavoriteCleanupFailure{}}
	pendingLearning := make([]db.UserItemLearning, 0)
	for _, target := range targets {
		report.Scanned++
		healthy, reason := favoriteAudioHealthy(target.AudioPath, target.AudioSize)
		if healthy {
			report.Healthy++
		} else {
			if _, err := regenerate(ctx, target.ItemType, target.ItemID); err != nil {
				log.Printf("[favorite-clean] repair audio failed item_type=%q item_id=%d text=%q reason=%q err=%v",
					target.ItemType, target.ItemID, target.Text, reason, err)
				report.Failures = append(report.Failures, FavoriteCleanupFailure{
					ItemType: target.ItemType, ItemID: target.ItemID, Text: target.Text,
					Stage: "audio", Err: err,
				})
				continue
			}
			report.AudioRepaired++
			sleepFavoriteBatch(opts.SleepBetweenCalls)
		}

		key := learningKey(target.LearningType, target.ItemID)
		if _, exists := learningKeys[key]; !exists {
			pendingLearning = append(pendingLearning,
				newLearningRow(target.LearningType, target.ItemID, time.Now()))
		}
	}
	return report, pendingLearning
}

func loadFavoriteCleanupTargets(ctx context.Context) ([]favoriteBatchTarget, error) {
	var wordRows []struct {
		ItemID    int64  `gorm:"column:item_id"`
		Text      string `gorm:"column:text"`
		AudioPath string `gorm:"column:audio_path"`
		AudioSize int64  `gorm:"column:audio_size"`
	}
	if err := db.DB.WithContext(ctx).Table("words AS w").
		Select("w.id AS item_id, w.word AS text, COALESCE(a.file_path, '') AS audio_path, COALESCE(a.file_size, 0) AS audio_size").
		Joins("LEFT JOIN word_audios AS a ON a.word_id = w.id AND UPPER(a.accent) = ?", "US").
		Order("w.id").Scan(&wordRows).Error; err != nil {
		log.Printf("[favorite-clean] query words failed err=%v", err)
		return nil, fmt.Errorf("query cleanup words: %w", err)
	}

	var phrases []db.Phrase
	if err := db.DB.WithContext(ctx).Order("id").Find(&phrases).Error; err != nil {
		log.Printf("[favorite-clean] query phrases failed err=%v", err)
		return nil, fmt.Errorf("query cleanup phrases: %w", err)
	}

	targets := make([]favoriteBatchTarget, 0, len(wordRows)+len(phrases))
	for _, row := range wordRows {
		targets = append(targets, favoriteBatchTarget{
			ItemType: ContextItemWord, LearningType: LearningItemWord,
			ItemID: row.ItemID, Text: row.Text, AudioPath: row.AudioPath, AudioSize: row.AudioSize,
		})
	}
	for _, phrase := range phrases {
		targets = append(targets, favoriteBatchTarget{
			ItemType: ContextItemPhrase, LearningType: LearningItemPhrase,
			ItemID: phrase.ID, Text: phrase.Phrase,
			AudioPath: phrase.AudioFilePath, AudioSize: phrase.AudioFileSize,
		})
	}
	return targets, nil
}

func loadFavoriteLearningKeys(ctx context.Context) (map[learningItemKey]struct{}, error) {
	var rows []db.UserItemLearning
	if err := db.DB.WithContext(ctx).
		Select("item_type", "item_id").
		Where("item_type IN ?", []int{LearningItemWord, LearningItemPhrase}).
		Find(&rows).Error; err != nil {
		log.Printf("[favorite-clean] query learning keys failed err=%v", err)
		return nil, fmt.Errorf("query learning keys: %w", err)
	}
	keys := make(map[learningItemKey]struct{}, len(rows))
	for _, row := range rows {
		keys[learningKey(row.ItemType, row.ItemID)] = struct{}{}
	}
	return keys, nil
}

func batchError(stage string, err error) error {
	return &FavoriteBatchError{Stage: stage, Err: err}
}

func sleepFavoriteBatch(delay time.Duration) {
	if delay > 0 {
		time.Sleep(delay)
	}
}
