package model

type NoteRequest struct {
	ItemType string `json:"itemType"`
	Text     string `json:"text"`
	Note     string `json:"note"`
}

type NoteData struct {
	ItemType  string `json:"itemType"`
	Text      string `json:"text"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type NoteResponse struct {
	Code int       `json:"code"`
	Msg  string    `json:"msg,omitempty"`
	Data *NoteData `json:"data,omitempty"`
}
