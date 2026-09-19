package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"aaa_word/biz/config"
)

// translationSystemPrompt 流式翻译提示词：只吐译文本身，不做任何结构化输出
const translationSystemPrompt = `你是一名英语翻译。用户会给你一句英文句子，请直接返回该句子最自然、地道、通顺的中文翻译。
必须完整翻译整个句子，包括冒号、分号后的所有并列成分和补充说明，不要提前结束，不要省略任何部分，不要用"……"或"等等"代替。
只输出译文本身，不要额外解释，不要输出 JSON、不要输出 markdown 代码块，不要在译文前后加引号或标签。`

// dsStreamChoice 流式响应中每帧的 choice
type dsStreamChoice struct {
	Delta struct {
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
	} `json:"delta"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// dsStreamFrame 流式响应一帧的最外层
type dsStreamFrame struct {
	Choices []dsStreamChoice `json:"choices"`
}

// TranslateSentenceStream 以 SSE 流式方式从 DeepSeek 拉取汉语翻译。
// onDelta 会被多次回调，每次带上一次增量文本；上游结束或出错时函数返回。
// 该函数是同步阻塞的，调用方应在独立 goroutine 中使用。
func TranslateSentenceStream(sentence string, onDelta func(string)) error {
	apiKey := config.LLMAPIKey()
	if apiKey == "" {
		log.Printf("[translate] api key not configured sentence=%q", truncate(sentence, 200))
		return ErrLLMNotConfigured
	}

	reqBody := dsRequest{
		Model: config.LLMModel(),
		Messages: []dsMessage{
			{Role: "system", Content: translationSystemPrompt},
			{Role: "user", Content: fmt.Sprintf("句子：%s", sentence)},
		},
		Temperature: 0.3,
		MaxTokens:   4096,
		Stream:      true,
		Thinking:    &dsThinkingConfig{Type: "disabled"},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[translate] marshal req failed sentence=%q err=%v", truncate(sentence, 200), err)
		return fmt.Errorf("marshal translate request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, config.LLMAPIURL(), bytes.NewReader(payload))
	if err != nil {
		log.Printf("[translate] build req failed sentence=%q err=%v", truncate(sentence, 200), err)
		return fmt.Errorf("build translate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	started := time.Now()
	log.Printf("[translate] stream start sentence=%q model=%s max_tokens=%d",
		truncate(sentence, 200), reqBody.Model, reqBody.MaxTokens)

	resp, err := doLLMRequest(httpReq)
	if err != nil {
		log.Printf("[translate] call deepseek failed sentence=%q err=%v elapsed=%s",
			truncate(sentence, 200), err, time.Since(started))
		return fmt.Errorf("call deepseek stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		log.Printf("[translate] deepseek non-200 sentence=%q status=%d body=%q elapsed=%s",
			truncate(sentence, 200), resp.StatusCode, string(body), time.Since(started))
		return fmt.Errorf("deepseek status %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	deltaCount := 0
	var accum strings.Builder
	finishReason := ""

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[translate] read stream failed sentence=%q err=%v deltas=%d partial=%q elapsed=%s",
				truncate(sentence, 200), err, deltaCount, truncate(accum.String(), 300), time.Since(started))
			return fmt.Errorf("read stream: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var frame dsStreamFrame
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			log.Printf("[translate] parse frame failed sentence=%q err=%v payload=%q",
				truncate(sentence, 120), err, truncate(payload, 300))
			continue
		}
		if len(frame.Choices) == 0 {
			continue
		}
		if fr := frame.Choices[0].FinishReason; fr != "" {
			finishReason = fr
		}
		delta := frame.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		deltaCount++
		accum.WriteString(delta)
		onDelta(delta)
	}

	log.Printf("[translate] stream done sentence=%q deltas=%d translation=%q finish_reason=%s duration=%s",
		truncate(sentence, 200), deltaCount, truncate(accum.String(), 500), finishReason, time.Since(started))
	if deltaCount == 0 {
		return fmt.Errorf("translate empty finish_reason=%s", finishReason)
	}
	return nil
}

// TranslateSentence 收集流式翻译结果并返回完整译文。
func TranslateSentence(sentence string) (string, error) {
	var result strings.Builder
	if err := TranslateSentenceStream(sentence, func(delta string) {
		result.WriteString(delta)
	}); err != nil {
		return "", err
	}
	translation := strings.TrimSpace(result.String())
	if translation == "" {
		log.Printf("[translate] collected empty result sentence=%q", truncate(sentence, 200))
		return "", errors.New("translate returned empty result")
	}
	return translation, nil
}
