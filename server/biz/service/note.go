package service

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxNoteLength = 2000

var (
	ErrNoteInvalid  = errors.New("invalid note request")
	ErrNoteTooLong  = errors.New("note is too long")
	noteWordPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z'-]*$`)
)

func NormalizeNoteTarget(itemType, text string) (string, string, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(itemType))
	switch normalizedType {
	case ContextItemWord:
		normalizedText := strings.ToLower(strings.TrimSpace(text))
		if normalizedText == "" || len(normalizedText) > 64 || !noteWordPattern.MatchString(normalizedText) {
			return "", "", ErrNoteInvalid
		}
		return normalizedType, normalizedText, nil
	case ContextItemPhrase:
		normalizedText, err := NormalizePhrase(text)
		if err != nil {
			return "", "", ErrNoteInvalid
		}
		return normalizedType, normalizedText, nil
	default:
		return "", "", ErrNoteInvalid
	}
}

func validateNoteContent(note string) error {
	if utf8.RuneCountInString(note) > MaxNoteLength {
		return ErrNoteTooLong
	}
	return nil
}

func GetNote(ctx context.Context, itemType, text string) (*model.NoteData, error) {
	normalizedType, normalizedText, err := NormalizeNoteTarget(itemType, text)
	if err != nil {
		log.Printf("[note-get] invalid target item_type=%q text=%q err=%v", itemType, text, err)
		return nil, err
	}
	if !db.Enabled() {
		log.Printf("[note-get] db disabled item_type=%q text=%q", normalizedType, normalizedText)
		return nil, db.ErrDBDisabled
	}

	var row db.ItemNote
	if err := db.DB.WithContext(ctx).
		Where("item_type = ? AND item_text = ?", normalizedType, normalizedText).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.NoteData{ItemType: normalizedType, Text: normalizedText}, nil
		}
		log.Printf("[note-get] query failed item_type=%q text=%q err=%v", normalizedType, normalizedText, err)
		return nil, err
	}
	return noteDataFromRow(row), nil
}

func SaveNote(ctx context.Context, req *model.NoteRequest) (*model.NoteData, error) {
	if req == nil {
		return nil, ErrNoteInvalid
	}
	normalizedType, normalizedText, err := NormalizeNoteTarget(req.ItemType, req.Text)
	if err != nil {
		log.Printf("[note-save] invalid target item_type=%q text=%q err=%v", req.ItemType, req.Text, err)
		return nil, err
	}
	noteLength := utf8.RuneCountInString(req.Note)
	if err := validateNoteContent(req.Note); err != nil {
		log.Printf("[note-save] note too long item_type=%q text=%q note_length=%d", normalizedType, normalizedText, noteLength)
		return nil, err
	}
	if !db.Enabled() {
		log.Printf("[note-save] db disabled item_type=%q text=%q note_length=%d", normalizedType, normalizedText, noteLength)
		return nil, db.ErrDBDisabled
	}

	if strings.TrimSpace(req.Note) == "" {
		if err := db.DB.WithContext(ctx).
			Where("item_type = ? AND item_text = ?", normalizedType, normalizedText).
			Delete(&db.ItemNote{}).Error; err != nil {
			log.Printf("[note-save] clear failed item_type=%q text=%q err=%v", normalizedType, normalizedText, err)
			return nil, err
		}
		return &model.NoteData{ItemType: normalizedType, Text: normalizedText}, nil
	}

	now := time.Now()
	row := db.ItemNote{
		ItemType:  normalizedType,
		ItemText:  normalizedText,
		Note:      req.Note,
		UpdatedAt: now,
	}
	if err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "item_type"}, {Name: "item_text"}},
		DoUpdates: clause.Assignments(map[string]any{
			"note":       req.Note,
			"updated_at": now,
		}),
	}).Create(&row).Error; err != nil {
		log.Printf("[note-save] upsert failed item_type=%q text=%q note_length=%d err=%v",
			normalizedType, normalizedText, noteLength, err)
		return nil, err
	}
	row.UpdatedAt = now
	return noteDataFromRow(row), nil
}

func noteDataFromRow(row db.ItemNote) *model.NoteData {
	data := &model.NoteData{
		ItemType: row.ItemType,
		Text:     row.ItemText,
		Note:     row.Note,
	}
	if !row.UpdatedAt.IsZero() {
		data.UpdatedAt = row.UpdatedAt.Format(time.RFC3339)
	}
	return data
}
