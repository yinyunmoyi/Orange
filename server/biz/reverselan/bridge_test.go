package reverselan

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"aaa_word/biz/discovery"
)

func TestBridgeRetriesCachedResultWithoutRepeatingLoopback(t *testing.T) {
	const unicodeBody = `{"data":{"phonetic":"/ˈpʌfəri/","chinese":"吹捧、夸大宣传","sentence":"She called it “puffery”—not evidence.","translation":"她称其为“吹嘘”，而非证据。","note":"puffery ≠ 欺诈"}}`
	var pollCalls atomic.Int32
	var resultCalls atomic.Int32
	android := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+validRegistration().Secret ||
			request.Header.Get(HeaderServerID) != testServerID ||
			request.Header.Get(HeaderGeneration) != testGeneration {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/reverse/capabilities":
			_ = json.NewEncoder(writer).Encode(Capabilities{
				APIVersion:   APIVersion,
				Capabilities: []string{CapabilityHTTPJSON},
				Lanes:        []string{LaneInteractive},
			})
		case "/v1/reverse/next":
			if pollCalls.Add(1) == 1 {
				_ = json.NewEncoder(writer).Encode(protocolRequest(
					http.MethodPost,
					"/api/v1/learning/sessions/8/answers",
					`{}`,
				))
				return
			}
			writer.WriteHeader(http.StatusUnauthorized)
		case "/v1/reverse/results/55555555-5555-4555-8555-555555555555":
			if got := request.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("result content type=%q", got)
			}
			var result Result
			if err := json.NewDecoder(request.Body).Decode(&result); err != nil {
				t.Errorf("decode result: %v", err)
			} else if result.Body != unicodeBody {
				t.Errorf("result body=%q", result.Body)
			}
			if resultCalls.Add(1) < 3 {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer android.Close()

	var loopbackCalls atomic.Int32
	loopback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		loopbackCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = writer.Write([]byte(unicodeBody))
	}))
	defer loopback.Close()

	forwarder := NewForwarder(8888, NewPolicyRegistry(LearningPolicies()...))
	forwarder.baseURL = loopback.URL
	item := &bridge{
		candidate: discovery.ReverseLanCandidate{
			ClientID:   testClientID,
			ServerID:   testServerID,
			Generation: testGeneration,
		},
		ticket:    mustTicket(t),
		forwarder: forwarder,
		client:    android.Client(),
		baseURLs:  []string{android.URL},
	}
	err := item.run(context.Background())
	if !errors.Is(err, errBridgeUnauthorized) {
		t.Fatalf("run error=%v", err)
	}
	if loopbackCalls.Load() != 1 {
		t.Fatalf("loopback calls=%d, want 1", loopbackCalls.Load())
	}
	if resultCalls.Load() != 3 {
		t.Fatalf("result calls=%d, want 3", resultCalls.Load())
	}
}

func mustTicket(t *testing.T) Ticket {
	t.Helper()
	registry := NewRegistry(testServerID, DefaultTicketTTL, DefaultTicketCapacity)
	ticket, err := registry.Register(validRegistration())
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}
