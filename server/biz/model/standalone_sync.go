package model

type StandaloneFavoriteSyncRequest struct {
	ClientID    string `json:"clientId"`
	ItemType    string `json:"itemType"`
	Text        string `json:"text"`
	Translation string `json:"translation,omitempty"`
}

type StandaloneFavoriteSyncData struct {
	ClientID   string `json:"clientId"`
	ItemType   string `json:"itemType"`
	ItemID     int64  `json:"itemId"`
	Idempotent bool   `json:"idempotent"`
}

type StandaloneFavoriteSyncResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg,omitempty"`
	Data *StandaloneFavoriteSyncData `json:"data,omitempty"`
}
