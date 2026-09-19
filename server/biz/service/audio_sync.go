package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"aaa_word/biz/converter"
	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
)

var (
	ErrAudioSyncInvalid  = errors.New("invalid audio sync request")
	ErrAudioSyncNotFound = errors.New("audio sync item not found")
)

type audioSyncFile struct {
	ItemType    string
	ItemID      int64
	AudioID     int64
	Accent      string
	SourceURL   string
	FilePath    string
	ContentType string
	FileSize    int64
}

func GetAudioSyncManifest(
	ctx context.Context,
	serverID string,
	now time.Time,
) (*model.AudioSyncManifestData, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, ErrAudioSyncInvalid
	}
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}

	var audios []db.WordAudio
	if err := db.DB.WithContext(ctx).
		Order("word_id ASC, id ASC").
		Find(&audios).Error; err != nil {
		return nil, fmt.Errorf("query word audio sync manifest: %w", err)
	}
	var phrases []db.Phrase
	if err := db.DB.WithContext(ctx).
		Order("id ASC").
		Find(&phrases).Error; err != nil {
		return nil, fmt.Errorf("query phrase audio sync manifest: %w", err)
	}

	items := make([]model.AudioSyncItem, 0, len(audios)+len(phrases))
	for _, audio := range audios {
		file := audioSyncFile{
			ItemType:    ContextItemWord,
			ItemID:      audio.WordID,
			AudioID:     audio.ID,
			Accent:      audio.Accent,
			SourceURL:   audio.SourceURL,
			FilePath:    audio.FilePath,
			ContentType: audio.ContentType,
			FileSize:    audio.FileSize,
		}
		if item, ok := buildAudioSyncItem(file); ok {
			items = append(items, item)
		}
	}
	for _, phrase := range phrases {
		file := audioSyncFile{
			ItemType:    ContextItemPhrase,
			ItemID:      phrase.ID,
			AudioID:     0,
			FilePath:    phrase.AudioFilePath,
			ContentType: phrase.AudioContentType,
			FileSize:    phrase.AudioFileSize,
		}
		if item, ok := buildAudioSyncItem(file); ok {
			items = append(items, item)
		}
	}
	return &model.AudioSyncManifestData{
		ServerID:    serverID,
		GeneratedAt: now,
		Items:       items,
	}, nil
}

func LookupAudioSyncFile(
	ctx context.Context,
	itemType string,
	audioKey int64,
	version string,
) (path, contentType, currentVersion string, err error) {
	if audioKey <= 0 || strings.TrimSpace(version) == "" ||
		(itemType != ContextItemWord && itemType != ContextItemPhrase) {
		return "", "", "", ErrAudioSyncInvalid
	}
	if !db.Enabled() {
		return "", "", "", db.ErrDBDisabled
	}

	var file audioSyncFile
	switch itemType {
	case ContextItemWord:
		var audio db.WordAudio
		if queryErr := db.DB.WithContext(ctx).Where("id = ?", audioKey).Take(&audio).Error; queryErr != nil {
			if errors.Is(queryErr, gorm.ErrRecordNotFound) {
				return "", "", "", ErrAudioSyncNotFound
			}
			return "", "", "", fmt.Errorf("query audio sync word: %w", queryErr)
		}
		file = audioSyncFile{
			ItemType:    ContextItemWord,
			ItemID:      audio.WordID,
			AudioID:     audio.ID,
			Accent:      audio.Accent,
			SourceURL:   audio.SourceURL,
			FilePath:    audio.FilePath,
			ContentType: audio.ContentType,
			FileSize:    audio.FileSize,
		}
	case ContextItemPhrase:
		var phrase db.Phrase
		if queryErr := db.DB.WithContext(ctx).Where("id = ?", audioKey).Take(&phrase).Error; queryErr != nil {
			if errors.Is(queryErr, gorm.ErrRecordNotFound) {
				return "", "", "", ErrAudioSyncNotFound
			}
			return "", "", "", fmt.Errorf("query audio sync phrase: %w", queryErr)
		}
		file = audioSyncFile{
			ItemType:    ContextItemPhrase,
			ItemID:      phrase.ID,
			FilePath:    phrase.AudioFilePath,
			ContentType: phrase.AudioContentType,
			FileSize:    phrase.AudioFileSize,
		}
	}
	stat, statErr := healthyAudioSyncFile(file)
	if statErr != nil {
		return "", "", "", ErrAudioSyncNotFound
	}
	currentVersion = audioSyncVersion(file, stat)
	if version != currentVersion {
		return "", "", currentVersion, ErrAudioSyncNotFound
	}
	contentType = strings.TrimSpace(file.ContentType)
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return file.FilePath, contentType, currentVersion, nil
}

func buildAudioSyncItem(file audioSyncFile) (model.AudioSyncItem, bool) {
	stat, err := healthyAudioSyncFile(file)
	if err != nil {
		return model.AudioSyncItem{}, false
	}
	version := audioSyncVersion(file, stat)
	audioKey := file.ItemID
	playbackURL := fmt.Sprintf("/api/v1/phrase/%d/audio", file.ItemID)
	if file.ItemType == ContextItemWord {
		audioKey = file.AudioID
		if strings.TrimSpace(file.SourceURL) == "" {
			return model.AudioSyncItem{}, false
		}
		playbackURL = converter.AudioProxyPath + "?src=" + url.QueryEscape(file.SourceURL)
	}
	return model.AudioSyncItem{
		ItemType: file.ItemType,
		ItemID:   file.ItemID,
		AudioID:  file.AudioID,
		Accent:   file.Accent,
		FileSize: stat.Size(),
		Version:  version,
		AudioURL: fmt.Sprintf(
			"/api/v1/audio-sync/items/%s/%d?v=%s",
			file.ItemType,
			audioKey,
			url.QueryEscape(version),
		),
		PlaybackURL: playbackURL,
	}, true
}

func healthyAudioSyncFile(file audioSyncFile) (os.FileInfo, error) {
	if file.ItemID <= 0 || file.FileSize <= 0 || strings.TrimSpace(file.FilePath) == "" {
		return nil, ErrAudioSyncNotFound
	}
	if file.ItemType == ContextItemWord && file.AudioID <= 0 {
		return nil, ErrAudioSyncNotFound
	}
	stat, err := os.Stat(file.FilePath)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != file.FileSize {
		return nil, ErrAudioSyncNotFound
	}
	return stat, nil
}

func audioSyncVersion(file audioSyncFile, stat os.FileInfo) string {
	payload := fmt.Sprintf(
		"%s:%d:%d:%d:%d",
		file.ItemType,
		file.ItemID,
		file.AudioID,
		stat.Size(),
		stat.ModTime().UnixNano(),
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
