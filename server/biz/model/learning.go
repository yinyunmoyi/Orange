package model

type LearningStatusCounts struct {
	NotStarted int64 `json:"notStarted"`
	Learning   int64 `json:"learning"`
	Learned    int64 `json:"learned"`
	Mastered   int64 `json:"mastered"`
}

type LearningPlanData struct {
	SessionID      int64                `json:"sessionId"`
	SessionStatus  string               `json:"sessionStatus"`
	TodayTotal     int                  `json:"todayTotal"`
	TodayNew       int                  `json:"todayNew"`
	TodayReview    int                  `json:"todayReview"`
	TodayRetry     int                  `json:"todayRetry"`
	TotalItems     int64                `json:"totalItems"`
	StatusCounts   LearningStatusCounts `json:"statusCounts"`
	HasCurrentCard bool                 `json:"hasCurrentCard"`
	CanExtend      bool                 `json:"canExtend"`
}

type LearningMeaning struct {
	PartOfSpeech string `json:"partOfSpeech"`
	Text         string `json:"text"`
}

type LearningCard struct {
	ItemType int               `json:"itemType"`
	ItemID   int64             `json:"itemId"`
	Word     string            `json:"word"`
	Level    int               `json:"level,omitempty"`
	Phonetic string            `json:"phonetic"`
	AudioURL string            `json:"audioUrl"`
	Chinese  []LearningMeaning `json:"chinese"`
	Contexts []Context         `json:"contexts"`
	English  []LearningMeaning `json:"english"`
	Note     string            `json:"note"`

	// 词条级元数据（ECDICT，未命中或短语时为零值）
	Pos     string   `json:"pos,omitempty"`
	Collins int      `json:"collins,omitempty"`
	Oxford  int      `json:"oxford,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type LearningCurrentData struct {
	SessionID   int64                   `json:"sessionId"`
	TurnNo      int                     `json:"turnNo"`
	QueueItemID int64                   `json:"queueItemId,omitempty"`
	QueueType   string                  `json:"queueType,omitempty"`
	Remaining   int64                   `json:"remaining"`
	Completed   bool                    `json:"completed"`
	Progress    LearningSessionProgress `json:"progress"`
	Card        *LearningCard           `json:"card,omitempty"`
}

type LearningSessionProgress struct {
	Total      int64 `json:"total"`
	NotStarted int64 `json:"notStarted"`
	InProgress int64 `json:"inProgress"`
	Completed  int64 `json:"completed"`
}

type LearningAnswerRequest struct {
	QueueItemID int64  `json:"queueItemId"`
	TurnNo      int    `json:"turnNo"`
	Answer      string `json:"answer"`
}

type LearningMasteryRequest struct {
	QueueItemID int64 `json:"queueItemId"`
	TurnNo      int   `json:"turnNo"`
}

type LearningPlanResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg,omitempty"`
	Data *LearningPlanData `json:"data,omitempty"`
}

type LearningCurrentResponse struct {
	Code int                  `json:"code"`
	Msg  string               `json:"msg,omitempty"`
	Data *LearningCurrentData `json:"data,omitempty"`
}
