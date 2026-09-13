package exa

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchUsesOfficialExaShapeAndBoundsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" || r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("unexpected request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["numResults"].(float64) != 3 || body["category"] != "news" {
			t.Fatalf("unexpected search body: %#v", body)
		}
		contents := body["contents"].(map[string]any)
		if _, ok := contents["text"]; !ok {
			t.Fatal("missing nested contents.text")
		}
		if _, ok := contents["highlights"]; !ok {
			t.Fatal("missing nested contents.highlights")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Report","url":"https://example.com/a","publishedDate":"2026-01-02","text":"evidence"}]}`))
	}))
	defer server.Close()
	client := New("test-key", server.Client())
	client.BaseURL = server.URL
	sources, err := client.Search(context.Background(), "Premier League news", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].PublishedAt == nil || sources[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected sources: %#v", sources)
	}
}

func TestSearchRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`not json`)) }))
	defer server.Close()
	client := New("key", server.Client())
	client.BaseURL = server.URL
	if _, err := client.Search(context.Background(), "query", 3); err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestSearchKeepsMissingPublicationDateNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"Undated","url":"https://example.com/undated","text":"evidence"}]}`))
	}))
	defer server.Close()
	client := New("key", server.Client())
	client.BaseURL = server.URL
	sources, err := client.Search(context.Background(), "query", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].PublishedAt != nil {
		t.Fatalf("missing publication date was not preserved as nil")
	}
}

func TestSearchReportsAuthenticationAndCancellation(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		defer server.Close()
		client := New("key", server.Client())
		client.BaseURL = server.URL
		client.MaxAttempts = 1
		if _, err := client.Search(context.Background(), "query", 1); err == nil || err.Error() != "Exa authentication failed (HTTP 401)" {
			t.Fatalf("unexpected authentication error: %v", err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		client := New("key", &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})})
		client.BaseURL = "http://exa.test"
		client.MaxAttempts = 1
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := client.Search(ctx, "query", 1)
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation error, got %v", err)
		}
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
