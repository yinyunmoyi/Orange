package model

// ContextTaskRequest 创建并同步执行一个语境处理任务。
type ContextTaskRequest struct {
	ItemType       string `json:"itemType"`
	ItemID         int64  `json:"itemId"`
	Paragraph      string `json:"paragraph"`
	SelectionStart int    `json:"selectionStart"`
	SelectionEnd   int    `json:"selectionEnd"`
	Source         string `json:"source,omitempty"`
}

// ContextTaskData 返回任务的最终或当前状态。
type ContextTaskData struct {
	TaskID        int64  `json:"taskId"`
	Status        string `json:"status"`
	ContextID     *int64 `json:"contextId,omitempty"`
	SentenceStart *int   `json:"sentenceStart,omitempty"`
	SentenceEnd   *int   `json:"sentenceEnd,omitempty"`
	LastError     string `json:"lastError,omitempty"`
}

// ContextTaskResponse 语境任务接口响应。
type ContextTaskResponse struct {
	Code int              `json:"code"`
	Msg  string           `json:"msg,omitempty"`
	Data *ContextTaskData `json:"data,omitempty"`
}
