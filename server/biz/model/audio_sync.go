package model

import "time"

type AudioSyncItem struct {
	ItemType    string `json:"itemType"`
	ItemID      int64  `json:"itemId"`
	AudioID     int64  `json:"audioId"`
	Accent      string `json:"accent,omitempty"`
	FileSize    int64  `json:"fileSize"`
	Version     string `json:"version"`
	AudioURL    string `json:"audioUrl"`
	PlaybackURL string `json:"playbackUrl"`
}

type AudioSyncManifestData struct {
	ServerID    string          `json:"serverId"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Items       []AudioSyncItem `json:"items"`
}

type AudioSyncResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg,omitempty"`
	Data *AudioSyncManifestData `json:"data,omitempty"`
}
