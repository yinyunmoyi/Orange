package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/storage"

	"gorm.io/gorm"
)

var (
	phrasePattern           = regexp.MustCompile(`^[A-Za-z][A-Za-z'-]*(?: [A-Za-z][A-Za-z'-]*){1,7}$`)
	brokenHyphenWrapPattern = regexp.MustCompile(`(?:^| )[A-Za-z]+-[A-Za-z] [A-Za-z]{2,}(?: |$)`)
	ErrPhraseInvalid        = errors.New("invalid phrase")
	ErrPhraseNotFound       = errors.New("phrase not found")
	phraseFavoriteMu        sync.Mutex
)

func NormalizePhrase(raw string) (string, error) {
	phrase := strings.ToLower(strings.Join(strings.Fields(raw), " "))
	if phrase == "" || len(phrase) > 80 ||
		!phrasePattern.MatchString(phrase) ||
		brokenHyphenWrapPattern.MatchString(phrase) {
		return "", ErrPhraseInvalid
	}
	return phrase, nil
}

func AddPhraseFavorite(ctx context.Context, req *model.PhraseFavoriteRequest) (int64, error) {
	if !db.Enabled() {
		log.Printf("[phrase-favorite] db disabled")
		return 0, db.ErrDBDisabled
	}
	if req == nil {
		log.Printf("[phrase-favorite] nil request")
		return 0, ErrPhraseInvalid
	}
	phrase, err := NormalizePhrase(req.Phrase)
	if err != nil {
		log.Printf("[phrase-favorite] invalid phrase raw=%q err=%v", req.Phrase, err)
		return 0, err
	}
	meanings, err := normalizePhraseMeanings(req.Meaning)
	if err != nil {
		log.Printf("[phrase-favorite] invalid meanings phrase=%q err=%v", phrase, err)
		return 0, err
	}

	// 单用户应用中收藏请求并发度很低；串行化可避免相同稳定音频路径在并发事务回滚时被误删。
	phraseFavoriteMu.Lock()
	defer phraseFavoriteMu.Unlock()

	if existing, err := findPhraseByText(ctx, phrase); err == nil {
		if err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return initializePhraseLearning(tx, existing.ID, time.Now())
		}); err != nil {
			log.Printf("[phrase-favorite] ensure learning failed phrase=%q phrase_id=%d err=%v", phrase, existing.ID, err)
			return 0, fmt.Errorf("initialize phrase learning: %w", err)
		}
		return existing.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("[phrase-favorite] lookup failed phrase=%q err=%v", phrase, err)
		return 0, fmt.Errorf("lookup phrase: %w", err)
	}

	audioBytes, contentType, err := SynthesizeMP3(phrase, "us")
	if err != nil {
		log.Printf("[phrase-favorite] synth failed phrase=%q err=%v", phrase, err)
		return 0, fmt.Errorf("synthesize phrase audio: %w", err)
	}
	audioPath, audioSize, err := storage.SavePhraseAudio(phrase, bytes.NewReader(audioBytes))
	if err != nil {
		log.Printf("[phrase-favorite] save audio failed phrase=%q err=%v", phrase, err)
		return 0, fmt.Errorf("save phrase audio: %w", err)
	}

	var phraseID int64
	err = db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := db.Phrase{
			Phrase:           phrase,
			AudioFilePath:    audioPath,
			AudioContentType: contentType,
			AudioFileSize:    audioSize,
		}
		if err := tx.Create(&row).Error; err != nil {
			log.Printf("[phrase-favorite] insert phrase failed phrase=%q err=%v", phrase, err)
			return fmt.Errorf("insert phrase: %w", err)
		}
		phraseID = row.ID

		rows := make([]db.PhraseMeaning, 0, len(meanings))
		for i, meaning := range meanings {
			rows = append(rows, db.PhraseMeaning{
				PhraseID:     phraseID,
				PartOfSpeech: meaning.PartOfSpeech,
				Definition:   meaning.Meaning,
				SortOrder:    i,
			})
		}
		if err := tx.Create(&rows).Error; err != nil {
			log.Printf("[phrase-favorite] insert meanings failed phrase=%q phrase_id=%d count=%d err=%v",
				phrase, phraseID, len(rows), err)
			return fmt.Errorf("insert phrase meanings: %w", err)
		}
		if err := initializePhraseLearning(tx, phraseID, time.Now()); err != nil {
			log.Printf("[phrase-favorite] initialize learning failed phrase=%q phrase_id=%d err=%v", phrase, phraseID, err)
			return err
		}
		recordActionBestEffort(tx, actionEventParams{
			EventID: serverActionEventID(model.ActionFavoriteCreated, ContextItemPhrase, phraseID),
			ItemType: ContextItemPhrase, ItemID: phraseID, ItemText: phrase,
			Action: model.ActionFavoriteCreated, Source: model.ActionSourceSystem,
			OccurredAt: time.Now(),
		})
		return nil
	})
	if err != nil {
		storage.Delete(audioPath)
		log.Printf("[phrase-favorite] transaction rollback phrase=%q path=%q err=%v", phrase, audioPath, err)
		return 0, err
	}
	log.Printf("[phrase-favorite] committed phrase=%q phrase_id=%d audio_size=%d", phrase, phraseID, audioSize)
	return phraseID, nil
}

