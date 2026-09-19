package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
)

const (
	DefaultContextVideoSyncDays  = 7
	MaxContextVideoSyncDays      = 7
	DefaultContextVideoSyncLimit = 1000
	MaxContextVideoSyncLimit     = 1000
)

type contextVideoSyncRow struct {
	VideoID    int64
	ContextID  int64
	ItemType   string
	ItemID     int64
	DurationMS int64
	FileSize   int64
	FilePath   string
	Version    string
	CreatedAt  time.Time
	DueAt      *time.Time
	Priority   int
}

func GetContextVideoSyncManifest(
	ctx context.Context,
	serverID string,
	days int,
	limit int,
) (*model.ContextVideoSyncManifestData, error) {
	return getContextVideoSyncManifest(ctx, serverID, days, limit, time.Now())
}

func getContextVideoSyncManifest(
	ctx context.Context,
	serverID string,
	days int,
	limit int,
	now time.Time,
) (*model.ContextVideoSyncManifestData, error) {
	if serverID == "" || days < 1 || days > MaxContextVideoSyncDays ||
		limit < 1 || limit > MaxContextVideoSyncLimit {
		return nil, ErrLearningInvalid
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}

	loc := learningLocation()
	localNow := now.In(loc)
	today := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		0, 0, 0, 0, loc,
	)
	tomorrowEnd := today.AddDate(0, 0, 2)
	horizonEnd := today.AddDate(0, 0, days+1)
	recentCutoff := localNow.AddDate(0, 0, -7)

	var rows []contextVideoSyncRow
	query := db.DB.WithContext(ctx).
		Table("context_videos AS cv").
		Select(`
			cv.id AS video_id,
			cv.context_id,
			c.item_type,
			c.item_id,
			cv.duration_ms,
			cv.file_size,
			cv.file_path,
			cv.dedupe_key AS version,
			cv.created_at,
			uil.next_review_at AS due_at,
			CASE
				WHEN uil.next_review_at IS NOT NULL AND uil.next_review_at < ? THEN 0
				WHEN cv.created_at >= ? THEN 1
				ELSE 2
			END AS priority`,
			tomorrowEnd,
			recentCutoff,
		).
		Joins("JOIN contexts AS c ON c.id = cv.context_id").
		Joins(`
			LEFT JOIN user_item_learning AS uil
			  ON uil.item_id = c.item_id
			 AND uil.item_type = CASE c.item_type
			   WHEN 'word' THEN ?
			   WHEN 'phrase' THEN ?
			 END`,
			LearningItemWord,
			LearningItemPhrase,
		).
		Where("c.item_type IN ?", []string{ContextItemWord, ContextItemPhrase}).
		Where("cv.file_size > 0").
		Where(`
			cv.created_at >= ?
			OR (
				uil.learning_status = ?
				AND uil.next_review_at IS NOT NULL
				AND uil.next_review_at < ?
			)`,
			recentCutoff,
			"learning",
			horizonEnd,
		).
		Order("priority ASC").
		Order("CASE WHEN due_at IS NULL THEN 1 ELSE 0 END ASC").
		Order("due_at ASC").
		Order("cv.created_at DESC").
		Order("cv.id DESC").
		Limit(limit)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query context video sync manifest: %w", err)
	}

	items := make([]model.ContextVideoSyncItem, 0, len(rows))
	for _, row := range rows {
		if row.VideoID <= 0 || row.ContextID <= 0 || row.ItemID <= 0 ||
			row.DurationMS <= 0 || row.FileSize <= 0 || row.Version == "" {
			continue
		}
		if stat, err := os.Stat(row.FilePath); err != nil ||
			!stat.Mode().IsRegular() || stat.Size() != row.FileSize {
			continue
		}
		items = append(items, model.ContextVideoSyncItem{
			VideoID:     row.VideoID,
			ContextID:   row.ContextID,
			ItemType:    row.ItemType,
			ItemID:      row.ItemID,
			DurationMS:  row.DurationMS,
			FileSize:    row.FileSize,
			Version:     row.Version,
			VideoURL:    fmt.Sprintf("/api/v1/context-videos/%d/video", row.VideoID),
			SubtitleURL: fmt.Sprintf("/api/v1/context-videos/%d/subtitles.vtt", row.VideoID),
			CreatedAt:   row.CreatedAt,
			DueAt:       row.DueAt,
			Priority:    row.Priority,
		})
	}
	return &model.ContextVideoSyncManifestData{
		ServerID:    serverID,
		GeneratedAt: localNow,
		HorizonEnd:  horizonEnd,
		Items:       items,
	}, nil
}
