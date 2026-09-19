package model

import "encoding/json"

const (
	ActionLookupOpened     = "lookup_opened"
	ActionDetailOpened     = "detail_opened"
	ActionAudioPlayed      = "audio_played"
	ActionFavoriteCreated  = "favorite_created"
	ActionFavoriteDeleted  = "favorite_deleted"
	ActionLearningKnown    = "learning_known"
	ActionLearningUnknown  = "learning_unknown"
	ActionLearningMastered = "learning_mastered"
	ActionLearningReset    = "learning_reset"
	ActionContextSaved     = "context_saved"
	ActionAudioRegenerated = "audio_regenerated"
)

const (
	ActionSourceAndroidReader     = "android_reader"
	ActionSourceAndroidWordDetail = "android_word_detail"
	ActionSourceAndroidLearning   = "android_learning"
	ActionSourceWebVideo          = "web_video"
	ActionSourceWebWordDetail     = "web_word_detail"
	ActionSourceSystem            = "system"
)

type ItemActionRequest struct {
	EventID  string          `json:"eventId"`
	Action   string          `json:"action"`
	Source   string          `json:"source"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type ItemActionData struct {
	ItemType string `json:"itemType"`
	ItemID   int64  `json:"itemId"`
	Level    int    `json:"level,omitempty"`
}

type ItemActionResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg,omitempty"`
	Data *ItemActionData `json:"data,omitempty"`
}
