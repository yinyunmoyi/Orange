package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type llmRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn llmRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestGetChineseMeaningContextHonorsCancellation_BitsUT(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	originalClient := llmClient
	defer func() { llmClient = originalClient }()

	llmClient = &http.Client{Transport: llmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetChineseMeaningContext(ctx, "pedantic")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetChineseMeaningContext() error = %v, want context canceled", err)
	}
}
