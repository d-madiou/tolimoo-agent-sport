package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"newsroom/internal/domain"
)

func TestDraftUsesConfiguredModelAndValidatesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("unexpected request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "vendor/model" {
			t.Fatalf("model = %#v", body["model"])
		}
		if body["max_tokens"] != float64(4096) {
			t.Fatalf("max_tokens = %#v", body["max_tokens"])
		}
		format := body["response_format"].(map[string]any)
		if format["type"] != "json_schema" {
			t.Fatalf("response format = %#v", format)
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"outcome\":\"nothing_new\",\"reason\":\"No evidence\",\"drafts\":[]}"}}],"usage":{"prompt_tokens":21,"completion_tokens":13,"total_tokens":34}}`))
	}))
	defer server.Close()
	client := New("test-key", "vendor/model", server.Client())
	client.BaseURL = server.URL
	output, err := client.Draft(context.Background(), "Premier League", []domain.SourceEvidence{{ID: "source_1", URL: "https://example.com", Title: "Example", Content: "Evidence"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if output.Outcome != "nothing_new" {
		t.Fatalf("output = %#v", output)
	}
}

func TestDraftRejectsMalformedModelJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not json"}}]}`))
	}))
	defer server.Close()
	client := New("key", "vendor/model", server.Client())
	client.BaseURL = server.URL
	if _, err := client.Draft(context.Background(), "assignment", nil, nil); !errors.Is(err, ErrMalformedJSON) {
		t.Fatalf("expected malformed JSON error, got %v", err)
	}
}

func TestDraftRejectsTokenLimitedOutputBeforeJSONDecoding(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"{\"outcome\":\"drafts\""}}],"usage":{"prompt_tokens":100,"completion_tokens":4096,"total_tokens":4196}}`))
	}))
	defer server.Close()
	client := New("key", "vendor/model", server.Client())
	client.BaseURL = server.URL
	if _, err := client.Draft(context.Background(), "assignment", nil, nil); !errors.Is(err, ErrTruncatedOutput) {
		t.Fatalf("expected truncated output error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("truncated output made %d requests, want 1", requests)
	}
}
