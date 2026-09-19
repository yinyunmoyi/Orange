package service

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type dictionaryRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn dictionaryRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type unexpectedEOFReader struct{}

func (unexpectedEOFReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestLookupRetriesTruncatedCachedResponse_BitsUT(t *testing.T) {
	originalClient := httpClient
	originalSleep := dictionaryRetrySleep
	defer func() {
		httpClient = originalClient
		dictionaryRetrySleep = originalSleep
	}()
	dictionaryRetrySleep = func(time.Duration) {}

	requestCount := 0
	httpClient = &http.Client{Transport: dictionaryRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		switch requestCount {
		case 1:
			if req.URL.Query().Get("_cache_bust") != "" {
				t.Fatalf("first request unexpectedly bypassed cache: %s", req.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(unexpectedEOFReader{}),
			}, nil
		case 2:
			if req.URL.Query().Get("_cache_bust") == "" {
				t.Fatalf("retry request missing cache bust parameter: %s", req.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`[{"word":"masked","phonetics":[],"meanings":[]}]`,
				)),
			}, nil
		default:
			return nil, errors.New("unexpected extra dictionary request")
		}
	})}

	data, err := Lookup("masked")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("Lookup() requests = %d, want 2", requestCount)
	}
	if data == nil || data.Word != "masked" {
		t.Fatalf("Lookup() data = %+v", data)
	}
}

func TestLookupRetriesTransient502Twice_BitsUT(t *testing.T) {
	originalClient := httpClient
	originalSleep := dictionaryRetrySleep
	defer func() {
		httpClient = originalClient
		dictionaryRetrySleep = originalSleep
	}()
	dictionaryRetrySleep = func(time.Duration) {}

	requestCount := 0
	httpClient = &http.Client{Transport: dictionaryRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		if requestCount > 1 && req.URL.Query().Get("_cache_bust") == "" {
			t.Fatalf("retry request missing cache bust parameter: %s", req.URL)
		}
		if requestCount < 3 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("bad gateway")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`[{"word":"brutishly","phonetics":[],"meanings":[]}]`,
			)),
		}, nil
	})}

	data, err := Lookup("brutishly")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if requestCount != 3 || data == nil || data.Word != "brutishly" {
		t.Fatalf("requests=%d data=%+v", requestCount, data)
	}
}

func TestLookupStopsAfterThreeTransientFailures_BitsUT(t *testing.T) {
	originalClient := httpClient
	originalSleep := dictionaryRetrySleep
	defer func() {
		httpClient = originalClient
		dictionaryRetrySleep = originalSleep
	}()
	dictionaryRetrySleep = func(time.Duration) {}

	requestCount := 0
	httpClient = &http.Client{Transport: dictionaryRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		}, nil
	})}

	data, err := LookupWithFallback("temporarily-unavailable")
	if err == nil {
		t.Fatal("LookupWithFallback() expected the exhausted retry error")
	}
	if requestCount != dictionaryMaxAttempts {
		t.Fatalf("LookupWithFallback() requests = %d, want %d", requestCount, dictionaryMaxAttempts)
	}
	if data == nil || data.Word != "temporarily-unavailable" ||
		len(data.Pronunciations) != 1 || data.Pronunciations[0].Accent != "US" {
		t.Fatalf("LookupWithFallback() data = %+v, want US TTS fallback", data)
	}
}

func TestLookupRetriesMalformedJSON_BitsUT(t *testing.T) {
	originalClient := httpClient
	originalSleep := dictionaryRetrySleep
	defer func() {
		httpClient = originalClient
		dictionaryRetrySleep = originalSleep
	}()
	dictionaryRetrySleep = func(time.Duration) {}

	requestCount := 0
	httpClient = &http.Client{Transport: dictionaryRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		body := `[{"word":"masked"`
		if requestCount == 2 {
			body = `[{"word":"masked","phonetics":[],"meanings":[]}]`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	data, err := Lookup("masked")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if requestCount != 2 || data == nil || data.Word != "masked" {
		t.Fatalf("requests=%d data=%+v", requestCount, data)
	}
}
