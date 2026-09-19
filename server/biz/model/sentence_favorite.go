package model

type SentenceFavoriteRequest struct {
	Sentence    string `json:"sentence"`
	Translation string `json:"translation"`
}

type SentenceTagData struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"createdAt"`
}

type SentenceFavoriteTagData struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type SentenceFavoriteData struct {
	ID          int64                     `json:"id"`
	Sentence    string                    `json:"sentence"`
	Translation string                    `json:"translation"`
	Note        string                    `json:"note"`
	CreatedAt   string                    `json:"createdAt"`
	Tags        []SentenceFavoriteTagData `json:"tags"`
}

type SentenceFavoriteStatusData struct {
	Favorited bool  `json:"favorited"`
	ID        int64 `json:"id,omitempty"`
}

type SentenceFavoriteListData struct {
	Items []SentenceFavoriteData `json:"items"`
	Total int64                  `json:"total"`
}

type SentenceFavoriteResponse struct {
	Code int                   `json:"code"`
	Msg  string                `json:"msg,omitempty"`
	Data *SentenceFavoriteData `json:"data,omitempty"`
}

type SentenceFavoriteStatusResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg,omitempty"`
	Data *SentenceFavoriteStatusData `json:"data,omitempty"`
}

type SentenceFavoriteListResponse struct {
	Code int                       `json:"code"`
	Msg  string                    `json:"msg,omitempty"`
	Data *SentenceFavoriteListData `json:"data,omitempty"`
}

type SentenceTagCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type SentenceFavoriteTagsUpdateRequest struct {
	TagIDs []int64 `json:"tagIds"`
	Note   string  `json:"note"`
}

type SentenceTagListData struct {
	Items []SentenceTagData `json:"items"`
	Total int64             `json:"total"`
}

type SentenceTagResponse struct {
	Code int              `json:"code"`
	Msg  string           `json:"msg,omitempty"`
	Data *SentenceTagData `json:"data,omitempty"`
}

type SentenceTagListResponse struct {
	Code int                  `json:"code"`
	Msg  string               `json:"msg,omitempty"`
	Data *SentenceTagListData `json:"data,omitempty"`
}