func normalizePhraseMeanings(data *model.MeaningData) ([]model.ChineseMeaning, error) {
	if data == nil {
		return nil, fmt.Errorf("%w: phrase meaning is required", ErrPhraseInvalid)
	}
	result := make([]model.ChineseMeaning, 0, len(data.Meanings))
	for _, meaning := range data.Meanings {
		text := strings.TrimSpace(meaning.Meaning)
		if text == "" {
			continue
		}
		result = append(result, model.ChineseMeaning{
			PartOfSpeech: strings.TrimSpace(meaning.PartOfSpeech),
			Meaning:      text,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: phrase meaning is required", ErrPhraseInvalid)
	}
	return result, nil
}

func initializePhraseLearning(tx *gorm.DB, phraseID int64, queuedAt time.Time) error {
	return initializeItemLearning(tx, LearningItemPhrase, phraseID, queuedAt)
}

func IsPhraseFavorited(ctx context.Context, raw string) (bool, int64, error) {
	if !db.Enabled() {
		return false, 0, db.ErrDBDisabled
	}
	phrase, err := NormalizePhrase(raw)
	if err != nil {
		return false, 0, err
	}
	var row db.Phrase
	if err := db.DB.WithContext(ctx).Select("id").
		Where("phrase = ?", phrase).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, 0, nil
		}
		log.Printf("[phrase-favorite-status] query failed phrase=%q err=%v", phrase, err)
		return false, 0, err
	}
	return true, row.ID, nil
}

func GetPhraseDetail(ctx context.Context, id int64) (*model.PhraseDetailData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	var phrase db.Phrase
	if err := db.DB.WithContext(ctx).Where("id = ?", id).Take(&phrase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPhraseNotFound
		}
		log.Printf("[phrase-detail] query phrase failed phrase_id=%d err=%v", id, err)
		return nil, err
	}
	var rows []db.PhraseMeaning
	if err := db.DB.WithContext(ctx).Where("phrase_id = ?", id).Order("sort_order, id").Find(&rows).Error; err != nil {
		log.Printf("[phrase-detail] query meanings failed phrase_id=%d err=%v", id, err)
		return nil, err
	}
	data := &model.PhraseDetailData{
		ItemType: ContextItemPhrase,
		ItemID:   phrase.ID,
		Phrase:   phrase.Phrase,
		AudioURL: fmt.Sprintf("/api/v1/phrase/%d/audio", phrase.ID),
		Meanings: make([]model.ChineseMeaning, 0, len(rows)),
		Contexts: []model.Context{},
	}
	for _, row := range rows {
		data.Meanings = append(data.Meanings, model.ChineseMeaning{
			PartOfSpeech: row.PartOfSpeech,
			Meaning:      row.Definition,
		})
	}
	contexts, err := ListContexts(ctx, ContextItemPhrase, id)
	if err == nil {
		data.Contexts = contexts.Contexts
	} else if !errors.Is(err, ErrNotFound) {
		log.Printf("[phrase-detail] query contexts failed phrase_id=%d err=%v", id, err)
		return nil, err
	}
	return data, nil
}

func LookupPhraseAudio(ctx context.Context, id int64) (string, string, error) {
	if !db.Enabled() {
		return "", "", db.ErrDBDisabled
	}
	var phrase db.Phrase
	if err := db.DB.WithContext(ctx).Select("audio_file_path", "audio_content_type").
		Where("id = ?", id).Take(&phrase).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[phrase-audio] query failed phrase_id=%d err=%v", id, err)
		}
		return "", "", err
	}
	if phrase.AudioFilePath == "" {
		log.Printf("[phrase-audio] empty path phrase_id=%d", id)
		return "", "", ErrPhraseNotFound
	}
	return phrase.AudioFilePath, phrase.AudioContentType, nil
}

func findPhraseByText(ctx context.Context, phrase string) (*db.Phrase, error) {
	var row db.Phrase
	if err := db.DB.WithContext(ctx).Where("phrase = ?", phrase).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
