package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MaxContextVideoBytes      = int64(24 << 20)
	MaxContextAudioBytes      = int64(4 << 20)
	maxContextVideoDurationMS = int64(60_000)
	maxContextVideoSubtitles  = 100
)

var (
	ErrContextVideoInvalid  = errors.New("invalid context video")
	ErrContextVideoNotFound = errors.New("context video not found")
	contextVideoFingerprint = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func NormalizeContextVideoUpload(req *model.ContextVideoUploadRequest) (*model.ContextVideoUploadRequest, error) {
	if req == nil || req.ContextID <= 0 {
		return nil, ErrContextVideoInvalid
	}
	fingerprint := strings.ToLower(strings.TrimSpace(req.SourceFingerprint))
	if !contextVideoFingerprint.MatchString(fingerprint) {
		return nil, ErrContextVideoInvalid
	}
	contentType := strings.ToLower(strings.TrimSpace(req.ContentType))
	if !strings.HasPrefix(contentType, "video/webm") {
		return nil, ErrContextVideoInvalid
	}
	if req.ClipStartMS < 0 || req.ClipEndMS <= req.ClipStartMS ||
		req.DurationMS <= 0 || req.DurationMS > maxContextVideoDurationMS ||
		absInt64(req.ClipEndMS-req.ClipStartMS-req.DurationMS) > 1_000 ||
		req.FileSize <= 0 || req.FileSize > MaxContextVideoBytes {
		return nil, ErrContextVideoInvalid
	}
	audioContentType := strings.ToLower(strings.TrimSpace(req.AudioContentType))
	if (audioContentType == "") != (req.AudioFileSize == 0) {
		return nil, ErrContextVideoInvalid
	}
	if audioContentType != "" &&
		(!strings.HasPrefix(audioContentType, "audio/webm") ||
			req.AudioFileSize <= 0 || req.AudioFileSize > MaxContextAudioBytes) {
		return nil, ErrContextVideoInvalid
	}
	if len(req.Subtitles) == 0 || len(req.Subtitles) > maxContextVideoSubtitles {
		return nil, ErrContextVideoInvalid
	}

	subtitles := make([]model.ContextVideoSubtitle, 0, len(req.Subtitles))
	for _, subtitle := range req.Subtitles {
		text := strings.TrimSpace(subtitle.Text)
		if text == "" || len([]rune(text)) > 500 ||
			subtitle.StartMS < 0 || subtitle.EndMS <= subtitle.StartMS ||
			subtitle.EndMS > req.DurationMS {
			return nil, ErrContextVideoInvalid
		}
		subtitles = append(subtitles, model.ContextVideoSubtitle{
			StartMS: subtitle.StartMS,
			EndMS:   subtitle.EndMS,
			Text:    text,
		})
	}

	return &model.ContextVideoUploadRequest{
		ContextID:         req.ContextID,
		SourceFingerprint: fingerprint,
		ClipStartMS:       req.ClipStartMS,
		ClipEndMS:         req.ClipEndMS,
		DurationMS:        req.DurationMS,
		ContentType:       contentType,
		FileSize:          req.FileSize,
		AudioContentType:  audioContentType,
		AudioFileSize:     req.AudioFileSize,
		Subtitles:         subtitles,
	}, nil
}

func ContextVideoDedupeKey(req *model.ContextVideoUploadRequest) string {
	raw := fmt.Sprintf("%d\x00%s\x00%d\x00%d",
		req.ContextID, req.SourceFingerprint, req.ClipStartMS, req.ClipEndMS)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func SaveContextVideo(
	ctx context.Context,
	rawReq *model.ContextVideoUploadRequest,
	videoBody io.Reader,
	audioBody io.Reader,
) (*model.ContextVideoData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	req, err := NormalizeContextVideoUpload(rawReq)
	if err != nil {
		return nil, err
	}
	dedupeKey := ContextVideoDedupeKey(req)
	existing, lookupErr := findContextVideoByDedupeKey(ctx, db.DB, dedupeKey)
	if lookupErr == nil && audioBody == nil {
		return contextVideoData(existing), nil
	}
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query context video: %w", lookupErr)
	}

	subtitlesJSON, err := json.Marshal(req.Subtitles)
	if err != nil {
		return nil, fmt.Errorf("marshal context video subtitles: %w", err)
	}
	var path string
	var actualSize int64
	if lookupErr == nil {
		path = existing.FilePath
		actualSize = existing.FileSize
	} else {
		path, actualSize, err = storage.SaveContextVideo(
			dedupeKey,
			io.LimitReader(videoBody, MaxContextVideoBytes+1),
		)
		if err != nil {
			return nil, fmt.Errorf("save context video file: %w", err)
		}
		if actualSize <= 0 || actualSize > MaxContextVideoBytes {
			_ = storage.Delete(path)
			return nil, ErrContextVideoInvalid
		}
	}

	var audioPath string
	var actualAudioSize int64
	if audioBody != nil {
		audioPath, actualAudioSize, err = storage.SaveContextOriginalAudio(
			dedupeKey,
			io.LimitReader(audioBody, MaxContextAudioBytes+1),
		)
		if err != nil {
			if lookupErr != nil {
				_ = storage.Delete(path)
			}
			return nil, fmt.Errorf("save context original audio: %w", err)
		}
		if actualAudioSize <= 0 || actualAudioSize > MaxContextAudioBytes {
			_ = storage.Delete(audioPath)
			if lookupErr != nil {
				_ = storage.Delete(path)
			}
			return nil, ErrContextVideoInvalid
		}
	}

	row := db.ContextVideo{
		ContextID:         req.ContextID,
		SourceFingerprint: req.SourceFingerprint,
		ClipStartMS:       req.ClipStartMS,
		ClipEndMS:         req.ClipEndMS,
		DurationMS:        req.DurationMS,
		FilePath:          path,
		ContentType:       req.ContentType,
		FileSize:          actualSize,
		SubtitlesJSON:     string(subtitlesJSON),
		DedupeKey:         dedupeKey,
	}
	preserveFile := false
	oldAudioPath := ""
	err = db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var contextRow db.Context
		lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", req.ContextID).Take(&contextRow).Error
		if lockErr != nil {
			if errors.Is(lockErr, gorm.ErrRecordNotFound) {
				return ErrContextItemNotFound
			}
			return fmt.Errorf("lock context video context: %w", lockErr)
		}
		if itemErr := lockContextItem(tx, contextRow.ItemType, contextRow.ItemID); itemErr != nil {
			return itemErr
		}
		if audioPath != "" {
			oldAudioPath = contextRow.AudioFilePath
			if updateErr := tx.Model(&db.Context{}).
				Where("id = ?", contextRow.ID).
				Updates(map[string]any{
					"audio_file_path":    audioPath,
					"audio_content_type": req.AudioContentType,
					"audio_file_size":    actualAudioSize,
				}).Error; updateErr != nil {
				return fmt.Errorf("update context original audio: %w", updateErr)
			}
		}

		if lookupErr == nil {
			row = *existing
			preserveFile = true
		} else {
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "dedupe_key"}},
				DoNothing: true,
			}).Create(&row)
			if result.Error != nil {
				return fmt.Errorf("create context video: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				existingRow, existingErr := findContextVideoByDedupeKey(ctx, tx, dedupeKey)
				if existingErr != nil {
					return fmt.Errorf("load existing context video: %w", existingErr)
				}
				row = *existingRow
				preserveFile = true
			}
		}
		return nil
	})
	if err != nil {
		if !preserveFile {
			if deleteErr := storage.Delete(path); deleteErr != nil {
				log.Printf("[context-video] cleanup failed path=%q err=%v", path, deleteErr)
			}
		}
		if audioPath != "" {
			_ = storage.Delete(audioPath)
		}
		return nil, err
	}
	if oldAudioPath != "" && oldAudioPath != audioPath {
		if deleteErr := storage.Delete(oldAudioPath); deleteErr != nil {
			log.Printf("[context-video] delete old context audio failed path=%q err=%v", oldAudioPath, deleteErr)
		}
	}
	return contextVideoData(&row), nil
}

