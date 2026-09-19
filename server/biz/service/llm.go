package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/model"
)

// llmClient 大模型专用 HTTP 客户端（超时比普通请求更长）
var llmClient = &http.Client{}

func doLLMRequest(req *http.Request) (*http.Response, error) {
	client := *llmClient
	client.Timeout = config.LLMTimeout()
	return client.Do(req)
}

// ErrLLMNotConfigured 未配置大模型 API Key
var ErrLLMNotConfigured = errors.New("llm api key not configured")

// ===== DeepSeek 请求/响应结构 =====

type dsMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type dsThinkingConfig struct {
	Type string `json:"type"`
}

type dsRequest struct {
	Model          string            `json:"model"`
	Messages       []dsMessage       `json:"messages"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens"`
	Stream         bool              `json:"stream"`
	ResponseFormat *dsRespFormat     `json:"response_format,omitempty"`
	Thinking       *dsThinkingConfig `json:"thinking,omitempty"`
}

type dsRespFormat struct {
	Type string `json:"type"`
}

type dsResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// llmMeaningResult 期望大模型返回的 JSON 结构
type llmMeaningResult struct {
	Meanings []model.ChineseMeaning `json:"meanings"`
}

// 提示词
const meaningSystemPrompt = `你是一部专业的英汉词典。用户会给你一个英语单词，你需要给出它的汉语释义。
要求：
1. 如果有多个义项，按在现代英语中的使用频率由高到低排序（最常用的放最前面）。
2. 每个义项都要尽量言简意赅，只给核心汉语释义，不要例句、不要解释。
3. 标注词性，用简写：n.、v.、adj.、adv.、prep.、conj.、pron.、num. 等；无法确定词性时留空字符串。
4. 严格只输出 JSON，不要输出任何多余文字或 markdown 代码块。
输出 JSON 格式如下：
{"meanings":[{"partOfSpeech":"n.","meaning":"信条、教义"},{"partOfSpeech":"v.","meaning":"……"}]}`

// GetChineseMeaning 调用 DeepSeek，返回单词的结构化汉语释义（按频率高到低）
func GetChineseMeaning(word string) (*model.MeaningData, error) {
	return GetChineseMeaningContext(context.Background(), word)
}

