package model

type LearningReviewForecastItem struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type LearningReviewForecastData struct {
	StartDate string                       `json:"startDate"`
	EndDate   string                       `json:"endDate"`
	Days      int                          `json:"days"`
	Total     int64                        `json:"total"`
	Items     []LearningReviewForecastItem `json:"items"`
}

type LearningReviewForecastResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg,omitempty"`
	Data *LearningReviewForecastData `json:"data,omitempty"`
}
