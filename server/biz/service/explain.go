package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/model"
)

// explainSystemPrompt AI 语境解释提示词：用一句自然汉语说明该词在此处的含义。
const explainSystemPrompt = `你是英语阅读理解助手。用户会给你：一个英语单词、一段语境（段落）。
用户会给你一个被【】标记的目标词，以及包含该词的英文语境。
你的任务不是翻译整句或整段，而是只围绕【】中的词，结合其所在局部结构和上下文，给出较详细的中文解释
要求：
1. 只输出汉字。
2、说明该词在句中的角色；如果它属于固定搭配、短语或修饰结构，要明确指出。
3、结合上下文说明它额外暗示了什么；如果没有明显暗示，就简洁说明其主要作用即可。
`

// llmExplainResult LLM 期望返回的结构
type llmExplainResult struct {
	Explanation string `json:"explanation"`
}

// GetContextualExplanation 调用 DeepSeek，返回单词在具体语境下的一句自然汉语解释
func GetContextualExplanation(word, ctxText string, start, end int) (*model.ExplainData, error) {
	apiKey := config.LLMAPIKey()
	if apiKey == "" {
		log.Printf("[explain] api key not configured word=%q ctx=%q", word, truncate(ctxText, 200))
		return nil, ErrLLMNotConfigured
	}

	// 高亮片段：用【】把目标词括起，帮模型精确锚定
	highlighted := highlightWord(ctxText, word, start, end)
	log.Printf("[explain] request word=%q start=%d end=%d ctx=%q highlighted=%q",
		word, start, end, truncate(ctxText, 300), truncate(highlighted, 300))

	userContent := fmt.Sprintf("目标单词：%s\n语境：%s", word, highlighted)

	reqBody := dsRequest{
		Model: config.LLMModel(),
		Messages: []dsMessage{
			{Role: "system", Content: explainSystemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature:    0.3,
		MaxTokens:      4096,
		Stream:         false,
		ResponseFormat: &dsRespFormat{Type: "text"},
		Thinking:       &dsThinkingConfig{Type: "disabled"},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[explain] marshal req failed word=%q err=%v", word, err)
		return nil, fmt.Errorf("marshal llm request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, config.LLMAPIURL(), bytes.NewReader(payload))
	if err != nil {
		log.Printf("[explain] build req failed word=%q err=%v", word, err)
		return nil, fmt.Errorf("build llm request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	t0 := time.Now()
	resp, err := doLLMRequest(httpReq)
	if err != nil {
		log.Printf("[explain] call deepseek failed word=%q err=%v elapsed=%s", word, err, time.Since(t0))
		return nil, fmt.Errorf("call deepseek: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		log.Printf("[explain] deepseek non-200 word=%q status=%d body=%q elapsed=%s",
			word, resp.StatusCode, string(body), time.Since(t0))
		return nil, fmt.Errorf("deepseek status %d", resp.StatusCode)
	}

	var dsResp dsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dsResp); err != nil {
		log.Printf("[explain] decode failed word=%q err=%v elapsed=%s", word, err, time.Since(t0))
		return nil, fmt.Errorf("decode deepseek response: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		log.Printf("[explain] empty choices word=%q usage=%+v elapsed=%s",
			word, dsResp.Usage, time.Since(t0))
		return nil, fmt.Errorf("deepseek returned no choices")
	}

	rawContent := dsResp.Choices[0].Message.Content
	finishReason := dsResp.Choices[0].FinishReason
	content := extractJSON(rawContent)
	explanation := strings.TrimSpace(content)
	if explanation == "" {
		log.Printf("[explain] empty explanation word=%q raw=%q finish_reason=%s usage=%+v elapsed=%s",
			word, truncate(rawContent, 500), finishReason, dsResp.Usage, time.Since(t0))
		return nil, ErrLLMBadOutput
	}
	log.Printf("[explain] ok word=%q explanation=%q finish_reason=%s usage=%+v elapsed=%s",
		word, truncate(explanation, 500), finishReason, dsResp.Usage, time.Since(t0))

	return &model.ExplainData{
		Word:        strings.ToLower(word),
		Explanation: explanation,
	}, nil
}

// highlightWord 用【】把目标词在 ctxText 中括起来。位置越界或不匹配时用 indexOf 兜底；再失败就返回原文。
func highlightWord(ctxText, word string, start, end int) string {
	if start >= 0 && end > start && end <= len(ctxText) &&
		strings.EqualFold(ctxText[start:end], word) {
		return ctxText[:start] + "【" + ctxText[start:end] + "】" + ctxText[end:]
	}
	// 兜底：大小写不敏感 indexOf
	idx := strings.Index(strings.ToLower(ctxText), strings.ToLower(word))
	if idx >= 0 {
		return ctxText[:idx] + "【" + ctxText[idx:idx+len(word)] + "】" + ctxText[idx+len(word):]
	}
	return ctxText
}