func GetChineseMeaningContext(ctx context.Context, word string) (*model.MeaningData, error) {
	apiKey := config.LLMAPIKey()
	if apiKey == "" {
		log.Printf("[llm.meaning] api key not configured word=%q", word)
		return nil, ErrLLMNotConfigured
	}

	reqBody := dsRequest{
		Model: config.LLMModel(),
		Messages: []dsMessage{
			{Role: "system", Content: meaningSystemPrompt},
			{Role: "user", Content: fmt.Sprintf("单词：%s", word)},
		},
		Temperature:    0.3, // 词典任务求稳定
		MaxTokens:      4096,
		Stream:         false,
		ResponseFormat: &dsRespFormat{Type: "json_object"},
		Thinking:       &dsThinkingConfig{Type: "disabled"},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[llm.meaning] marshal req failed word=%q err=%v", word, err)
		return nil, fmt.Errorf("marshal llm request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.LLMAPIURL(), bytes.NewReader(payload))
	if err != nil {
		log.Printf("[llm.meaning] build req failed word=%q err=%v", word, err)
		return nil, fmt.Errorf("build llm request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	t0 := time.Now()
	log.Printf("[llm.meaning] request word=%q model=%s max_tokens=%d temperature=%.2f",
		word, reqBody.Model, reqBody.MaxTokens, reqBody.Temperature)
	resp, err := doLLMRequest(httpReq)
	if err != nil {
		log.Printf("[llm.meaning] call deepseek failed word=%q err=%v elapsed=%s", word, err, time.Since(t0))
		return nil, fmt.Errorf("call deepseek: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := readBodySnippet(resp.Body, 2048)
		log.Printf("[llm.meaning] deepseek non-200 word=%q status=%d body=%q elapsed=%s",
			word, resp.StatusCode, body, time.Since(t0))
		return nil, fmt.Errorf("deepseek status %d", resp.StatusCode)
	}

	var dsResp dsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dsResp); err != nil {
		log.Printf("[llm.meaning] decode failed word=%q err=%v elapsed=%s", word, err, time.Since(t0))
		return nil, fmt.Errorf("decode deepseek response: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		log.Printf("[llm.meaning] empty choices word=%q usage=%+v elapsed=%s",
			word, dsResp.Usage, time.Since(t0))
		return nil, fmt.Errorf("deepseek returned no choices")
	}

	rawContent := dsResp.Choices[0].Message.Content
	finishReason := dsResp.Choices[0].FinishReason
	content := extractJSON(rawContent)
	var result llmMeaningResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 截断兜底：finish_reason=length 且末尾断在半个义项上，尝试抢救前面已经完整的 {...}
		if salvaged, ok := salvageTruncatedMeanings(rawContent); ok {
			var salvagedResult llmMeaningResult
			if err2 := json.Unmarshal([]byte(salvaged), &salvagedResult); err2 == nil && len(salvagedResult.Meanings) > 0 {
				log.Printf("[llm.meaning] salvaged truncated json word=%q salvaged_meanings=%d finish_reason=%s usage=%+v elapsed=%s",
					word, len(salvagedResult.Meanings), finishReason, dsResp.Usage, time.Since(t0))
				result = salvagedResult
				goto parsed
			}
		}
		log.Printf("[llm.meaning] parse json failed word=%q raw=%q extracted=%q finish_reason=%s usage=%+v err=%v elapsed=%s",
			word, truncate(rawContent, 500), truncate(content, 500), finishReason, dsResp.Usage, err, time.Since(t0))
		return nil, fmt.Errorf("parse llm content as json: %w", err)
	}
parsed:
	if len(result.Meanings) == 0 {
		log.Printf("[llm.meaning] no meanings in result word=%q content=%q finish_reason=%s usage=%+v elapsed=%s",
			word, truncate(content, 500), finishReason, dsResp.Usage, time.Since(t0))
		return nil, ErrNotFound
	}

	posSummary := make([]string, 0, len(result.Meanings))
	for _, m := range result.Meanings {
		posSummary = append(posSummary, fmt.Sprintf("%s|%s", m.PartOfSpeech, truncate(m.Meaning, 40)))
	}
	log.Printf("[llm.meaning] ok word=%q meanings=%v finish_reason=%s usage=%+v elapsed=%s",
		word, posSummary, finishReason, dsResp.Usage, time.Since(t0))

	return &model.MeaningData{
		Word:     strings.ToLower(word),
		Meanings: result.Meanings,
	}, nil
}

// phraseSystemPrompt 用于短语（含固定搭配、短语动词、介词短语等）
const phraseSystemPrompt = `你是一部专业的英汉词典。用户会给你一个英语短语（可能是短语动词、介词短语、固定搭配或其他多词组合），你需要给出这个短语作为整体的汉语释义。
要求：
1. 请给出整个短语的核心含义，不要逐词翻译。
2. 如果短语有多个常见含义，按使用频率由高到低排序（最常用的放最前面）；单一含义即返回一条。
3. 每条释义都要言简意赅，只给核心汉语释义，不要例句、不要解释。
4. partOfSpeech 字段可留空字符串，或填写用于描述短语类型的简短标签，例如 "短语动词"、"介词短语"、"固定搭配"、"习语" 等。
5. 严格只输出 JSON，不要输出任何多余文字或 markdown 代码块。
输出 JSON 格式如下：
{"meanings":[{"partOfSpeech":"短语动词","meaning":"期待、盼望"}]}`

// GetChinesePhraseMeaning 调用 DeepSeek，返回短语作为整体的结构化汉语释义
func GetChinesePhraseMeaning(phrase string) (*model.MeaningData, error) {
	return GetChinesePhraseMeaningContext(context.Background(), phrase)
}

func GetChinesePhraseMeaningContext(ctx context.Context, phrase string) (*model.MeaningData, error) {
	apiKey := config.LLMAPIKey()
	if apiKey == "" {
		log.Printf("[llm.phrase] api key not configured phrase=%q", phrase)
		return nil, ErrLLMNotConfigured
	}

	reqBody := dsRequest{
		Model: config.LLMModel(),
		Messages: []dsMessage{
			{Role: "system", Content: phraseSystemPrompt},
			{Role: "user", Content: fmt.Sprintf("短语:%s", phrase)},
		},
		Temperature:    0.3,
		MaxTokens:      2048,
		Stream:         false,
		ResponseFormat: &dsRespFormat{Type: "json_object"},
		Thinking:       &dsThinkingConfig{Type: "disabled"},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[llm.phrase] marshal req failed phrase=%q err=%v", phrase, err)
		return nil, fmt.Errorf("marshal llm request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.LLMAPIURL(), bytes.NewReader(payload))
	if err != nil {
		log.Printf("[llm.phrase] build req failed phrase=%q err=%v", phrase, err)
		return nil, fmt.Errorf("build llm request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	t0 := time.Now()
	log.Printf("[llm.phrase] request phrase=%q model=%s max_tokens=%d temperature=%.2f",
		phrase, reqBody.Model, reqBody.MaxTokens, reqBody.Temperature)
	resp, err := doLLMRequest(httpReq)
	if err != nil {
		log.Printf("[llm.phrase] call deepseek failed phrase=%q err=%v elapsed=%s", phrase, err, time.Since(t0))
		return nil, fmt.Errorf("call deepseek: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := readBodySnippet(resp.Body, 2048)
		log.Printf("[llm.phrase] deepseek non-200 phrase=%q status=%d body=%q elapsed=%s",
			phrase, resp.StatusCode, body, time.Since(t0))
		return nil, fmt.Errorf("deepseek status %d", resp.StatusCode)
	}

	var dsResp dsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dsResp); err != nil {
		log.Printf("[llm.phrase] decode failed phrase=%q err=%v elapsed=%s", phrase, err, time.Since(t0))
		return nil, fmt.Errorf("decode deepseek response: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		log.Printf("[llm.phrase] empty choices phrase=%q usage=%+v elapsed=%s",
			phrase, dsResp.Usage, time.Since(t0))
		return nil, fmt.Errorf("deepseek returned no choices")
	}

	rawContent := dsResp.Choices[0].Message.Content
	finishReason := dsResp.Choices[0].FinishReason
	content := extractJSON(rawContent)
	var result llmMeaningResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		if salvaged, ok := salvageTruncatedMeanings(rawContent); ok {
			var salvagedResult llmMeaningResult
			if err2 := json.Unmarshal([]byte(salvaged), &salvagedResult); err2 == nil && len(salvagedResult.Meanings) > 0 {
				log.Printf("[llm.phrase] salvaged truncated json phrase=%q salvaged_meanings=%d finish_reason=%s usage=%+v elapsed=%s",
					phrase, len(salvagedResult.Meanings), finishReason, dsResp.Usage, time.Since(t0))
				result = salvagedResult
				goto parsed
			}
		}
		log.Printf("[llm.phrase] parse json failed phrase=%q raw=%q extracted=%q finish_reason=%s usage=%+v err=%v elapsed=%s",
			phrase, truncate(rawContent, 500), truncate(content, 500), finishReason, dsResp.Usage, err, time.Since(t0))
		return nil, fmt.Errorf("parse llm content as json: %w", err)
	}
parsed:
	if len(result.Meanings) == 0 {
		log.Printf("[llm.phrase] no meanings in result phrase=%q content=%q finish_reason=%s usage=%+v elapsed=%s",
			phrase, truncate(content, 500), finishReason, dsResp.Usage, time.Since(t0))
		return nil, ErrNotFound
	}

	posSummary := make([]string, 0, len(result.Meanings))
	for _, m := range result.Meanings {
		posSummary = append(posSummary, fmt.Sprintf("%s|%s", m.PartOfSpeech, truncate(m.Meaning, 40)))
	}
	log.Printf("[llm.phrase] ok phrase=%q meanings=%v finish_reason=%s usage=%+v elapsed=%s",
		phrase, posSummary, finishReason, dsResp.Usage, time.Since(t0))

	return &model.MeaningData{
		Word:     strings.ToLower(phrase),
		Meanings: result.Meanings,
	}, nil
}

// extractJSON 去除大模型可能包裹的 markdown 代码块，取出其中的 JSON 主体
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// 去掉 ```json ... ``` 或 ``` ... ``` 包裹
	if after, ok := strings.CutPrefix(s, "```"); ok {
		s = after
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimPrefix(s, "JSON")
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	// 兜底：截取第一个 { 到最后一个 } 之间的内容
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// salvageTruncatedMeanings 当模型因 max_tokens 截断导致返回的 JSON 不完整时，
// 尝试从残缺字符串中抢救出前面已经完整闭合的 {...} 条目，拼成合法的 meanings JSON。
// 返回 ok=false 表示无法救出任何完整义项。
func salvageTruncatedMeanings(content string) (string, bool) {
	arrStart := strings.Index(content, "\"meanings\"")
	if arrStart < 0 {
		return "", false
	}
	bracket := strings.IndexByte(content[arrStart:], '[')
	if bracket < 0 {
		return "", false
	}
	body := content[arrStart+bracket+1:]

	// 逐字符扫描，收集深度归零（即完整闭合）的顶层 {...} 段
	var items []string
	depth := 0
	inStr := false
	escape := false
	itemStart := -1
	for i := 0; i < len(body); i++ {
		c := body[i]
		if escape {
			escape = false
			continue
		}
		if inStr {
			if c == '\\' {
				escape = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				itemStart = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && itemStart >= 0 {
				items = append(items, body[itemStart:i+1])
				itemStart = -1
			}
		case ']':
			if depth == 0 {
				goto done
			}
		}
	}
done:
	if len(items) == 0 {
		return "", false
	}
	return "{\"meanings\":[" + strings.Join(items, ",") + "]}", true
}
