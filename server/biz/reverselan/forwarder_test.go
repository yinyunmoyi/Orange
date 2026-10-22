package reverselan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestForwarderUsesFixedLoopbackTargetAndAllowedHeaders(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/api/v1/learning/sessions/8/answers" {
			t.Fatalf("path=%q", request.URL.Path)
		}
		if request.Header.Get("X-Orange-Debug-Trace-Id") != "trace-one" {
			t.Fatalf("trace=%q", request.Header.Get("X-Orange-Debug-Trace-Id"))
		}
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
			t.Fatal("sensitive header reached loopback")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"code":409}`))
	}))
	defer target.Close()

	forwarder := NewForwarder(8888, NewPolicyRegistry(LearningPolicies()...))
	forwarder.baseURL = target.URL
	request := protocolRequest(http.MethodPost, "/api/v1/learning/sessions/8/answers", `{}`)
	request.Headers = map[string]string{
		"Content-Type":            "application/json",
		"X-Orange-Debug-Trace-Id": "trace-one",
	}
	result, err := forwarder.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusConflict || result.Body != `{"code":409}` {
		t.Fatalf("result=%+v", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("loopback calls=%d", calls.Load())
	}
}

func TestForwarderDoesNotFollowRedirect(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	defer destination.Close()
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", destination.URL)
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer target.Close()

	forwarder := NewForwarder(8888, NewPolicyRegistry(LearningPolicies()...))
	forwarder.baseURL = target.URL
	result, err := forwarder.Forward(
		context.Background(),
		protocolRequest(http.MethodGet, "/api/v1/learning/sessions/8/current", ""),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d", result.Status)
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect destination was called")
	}
}
