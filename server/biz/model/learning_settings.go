package model

type LearningSettingsRequest struct {
	DailyNewLimit    int `json:"dailyNewLimit"`
	DailyReviewLimit int `json:"dailyReviewLimit"`
}

type LearningSettingsData struct {
	DailyNewLimit     int    `json:"dailyNewLimit"`
	DailyReviewLimit  int    `json:"dailyReviewLimit"`
	RemainingNewCount int64  `json:"remainingNewCount"`
	StudyDate         string `json:"studyDate"`
}

type LearningSettingsResponse struct {
	Code int                   `json:"code"`
	Msg  string                `json:"msg,omitempty"`
	Data *LearningSettingsData `json:"data,omitempty"`
}
