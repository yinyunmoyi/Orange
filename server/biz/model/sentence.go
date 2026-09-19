package model

// ===== 长难句分析接口 =====

// SentenceAnalyzeRequest 句子分析请求体
type SentenceAnalyzeRequest struct {
	Sentence string `json:"sentence"`
}

// SentenceChunk 句子结构块
type SentenceChunk struct {
	Text        string `json:"text"`        // 原句中的连续子串（后端会强制对齐到原句）
	Type        string `json:"type"`        // main / modifier / adverbial / parenthetical / participle / quote / other
	Role        string `json:"role"`        // 简短中文说明（模型自由填）
	Translation string `json:"translation"` // 该块的中文翻译
}

// SentenceAnalysis 分析结果（仅结构 + chunks；翻译走独立 SSE 接口）
type SentenceAnalysis struct {
	Sentence  string          `json:"sentence"`
	Structure string          `json:"structure,omitempty"`
	Chunks    []SentenceChunk `json:"chunks"`
}

// SentenceAnalyzeResponse 响应包裹
type SentenceAnalyzeResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg,omitempty"`
	Data *SentenceAnalysis `json:"data,omitempty"`
}
