package model

type PhraseFavoriteRequest struct {
	Phrase  string       `json:"phrase"`
	Meaning *MeaningData `json:"meaning"`
}

type PhraseFavoriteData struct {
	ItemID int64 `json:"itemId"`
}

type PhraseFavoriteResponse struct {
	Code int                 `json:"code"`
	Msg  string              `json:"msg,omitempty"`
	Data *PhraseFavoriteData `json:"data,omitempty"`
}

type PhraseFavoriteStatusData struct {
	Favorited bool  `json:"favorited"`
	ItemID    int64 `json:"itemId,omitempty"`
}

type PhraseFavoriteStatusResponse struct {
	Code int                       `json:"code"`
	Msg  string                    `json:"msg,omitempty"`
	Data *PhraseFavoriteStatusData `json:"data,omitempty"`
}

type PhraseDetailData struct {
	ItemType string           `json:"itemType"`
	ItemID   int64            `json:"itemId"`
	Phrase   string           `json:"phrase"`
	AudioURL string           `json:"audioUrl"`
	Meanings []ChineseMeaning `json:"meanings"`
	Contexts []Context        `json:"contexts"`
}

type PhraseDetailResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg,omitempty"`
	Data *PhraseDetailData `json:"data,omitempty"`
}
