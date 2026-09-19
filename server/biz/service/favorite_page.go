package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
)

var ErrFavoritePageInvalid = errors.New("invalid favorite page request")

type favoriteGroupCountRow struct {
	BucketType string `gorm:"column:bucket_type"`
	BucketKey  string `gorm:"column:bucket_key"`
	Count      int64  `gorm:"column:count"`
}

type favoriteCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ItemType  string    `json:"itemType"`
	ItemID    int64     `json:"itemId"`
}

type favoritePageCandidate struct {
	ItemType  string
	ItemID    int64
	Text      string
	Level     int
	CreatedAt time.Time
}

func ListFavoriteGroups(ctx context.Context, now time.Time) (*model.FavoriteGroupListData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	loc := learningLocation()
	localNow := now.In(loc)
	today := startOfFavoriteDay(localNow)
	recentStart := today.AddDate(0, 0, -6)
	historyStart := today.AddDate(0, 0, -29)

	const query = `
SELECT
  CASE
    WHEN created_at >= ? THEN 'day'
    WHEN created_at >= ? THEN 'week'
    ELSE 'month'
  END AS bucket_type,
  CASE
    WHEN created_at >= ? THEN DATE_FORMAT(created_at, '%Y-%m-%d')
    WHEN created_at >= ? THEN DATE_FORMAT(DATE_SUB(DATE(created_at), INTERVAL WEEKDAY(created_at) DAY), '%Y-%m-%d')
    ELSE DATE_FORMAT(created_at, '%Y-%m-01')
  END AS bucket_key,
  COUNT(*) AS count
FROM (
  SELECT created_at FROM words
  UNION ALL
  SELECT created_at FROM phrases
) favorites
GROUP BY bucket_type, bucket_key`

	var rows []favoriteGroupCountRow
	if err := db.DB.WithContext(ctx).Raw(
		query, recentStart, historyStart, recentStart, historyStart,
	).Scan(&rows).Error; err != nil {
		log.Printf("[favorite-groups] aggregate failed recent_start=%s history_start=%s err=%v",
			recentStart.Format(time.RFC3339), historyStart.Format(time.RFC3339), err)
		return nil, fmt.Errorf("aggregate favorite groups: %w", err)
	}
	groups, total, err := buildFavoriteGroupSummaries(rows, localNow, loc)
	if err != nil {
		log.Printf("[favorite-groups] build failed rows=%v err=%v", rows, err)
		return nil, err
	}
	return &model.FavoriteGroupListData{Groups: groups, Total: total}, nil
}

func buildFavoriteGroupSummaries(
	rows []favoriteGroupCountRow,
	now time.Time,
	loc *time.Location,
) ([]model.FavoriteGroupSummary, int64, error) {
	today := startOfFavoriteDay(now.In(loc))
	recentStart := today.AddDate(0, 0, -6)
	historyStart := today.AddDate(0, 0, -29)
	groups := make([]model.FavoriteGroupSummary, 0, len(rows))
	var total int64

	for _, row := range rows {
		if row.Count <= 0 {
			continue
		}
		bucketStart, err := time.ParseInLocation("2006-01-02", row.BucketKey, loc)
		if err != nil {
			return nil, 0, fmt.Errorf("parse favorite group %q: %w", row.BucketKey, err)
		}
		rangeStart := bucketStart
		rangeEnd := bucketStart
		displayStart := bucketStart
		displayEnd := bucketStart
		switch row.BucketType {
		case "day":
			rangeEnd = bucketStart.AddDate(0, 0, 1)
			displayEnd = bucketStart
		case "week":
			naturalEnd := bucketStart.AddDate(0, 0, 7)
			rangeStart = maxFavoriteTime(bucketStart, historyStart)
			rangeEnd = minFavoriteTime(naturalEnd, recentStart)
			displayEnd = naturalEnd.AddDate(0, 0, -1)
		case "month":
			naturalEnd := bucketStart.AddDate(0, 1, 0)
			rangeEnd = minFavoriteTime(naturalEnd, historyStart)
			displayEnd = naturalEnd.AddDate(0, 0, -1)
		default:
			return nil, 0, fmt.Errorf("unknown favorite group type %q", row.BucketType)
		}
		if !rangeStart.Before(rangeEnd) {
			return nil, 0, fmt.Errorf("empty favorite group range type=%s key=%s", row.BucketType, row.BucketKey)
		}
		groups = append(groups, model.FavoriteGroupSummary{
			ID:           row.BucketType + "-" + row.BucketKey,
			Type:         row.BucketType,
			RangeStart:   rangeStart.Format(time.RFC3339),
			RangeEnd:     rangeEnd.Format(time.RFC3339),
			DisplayStart: displayStart.Format(time.RFC3339),
			DisplayEnd:   displayEnd.Format(time.RFC3339),
			Count:        row.Count,
		})
		total += row.Count
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].RangeStart > groups[j].RangeStart
	})
	return groups, total, nil
}

