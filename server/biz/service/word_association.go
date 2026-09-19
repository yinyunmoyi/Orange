package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/model"
)

const wordAssociationLimit = 10

var associationWordPattern = regexp.MustCompile(`^[a-z][a-z'-]*$`)

const wordAssociationSystemPrompt = `你是一名英语词汇专家。用户会提供一个目标英语单词，以及可选的意思特征和拼写特征。请联想最容易与目标词混淆或联系起来的真实英语单词。
要求：
1. 同时考虑语义和拼写相似度；某项特征为空时，不要为该维度附加限制。
2. 不要返回目标词本身，不要返回短语、专有名词、变形拼写或虚构词。
3. 最多返回 10 个候选词，按综合相似度从高到低排列。
4. word 必须是小写英语单词；topMeaningText 是最常见且简洁的中文含义；similarity 是 0 到 1 的数字。
5. 严格只输出 JSON object，不要输出 markdown 或额外文字。
输出格式：
{"items":[{"word":"affection","topMeaningText":"喜爱；感情","similarity":0.94}]}`

type wordAssociationLLMResult struct {
	Items []model.WordAssociationItem `json:"items"`
}

type indexedAssociationItem struct {
	item  model.WordAssociationItem
	index int
}

func AssociateWords(
	ctx context.Context,
	word string,
	meaningHint string,
	spellingHint string,
) (*model.WordAssociationData, error) {
	apiKey := config.LLMAPIKey()
	if apiKey == "" {
		log.Printf("[llm.association] api key not configured word=%q", word)
		return nil, ErrLLMNotConfigured
	}

	normalizedWord := strings.ToLower(strings.TrimSpace(word))
	userInput, err := json.Marshal(map[string]string{
		"word":         normalizedWord,
		"meaningHint":  strings.TrimSpace(meaningHint),
		"spellingHint": strings.TrimSpace(spellingHint),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal association input: %w", err)
	}
	reqBody := dsRequest{
		Model: config.LLMModel(),
		Messages: []dsMessage{
			{Role: "system", Content: wordAssociationSystemPrompt},
			{Role: "user", Content: string(userInput)},
		},
		Temperature:    0.4,
		MaxTokens:      2048,
		Stream:         false,
		ResponseFormat: &dsRespFormat{Type: "json_object"},
		Thinking:       &dsThinkingConfig{Type: "disabled"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal association request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.LLMAPIURL(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build association request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	started := time.Now()
	log.Printf(
		"[llm.association] request word=%q meaning_hint_runes=%d spelling_hint_runes=%d",
		normalizedWord,
		len([]rune(strings.TrimSpace(meaningHint))),
		len([]rune(strings.TrimSpace(spellingHint))),
	)
	resp, err := doLLMRequest(httpReq)
	if err != nil {
		log.Printf("[llm.association] call failed word=%q err=%v elapsed=%s",
			normalizedWord, err, time.Since(started))
		return nil, fmt.Errorf("call deepseek association: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body := readBodySnippet(resp.Body, 2048)
		log.Printf("[llm.association] non-200 word=%q status=%d body=%q elapsed=%s",
			normalizedWord, resp.StatusCode, body, time.Since(started))
		return nil, fmt.Errorf("deepseek association status %d", resp.StatusCode)
	}

	var dsResp dsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dsResp); err != nil {
		return nil, fmt.Errorf("decode association response: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		return nil, fmt.Errorf("deepseek association returned no choices")
	}
	content := extractJSON(dsResp.Choices[0].Message.Content)
	var raw wordAssociationLLMResult
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		log.Printf("[llm.association] parse failed word=%q content=%q err=%v elapsed=%s",
			normalizedWord, truncate(content, 500), err, time.Since(started))
		return nil, fmt.Errorf("parse association content: %w", err)
	}

	items := sanitizeAssociationItems(normalizedWord, raw.Items)
	summary := make([]string, 0, len(items))
	for _, item := range items {
		summary = append(summary, fmt.Sprintf("%s:%.3f", item.Word, item.Similarity))
	}
	log.Printf("[llm.association] ok word=%q items=%v usage=%+v elapsed=%s",
		normalizedWord, summary, dsResp.Usage, time.Since(started))
	return &model.WordAssociationData{Items: items}, nil
}

func sanitizeAssociationItems(
	target string,
	raw []model.WordAssociationItem,
) []model.WordAssociationItem {
	target = strings.ToLower(strings.TrimSpace(target))
	byWord := make(map[string]indexedAssociationItem, len(raw))
	for index, candidate := range raw {
		word := strings.ToLower(strings.TrimSpace(candidate.Word))
		meaning := strings.TrimSpace(candidate.TopMeaningText)
		if word == target || meaning == "" || !associationWordPattern.MatchString(word) {
			continue
		}
		score := candidate.Similarity
		if score < 0 {
			score = 0
		} else if score > 1 {
			score = 1
		}
		item := model.WordAssociationItem{
			Word:           word,
			TopMeaningText: meaning,
			Similarity:     score,
		}
		existing, exists := byWord[word]
		if !exists || score > existing.item.Similarity {
			if exists {
				index = existing.index
			}
			byWord[word] = indexedAssociationItem{item: item, index: index}
		}
	}

	indexed := make([]indexedAssociationItem, 0, len(byWord))
	for _, item := range byWord {
		indexed = append(indexed, item)
	}
	sort.SliceStable(indexed, func(i, j int) bool {
		if indexed[i].item.Similarity == indexed[j].item.Similarity {
			return indexed[i].index < indexed[j].index
		}
		return indexed[i].item.Similarity > indexed[j].item.Similarity
	})
	if len(indexed) > wordAssociationLimit {
		indexed = indexed[:wordAssociationLimit]
	}
	items := make([]model.WordAssociationItem, 0, len(indexed))
	for _, item := range indexed {
		items = append(items, item.item)
	}
	return items
}
