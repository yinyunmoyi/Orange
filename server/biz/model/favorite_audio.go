package model

type FavoriteAudioData struct {
	AudioURL string `json:"audioUrl"`
}

type FavoriteAudioResponse struct {
	Code int                `json:"code"`
	Msg  string             `json:"msg,omitempty"`
	Data *FavoriteAudioData `json:"data,omitempty"`
}