func ListFavoritePage(
	ctx context.Context,
	start time.Time,
	end time.Time,
	limit int,
	rawCursor string,
) (*model.FavoritePageData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	if !start.Before(end) || limit < 1 || limit > 100 {
		return nil, ErrFavoritePageInvalid
	}
	var cursor *favoriteCursor
	if rawCursor != "" {
		decoded, err := decodeFavoriteCursor(rawCursor)
		if err != nil {
			return nil, ErrFavoritePageInvalid
		}
		cursor = &decoded
	}

	words, err := loadFavoriteWordCandidates(ctx, start, end, limit+1, cursor)
	if err != nil {
		return nil, err
	}
	phrases, err := loadFavoritePhraseCandidates(ctx, start, end, limit+1, cursor)
	if err != nil {
		return nil, err
	}
	candidates := append(words, phrases...)
	sortFavoritePageCandidates(candidates)
	hasMore := len(candidates) > limit
	if hasMore {
		candidates = candidates[:limit]
	}
	items, err := favoriteCandidatesToItems(ctx, candidates)
	if err != nil {
		return nil, err
	}
	data := &model.FavoritePageData{Items: items}
	if hasMore && len(candidates) > 0 {
		data.NextCursor, err = encodeFavoriteCursor(candidates[len(candidates)-1])
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func loadFavoriteWordCandidates(
	ctx context.Context, start, end time.Time, limit int, cursor *favoriteCursor,
) ([]favoritePageCandidate, error) {
	var rows []db.Word
	query := db.DB.WithContext(ctx).Where("created_at >= ? AND created_at < ?", start, end)
	query = applyFavoriteCursor(query, ContextItemWord, cursor)
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		log.Printf("[favorite-page] query words failed start=%s end=%s err=%v", start, end, err)
		return nil, fmt.Errorf("query favorite words: %w", err)
	}
	result := make([]favoritePageCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, favoritePageCandidate{
			ItemType: ContextItemWord, ItemID: row.ID, Text: row.Word, Level: row.Level, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func loadFavoritePhraseCandidates(
	ctx context.Context, start, end time.Time, limit int, cursor *favoriteCursor,
) ([]favoritePageCandidate, error) {
	var rows []db.Phrase
	query := db.DB.WithContext(ctx).Where("created_at >= ? AND created_at < ?", start, end)
	query = applyFavoriteCursor(query, ContextItemPhrase, cursor)
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		log.Printf("[favorite-page] query phrases failed start=%s end=%s err=%v", start, end, err)
		return nil, fmt.Errorf("query favorite phrases: %w", err)
	}
	result := make([]favoritePageCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, favoritePageCandidate{
			ItemType: ContextItemPhrase, ItemID: row.ID, Text: row.Phrase, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func applyFavoriteCursor(query *gorm.DB, itemType string, cursor *favoriteCursor) *gorm.DB {
	if cursor == nil {
		return query
	}
	candidateOrder := favoriteTypeOrder(itemType)
	cursorOrder := favoriteTypeOrder(cursor.ItemType)
	switch {
	case candidateOrder < cursorOrder:
		return query.Where("created_at < ?", cursor.CreatedAt)
	case candidateOrder > cursorOrder:
		return query.Where("created_at <= ?", cursor.CreatedAt)
	default:
		return query.Where("created_at < ? OR (created_at = ? AND id < ?)",
			cursor.CreatedAt, cursor.CreatedAt, cursor.ItemID)
	}
}

func favoriteCandidatesToItems(
	ctx context.Context,
	candidates []favoritePageCandidate,
) ([]model.FavoriteListItem, error) {
	wordIDs := make([]int64, 0)
	phraseIDs := make([]int64, 0)
	for _, candidate := range candidates {
		if candidate.ItemType == ContextItemWord {
			wordIDs = append(wordIDs, candidate.ItemID)
		} else {
			phraseIDs = append(phraseIDs, candidate.ItemID)
		}
	}
	topWords := make(map[int64]db.WordMeaning)
	if len(wordIDs) > 0 {
		var meanings []db.WordMeaning
		if err := db.DB.WithContext(ctx).Where("word_id IN ? AND kind = ?", wordIDs, "zh").
			Order("word_id, sort_order").Find(&meanings).Error; err != nil {
			return nil, fmt.Errorf("query favorite page word meanings: %w", err)
		}
		for _, meaning := range meanings {
			if _, exists := topWords[meaning.WordID]; !exists {
				topWords[meaning.WordID] = meaning
			}
		}
	}
	topPhrases := make(map[int64]db.PhraseMeaning)
	if len(phraseIDs) > 0 {
		var meanings []db.PhraseMeaning
		if err := db.DB.WithContext(ctx).Where("phrase_id IN ?", phraseIDs).
			Order("phrase_id, sort_order").Find(&meanings).Error; err != nil {
			return nil, fmt.Errorf("query favorite page phrase meanings: %w", err)
		}
		for _, meaning := range meanings {
			if _, exists := topPhrases[meaning.PhraseID]; !exists {
				topPhrases[meaning.PhraseID] = meaning
			}
		}
	}
	items := make([]model.FavoriteListItem, 0, len(candidates))
	for _, candidate := range candidates {
		item := model.FavoriteListItem{
			ItemType: candidate.ItemType, ItemID: candidate.ItemID, Text: candidate.Text,
			CreatedAt: candidate.CreatedAt.Format(time.RFC3339),
		}
		if candidate.ItemType == ContextItemWord {
			item.Word = candidate.Text
			item.Level = candidate.Level
			if top, ok := topWords[candidate.ItemID]; ok {
				item.TopMeaningPOS, item.TopMeaningText = top.PartOfSpeech, top.Definition
			}
		} else if top, ok := topPhrases[candidate.ItemID]; ok {
			item.TopMeaningPOS, item.TopMeaningText = top.PartOfSpeech, top.Definition
		}
		items = append(items, item)
	}
	return items, nil
}

func sortFavoritePageCandidates(items []favoritePageCandidate) {
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		if items[i].ItemType != items[j].ItemType {
			return favoriteTypeOrder(items[i].ItemType) < favoriteTypeOrder(items[j].ItemType)
		}
		return items[i].ItemID > items[j].ItemID
	})
}

func encodeFavoriteCursor(candidate favoritePageCandidate) (string, error) {
	body, err := json.Marshal(favoriteCursor{
		CreatedAt: candidate.CreatedAt, ItemType: candidate.ItemType, ItemID: candidate.ItemID,
	})
	if err != nil {
		return "", fmt.Errorf("encode favorite cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func decodeFavoriteCursor(raw string) (favoriteCursor, error) {
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return favoriteCursor{}, ErrFavoritePageInvalid
	}
	var cursor favoriteCursor
	if err := json.Unmarshal(body, &cursor); err != nil {
		return favoriteCursor{}, ErrFavoritePageInvalid
	}
	if cursor.ItemID <= 0 ||
		(cursor.ItemType != ContextItemWord && cursor.ItemType != ContextItemPhrase) ||
		cursor.CreatedAt.IsZero() {
		return favoriteCursor{}, ErrFavoritePageInvalid
	}
	return cursor, nil
}

func favoriteTypeOrder(itemType string) int {
	if itemType == ContextItemWord {
		return 1
	}
	return 2
}

func startOfFavoriteDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func minFavoriteTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxFavoriteTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
