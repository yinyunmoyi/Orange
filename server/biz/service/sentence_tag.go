package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MaxSentenceTagNameLength      = 24
	MaxSentenceFavoriteNoteLength = 300
	MaxSentenceTagsPerItem        = 20
)

var (
	ErrSentenceTagInvalid   = errors.New("invalid sentence tag request")
	ErrSentenceTagConflict  = errors.New("sentence tag already exists")
	ErrSentenceTagNotFound  = errors.New("sentence tag not found")
	ErrSentenceTagDuplicate = errors.New("duplicate sentence tag assignment")
)

var SentenceTagColors = []string{
	"#D32F2F",
	"#E65100",
	"#F9A825",
	"#2E7D32",
	"#00897B",
	"#1565C0",
	"#5E35B1",
	"#C2185B",
}

var sentenceTagColorSet = func() map[string]struct{} {
	result := make(map[string]struct{}, len(SentenceTagColors))
	for _, color := range SentenceTagColors {
		result[color] = struct{}{}
	}
	return result
}()

func NormalizeSentenceTagName(name string) (string, error) {
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrSentenceTagInvalid
		}
	}
	normalized := strings.Join(strings.Fields(name), " ")
	if normalized == "" || utf8.RuneCountInString(normalized) > MaxSentenceTagNameLength {
		return "", ErrSentenceTagInvalid
	}
	return normalized, nil
}

func NormalizeSentenceFavoriteNote(note string) (string, error) {
	for _, r := range note {
		if unicode.IsControl(r) && r != '\n' && r != '\r' {
			return "", ErrSentenceTagInvalid
		}
	}
	normalized := strings.TrimSpace(
		strings.ReplaceAll(strings.ReplaceAll(note, "\r\n", "\n"), "\r", "\n"),
	)
	if utf8.RuneCountInString(normalized) > MaxSentenceFavoriteNoteLength {
		return "", ErrSentenceTagInvalid
	}
	return normalized, nil
}

func NormalizeSentenceTagColor(color string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(color))
	if _, ok := sentenceTagColorSet[normalized]; !ok {
		return "", ErrSentenceTagInvalid
	}
	return normalized, nil
}

func CreateSentenceTag(
	ctx context.Context,
	req *model.SentenceTagCreateRequest,
) (*model.SentenceTagData, error) {
	if req == nil {
		return nil, ErrSentenceTagInvalid
	}
	name, err := NormalizeSentenceTagName(req.Name)
	if err != nil {
		return nil, err
	}
	color, err := NormalizeSentenceTagColor(req.Color)
	if err != nil {
		return nil, err
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}

	row := db.SentenceTag{Name: name, Color: color}
	if err := db.DB.WithContext(ctx).Create(&row).Error; err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, ErrSentenceTagConflict
		}
		return nil, err
	}
	return sentenceTagDataFromRow(row), nil
}

func ListSentenceTags(ctx context.Context) (*model.SentenceTagListData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	var rows []db.SentenceTag
	if err := db.DB.WithContext(ctx).
		Order("created_at ASC").
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]model.SentenceTagData, 0, len(rows))
	for _, row := range rows {
		items = append(items, *sentenceTagDataFromRow(row))
	}
	return &model.SentenceTagListData{Items: items, Total: int64(len(items))}, nil
}

func ReplaceSentenceFavoriteTags(
	ctx context.Context,
	sentenceFavoriteID int64,
	tagIDs []int64,
	note string,
) (*model.SentenceFavoriteData, error) {
	if sentenceFavoriteID <= 0 || len(tagIDs) > MaxSentenceTagsPerItem {
		return nil, ErrSentenceTagInvalid
	}
	normalizedNote, err := NormalizeSentenceFavoriteNote(note)
	if err != nil {
		return nil, err
	}
	if len(tagIDs) > 0 && normalizedNote == "" {
		return nil, ErrSentenceTagInvalid
	}
	uniqueTagIDs := make([]int64, 0, len(tagIDs))
	seen := make(map[int64]struct{}, len(tagIDs))
	for _, tagID := range tagIDs {
		if tagID <= 0 {
			return nil, ErrSentenceTagInvalid
		}
		if _, exists := seen[tagID]; exists {
			return nil, ErrSentenceTagDuplicate
		}
		seen[tagID] = struct{}{}
		uniqueTagIDs = append(uniqueTagIDs, tagID)
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}

	err = db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sentence db.SentenceFavorite
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Take(&sentence, sentenceFavoriteID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSentenceFavoriteNotFound
			}
			return err
		}

		if len(uniqueTagIDs) > 0 {
			var count int64
			if err := tx.Model(&db.SentenceTag{}).
				Where("id IN ?", uniqueTagIDs).
				Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(uniqueTagIDs)) {
				return ErrSentenceTagNotFound
			}
		}

		if err := tx.Model(&db.SentenceFavorite{}).
			Where("id = ?", sentenceFavoriteID).
			Update("note", normalizedNote).Error; err != nil {
			return err
		}
		if err := tx.Where("sentence_favorite_id = ?", sentenceFavoriteID).
			Delete(&db.SentenceFavoriteTag{}).Error; err != nil {
			return err
		}
		if len(uniqueTagIDs) == 0 {
			return nil
		}
		rows := make([]db.SentenceFavoriteTag, 0, len(uniqueTagIDs))
		for _, tagID := range uniqueTagIDs {
			rows = append(rows, db.SentenceFavoriteTag{
				SentenceFavoriteID: sentenceFavoriteID,
				TagID:              tagID,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	return GetSentenceFavorite(ctx, sentenceFavoriteID)
}

func sentenceTagDataFromRow(row db.SentenceTag) *model.SentenceTagData {
	return &model.SentenceTagData{
		ID:        row.ID,
		Name:      row.Name,
		Color:     row.Color,
		CreatedAt: row.CreatedAt.Format(time.RFC3339),
	}
}
