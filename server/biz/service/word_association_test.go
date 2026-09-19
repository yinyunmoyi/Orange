package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"aaa_word/biz/model"
)

func TestSanitizeAssociationItems(t *testing.T) {
	raw := []model.WordAssociationItem{
		{Word: "affect", TopMeaningText: "目标词", Similarity: 1},
		{Word: " Affection ", TopMeaningText: " 喜爱 ", Similarity: 0.7},
		{Word: "affection", TopMeaningText: "感情", Similarity: 0.95},
		{Word: "effect", TopMeaningText: "效果", Similarity: 1.4},
		{Word: "invalid word", TopMeaningText: "非法", Similarity: 0.9},
		{Word: "empty", TopMeaningText: " ", Similarity: 0.9},
		{Word: "afflict", TopMeaningText: "折磨", Similarity: -0.2},
		{Word: "affix", TopMeaningText: "附加", Similarity: 0.8},
		{Word: "effective", TopMeaningText: "有效的", Similarity: 0.79},
		{Word: "affected", TopMeaningText: "受影响的", Similarity: 0.78},
		{Word: "affable", TopMeaningText: "和蔼的", Similarity: 0.77},
		{Word: "efface", TopMeaningText: "抹去", Similarity: 0.76},
		{Word: "affair", TopMeaningText: "事情", Similarity: 0.75},
		{Word: "afford", TopMeaningText: "负担得起", Similarity: 0.74},
		{Word: "effort", TopMeaningText: "努力", Similarity: 0.73},
		{Word: "affiliate", TopMeaningText: "附属", Similarity: 0.72},
	}

	items := sanitizeAssociationItems("affect", raw)
	if len(items) != 10 {
		t.Fatalf("len(items) = %d, want 10", len(items))
	}
	if items[0].Word != "effect" || items[0].Similarity != 1 {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1].Word != "affection" || items[1].Similarity != 0.95 ||
		items[1].TopMeaningText != "感情" {
		t.Fatalf("deduplicated item = %+v", items[1])
	}
	for index := 1; index < len(items); index++ {
		if items[index-1].Similarity < items[index].Similarity {
			t.Fatalf("items not sorted at %d: %+v", index, items)
		}
	}
}

func TestAssociateWordsSendsHintsAndParsesResponse(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body dsRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Messages) != 2 {
			t.Fatalf("messages = %+v", body.Messages)
		}
		var input map[string]string
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &input); err != nil {
			t.Fatalf("decode user input: %v", err)
		}
		if input["word"] != "affect" || input["meaningHint"] != "情感" ||
			input["spellingHint"] != "aff" {
			t.Fatalf("input = %+v", input)
		}
		response := `{
			"choices":[{
				"message":{"content":"{\"items\":[{\"word\":\"affection\",\"topMeaningText\":\"喜爱\",\"similarity\":0.9}]}"},
				"finish_reason":"stop"
			}],
			"usage":{"prompt_tokens":10,"completion_tokens":8,"total_tokens":18}
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(response)),
		}, nil
	})}

	data, err := AssociateWords(context.Background(), " Affect ", " 情感 ", " aff ")
	if err != nil {
		t.Fatalf("AssociateWords error = %v", err)
	}
	if len(data.Items) != 1 || data.Items[0].Word != "affection" {
		t.Fatalf("data = %+v", data)
	}
}

func TestAssociateWordsAllowsEmptyHints(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body dsRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body.Messages[1].Content, `"meaningHint":""`) ||
			!strings.Contains(body.Messages[1].Content, `"spellingHint":""`) {
			t.Fatalf("user input = %s", body.Messages[1].Content)
		}
		response := `{"choices":[{"message":{"content":"{\"items\":[]}"},"finish_reason":"stop"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(response)),
		}, nil
	})}

	data, err := AssociateWords(context.Background(), "affect", "", "")
	if err != nil || data == nil || len(data.Items) != 0 {
		t.Fatalf("AssociateWords = (%+v, %v)", data, err)
	}
}

func TestAssociateWordsRejectsInvalidLLMOutput(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		response := `{"choices":[{"message":{"content":"not-json"},"finish_reason":"stop"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(response)),
		}, nil
	})}

	if _, err := AssociateWords(context.Background(), "affect", "", ""); err == nil {
		t.Fatal("AssociateWords error = nil")
	}
}

func TestAssociateWordsRejectsEmptyChoices(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"choices":[]}`)),
		}, nil
	})}

	if _, err := AssociateWords(context.Background(), "affect", "", ""); err == nil {
		t.Fatal("AssociateWords error = nil")
	}
}

func TestAssociateWordsRejectsNonOKResponse(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		}, nil
	})}

	if _, err := AssociateWords(context.Background(), "affect", "", ""); err == nil {
		t.Fatal("AssociateWords error = nil")
	}
}

func TestAssociateWordsHonorsCancellation(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := AssociateWords(ctx, "affect", "", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AssociateWords error = %v, want context canceled", err)
	}
}
