package config

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

func TestLearningDailyNewLimit(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "valid", raw: "12", want: 12},
		{name: "not number", raw: "many", want: 50},
		{name: "zero", raw: "0", want: 50},
		{name: "negative", raw: "-1", want: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LEARNING_DAILY_NEW_LIMIT", tt.raw)
			if got := LearningDailyNewLimit(); got != tt.want {
				t.Fatalf("LearningDailyNewLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTypedConfigParsing(t *testing.T) {
	t.Run("valid values", func(t *testing.T) {
		t.Setenv("SERVER_PORT", "9090")
		t.Setenv("SERVER_MAX_REQUEST_BODY_BYTES", "4096")
		t.Setenv("LLM_TIMEOUT_SECONDS", "45")
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example, https://b.example")

		if got := ServerPort(); got != 9090 {
			t.Fatalf("ServerPort() = %d, want 9090", got)
		}
		if got := ServerMaxRequestBodyBytes(); got != 4096 {
			t.Fatalf("ServerMaxRequestBodyBytes() = %d, want 4096", got)
		}
		if got := LLMTimeout(); got != 45*time.Second {
			t.Fatalf("LLMTimeout() = %s, want 45s", got)
		}
		if got := CORSAllowedOrigins(); len(got) != 2 || got[1] != "https://b.example" {
			t.Fatalf("CORSAllowedOrigins() = %#v", got)
		}
	})

	t.Run("invalid values use defaults without leaking value", func(t *testing.T) {
		const secretValue = "secret-invalid-port"
		t.Setenv("SERVER_PORT", secretValue)
		var output bytes.Buffer
		previous := log.Writer()
		log.SetOutput(&output)
		t.Cleanup(func() { log.SetOutput(previous) })

		if got := ServerPort(); got != 8888 {
			t.Fatalf("ServerPort() = %d, want 8888", got)
		}
		if strings.Contains(output.String(), secretValue) {
			t.Fatal("configuration log leaked invalid value")
		}
	})

	t.Run("invalid URL uses default", func(t *testing.T) {
		t.Setenv("LLM_API_URL", "file:///tmp/private")
		if got := LLMAPIURL(); got != "https://api.deepseek.com/chat/completions" {
			t.Fatalf("LLMAPIURL() = %q", got)
		}
	})
}

func TestEnvironmentOverridesLoadedValues(t *testing.T) {
	t.Setenv("CONFIG_PRIORITY_TEST", "environment")
	if got := Get("CONFIG_PRIORITY_TEST", "default"); got != "environment" {
		t.Fatalf("Get() = %q, want environment", got)
	}
}
