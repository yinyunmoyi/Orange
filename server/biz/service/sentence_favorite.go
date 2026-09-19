package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxSentenceTranslationLength = 2000

var (
	ErrSentenceFavoriteInvalid  = errors.New("invalid sentence favorite request")
	ErrSentenceTranslationLong  = errors.New("sentence translation is too long")
	ErrSentenceFavoriteNotFound = errors.New("sentence favorite not found")
	sentenceFavoritePattern     = regexp.MustCompile(`^[\x20-\x7E\p{Latin}\p{M}]+$`)
)

func NormalizeFavoriteSentence(sentence string) (string, error) {
	normalized := strings.Join(strings.Fields(sentence), " ")
	if normalized == "" ||
		utf8.RuneCountInString(normalized) > 500 ||
		!sentenceFavoritePattern.MatchString(normalized) {
		return "", ErrSentenceFavoriteInvalid
	}
	return normalized, nil
}

func SentenceFavoriteDedupeKey(sentence string) string {
	sum := sha256.Sum256([]byte(sentence))
	return hex.EncodeToString(sum[:])
}

func SaveSentenceFavorite(
	ctx context.Context,
	req *model.SentenceFavoriteRequest,
) (*model.SentenceFavoriteData, error) {
	if req == nil {
		return nil, ErrSentenceFavoriteInvalid
	}
	sentence, err := NormalizeFavoriteSentence(req.Sentence)
	if err != nil {
		return nil, err
	}
	translation := strings.TrimSpace(req.Translation)
	if translation == "" {
		return nil, ErrSentenceFavoriteInvalid
	}
	if utf8.RuneCountInString(translation) > MaxSentenceTranslationLength {
		return nil, ErrSentenceTranslationLong
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}

	dedupeKey := SentenceFavoriteDedupeKey(sentence)
	row := db.SentenceFavorite{
		Sentence:    sentence,
		Translation: translation,
		DedupeKey:   dedupeKey,
	}
	if err := db.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return nil, err
	}
	if err := db.DB.WithContext(ctx).
		Where("dedupe_key = ?", dedupeKey).
		Take(&row).Error; err != nil {
		return nil, err
	}
	return sentenceFavoriteDataFromRow(row), nil
}

func GetSentenceFavoriteStatus(
	ctx context.Context,
	sentence string,
) (*model.SentenceFavoriteStatusData, error) {
	normalized, err := NormalizeFavoriteSentence(sentence)
	if err != nil {
		return nil, err
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}

	var row db.SentenceFavorite
	err = db.DB.WithContext(ctx).
		Select("id").
		Where("dedupe_key = ?", SentenceFavoriteDedupeKey(normalized)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.SentenceFavoriteStatusData{Favorited: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.SentenceFavoriteStatusData{Favorited: true, ID: row.ID}, nil
}

func ListSentenceFavorites(ctx context.Context, filterTagIDs ...int64) (*model.SentenceFavoriteListData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	var rows []db.SentenceFavorite
	query := db.DB.WithContext(ctx).Model(&db.SentenceFavorite{})
	if len(filterTagIDs) > 0 {
		query = query.
			Joins("JOIN sentence_favorite_tags ON sentence_favorite_tags.sentence_favorite_id = sentence_favorites.id").
			Where("sentence_favorite_tags.tag_id IN ?", filterTagIDs).
			Group("sentence_favorites.id")
	}
	if err := query.
		Order("sentence_favorites.created_at DESC").
		Order("sentence_favorites.id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]model.SentenceFavoriteData, 0, len(rows))
	for _, row := range rows {
		items = append(items, *sentenceFavoriteDataFromRow(row))
	}
	if err := attachSentenceFavoriteTags(ctx, items); err != nil {
		return nil, err
	}
	return &model.SentenceFavoriteListData{
		Items: items,
		Total: int64(len(items)),
	}, nil
}

func GetSentenceFavorite(
	ctx context.Context,
	id int64,
) (*model.SentenceFavoriteData, error) {
	if id <= 0 {
		return nil, ErrSentenceFavoriteInvalid
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	var row db.SentenceFavorite
	if err := db.DB.WithContext(ctx).Take(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSentenceFavoriteNotFound
		}
		return nil, err
	}
	data := sentenceFavoriteDataFromRow(row)
	items := []model.SentenceFavoriteData{*data}
	if err := attachSentenceFavoriteTags(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

func sentenceFavoriteDataFromRow(row db.SentenceFavorite) *model.SentenceFavoriteData {
	return &model.SentenceFavoriteData{
		ID:          row.ID,
		Sentence:    row.Sentence,
		Translation: row.Translation,
		Note:        row.Note,
		CreatedAt:   row.CreatedAt.Format(time.RFC3339),
		Tags:        []model.SentenceFavoriteTagData{},
	}
}

type sentenceFavoriteTagRow struct {
	SentenceFavoriteID int64  `gorm:"column:sentence_favorite_id"`
	TagID              int64  `gorm:"column:tag_id"`
	Name               string `gorm:"column:name"`
	Color              string `gorm:"column:color"`
}

func attachSentenceFavoriteTags(
	ctx context.Context,
	items []model.SentenceFavoriteData,
) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	itemByID := make(map[int64]*model.SentenceFavoriteData, len(items))
	for index := range items {
		ids = append(ids, items[index].ID)
		itemByID[items[index].ID] = &items[index]
	}

	var rows []sentenceFavoriteTagRow
	if err := db.DB.WithContext(ctx).
		Table("sentence_favorite_tags AS relations").
		Select("relations.sentence_favorite_id, relations.tag_id, tags.name, tags.color").
		Joins("JOIN sentence_tags AS tags ON tags.id = relations.tag_id").
		Where("relations.sentence_favorite_id IN ?", ids).
		Order("relations.created_at ASC").
		Order("relations.tag_id ASC").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		item := itemByID[row.SentenceFavoriteID]
		if item == nil {
			continue
		}
		item.Tags = append(item.Tags, model.SentenceFavoriteTagData{
			ID:    row.TagID,
			Name:  row.Name,
			Color: row.Color,
		})
	}
	return nil
}
