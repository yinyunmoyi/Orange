package model

import "time"

type ContextVideoSyncPingData struct {
	ServerID   string `json:"serverId"`
	APIVersion int    `json:"apiVersion"`
}

type ContextVideoSyncItem struct {
	VideoID     int64      `json:"videoId"`
	ContextID   int64      `json:"contextId"`
	ItemType    string     `json:"itemType"`
	ItemID      int64      `json:"itemId"`
	DurationMS  int64      `json:"durationMs"`
	FileSize    int64      `json:"fileSize"`
	Version     string     `json:"version"`
	VideoURL    string     `json:"videoUrl"`
	SubtitleURL string     `json:"subtitleUrl"`
	CreatedAt   time.Time  `json:"createdAt"`
	DueAt       *time.Time `json:"dueAt,omitempty"`
	Priority    int        `json:"priority"`
}

type ContextVideoSyncManifestData struct {
	ServerID    string                 `json:"serverId"`
	GeneratedAt time.Time              `json:"generatedAt"`
	HorizonEnd  time.Time              `json:"horizonEnd"`
	Items       []ContextVideoSyncItem `json:"items"`
}

type ContextVideoSyncResponse struct {
	Code int                           `json:"code"`
	Msg  string                        `json:"msg,omitempty"`
	Data *ContextVideoSyncManifestData `json:"data,omitempty"`
}

type ContextVideoSyncPingResponse struct {
	Code int                       `json:"code"`
	Msg  string                    `json:"msg,omitempty"`
	Data *ContextVideoSyncPingData `json:"data,omitempty"`
}