func LookupContextVideo(ctx context.Context, videoID int64) (*db.ContextVideo, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	if videoID <= 0 {
		return nil, ErrContextVideoInvalid
	}
	var row db.ContextVideo
	if err := db.DB.WithContext(ctx).Where("id = ?", videoID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrContextVideoNotFound
		}
		return nil, err
	}
	return &row, nil
}

func ContextVideoVTT(row *db.ContextVideo) (string, error) {
	if row == nil {
		return "", ErrContextVideoInvalid
	}
	var subtitles []model.ContextVideoSubtitle
	if decodeErr := json.Unmarshal([]byte(row.SubtitlesJSON), &subtitles); decodeErr != nil {
		return "", fmt.Errorf("decode context video subtitles: %w", decodeErr)
	}
	var builder strings.Builder
	builder.WriteString("WEBVTT\n\n")
	for index, subtitle := range subtitles {
		fmt.Fprintf(&builder, "%d\n%s --> %s\n%s\n\n",
			index+1,
			formatVTTTime(subtitle.StartMS),
			formatVTTTime(subtitle.EndMS),
			strings.ReplaceAll(subtitle.Text, "-->", "--\u200b>"),
		)
	}
	return builder.String(), nil
}

func LoadContextVideos(
	ctx context.Context,
	contextIDs []int64,
) (map[int64][]model.ContextVideoData, error) {
	result := make(map[int64][]model.ContextVideoData)
	if len(contextIDs) == 0 {
		return result, nil
	}
	var rows []db.ContextVideo
	if err := db.DB.WithContext(ctx).
		Where("context_id IN ?", contextIDs).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list context videos: %w", err)
	}
	for i := range rows {
		row := &rows[i]
		result[row.ContextID] = append(result[row.ContextID], *contextVideoData(row))
	}
	return result, nil
}

func findContextVideoByDedupeKey(
	ctx context.Context,
	query *gorm.DB,
	dedupeKey string,
) (*db.ContextVideo, error) {
	var row db.ContextVideo
	if err := query.WithContext(ctx).Where("dedupe_key = ?", dedupeKey).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func contextVideoData(row *db.ContextVideo) *model.ContextVideoData {
	return &model.ContextVideoData{
		ID:          row.ID,
		VideoURL:    fmt.Sprintf("/api/v1/context-videos/%d/video", row.ID),
		SubtitleURL: fmt.Sprintf("/api/v1/context-videos/%d/subtitles.vtt", row.ID),
		DurationMS:  row.DurationMS,
	}
}

func formatVTTTime(milliseconds int64) string {
	hours := milliseconds / 3_600_000
	minutes := milliseconds % 3_600_000 / 60_000
	seconds := milliseconds % 60_000 / 1_000
	millis := milliseconds % 1_000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
