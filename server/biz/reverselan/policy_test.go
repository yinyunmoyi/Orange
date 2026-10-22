package reverselan

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestLearningPolicyAllowlist(t *testing.T) {
	registry := NewPolicyRegistry(LearningPolicies()...)
	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name:    "current",
			request: protocolRequest(http.MethodGet, "/api/v1/learning/sessions/1/current", ""),
			want:    "learning-current",
		},
		{
			name:    "answer",
			request: protocolRequest(http.MethodPost, "/api/v1/learning/sessions/42/answers", `{}`),
			want:    "learning-answer",
		},
		{
			name:    "mastery",
			request: protocolRequest(http.MethodPost, "/api/v1/learning/sessions/999/mastery", `{}`),
			want:    "learning-mastery",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := registry.Match(test.request)
			if err != nil {
				t.Fatal(err)
			}
			if policy.Name != test.want || policy.Lane != LaneInteractive {
				t.Fatalf("policy=%+v", policy)
			}
		})
	}
}

func TestLearningPolicyRejectsOpenProxyInputs(t *testing.T) {
	registry := NewPolicyRegistry(LearningPolicies()...)
	tests := []struct {
		name   string
		change func(*Request)
	}{
		{
			name: "absolute URL",
			change: func(value *Request) {
				value.Path = "http://169.254.169.254/api/v1/learning/sessions/1/current"
			},
		},
		{
			name: "query suffix",
			change: func(value *Request) {
				value.Path += "?redirect=http://example.com"
			},
		},
		{
			name: "unknown route",
			change: func(value *Request) {
				value.Path = "/api/v1/audio-sync/manifest"
			},
		},
		{
			name: "wrong lane",
			change: func(value *Request) {
				value.Lane = LaneBackground
			},
		},
		{
			name: "authorization header",
			change: func(value *Request) {
				value.Headers["Authorization"] = "secret"
			},
		},
		{
			name: "oversized body",
			change: func(value *Request) {
				value.Method = http.MethodPost
				value.Path = "/api/v1/learning/sessions/1/answers"
				value.Body = strings.Repeat("x", int(MaxCommandBodyBytes+1))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := protocolRequest(http.MethodGet, "/api/v1/learning/sessions/1/current", "")
			test.change(&request)
			if _, err := registry.Match(request); !errors.Is(err, ErrPolicyRejected) {
				t.Fatalf("error=%v, want policy rejection", err)
			}
		})
	}
}

func protocolRequest(method, path, body string) Request {
	return Request{
		APIVersion: APIVersion,
		RequestID:  "55555555-5555-4555-8555-555555555555",
		Lane:       LaneInteractive,
		Method:     method,
		Path:       path,
		Headers: map[string]string{
			"Content-Type":            "application/json",
			"X-Orange-Debug-Trace-Id": "trace-id",
		},
		Body: body,
	}
}
