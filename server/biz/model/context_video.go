package model

// ContextVideoSubtitle 是相对视频片段开始时间的英文字幕。
type ContextVideoSubtitle struct {
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// ContextVideoUploadRequest 是视频文件之外的上传元数据。
type ContextVideoUploadRequest struct {
	ContextID         int64
	SourceFingerprint string
	ClipStartMS       int64
	ClipEndMS         int64
	DurationMS        int64
	ContentType       string
	FileSize          int64
	AudioContentType  string
	AudioFileSize     int64
	Subtitles         []ContextVideoSubtitle
}

// ContextVideoData 是语境查询及上传响应中的视频信息。
type ContextVideoData struct {
	ID          int64  `json:"id"`
	VideoURL    string `json:"videoUrl"`
	SubtitleURL string `json:"subtitleUrl"`
	DurationMS  int64  `json:"durationMs"`
}

type ContextVideoResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg,omitempty"`
	Data *ContextVideoData `json:"data,omitempty"`
}
