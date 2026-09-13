package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openDatabase(filepath.Join(t.TempDir(), "newsroom.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := seed(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFreshDatabasePragmasAndIdempotentSeed(t *testing.T) {
	db := testDatabase(t)
	assertAgentCount(t, db, 3)

	var foreignKeys, busyTimeout int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 || busyTimeout != 5000 {
		t.Fatalf("unexpected pragmas: foreign_keys=%d busy_timeout=%d", foreignKeys, busyTimeout)
	}
	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := seed(db); err != nil {
		t.Fatal(err)
	}
	assertAgentCount(t, db, 3)
}

func TestRestartPreservesExactlyThreeSeededAgents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "newsroom.db")
	db, err := openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := seed(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := seed(db); err != nil {
		t.Fatal(err)
	}
	assertAgentCount(t, db, 3)
}

func TestReadAPIs(t *testing.T) {
	db := testDatabase(t)
	handler := newAPI(db, []string{"http://localhost:8081"}, testLogger(), nil)

	t.Run("health", func(t *testing.T) {
		response := request(t, handler, http.MethodGet, "/health", "")
		if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\"}\n" {
			t.Fatalf("unexpected health response: %d %s", response.Code, response.Body.String())
		}
	})
	t.Run("agents", func(t *testing.T) {
		response := request(t, handler, http.MethodGet, "/api/v1/agents", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		var body struct {
			Agents []struct {
				ID string `json:"id"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Agents) != 3 {
			t.Fatalf("agent count = %d, want 3", len(body.Agents))
		}
	})
	t.Run("new conversation is empty", func(t *testing.T) {
		response := request(t, handler, http.MethodGet, "/api/v1/agents/bundesliga/messages", "")
		if response.Code != http.StatusOK || response.Body.String() != "{\"messages\":[]}\n" {
			t.Fatalf("unexpected messages response: %d %s", response.Code, response.Body.String())
		}
	})
	t.Run("unknown agent", func(t *testing.T) {
		response := request(t, handler, http.MethodGet, "/api/v1/agents/unknown/messages", "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
		var body map[string]map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["error"]["code"] != "not_found" {
			t.Fatalf("error code = %q, want not_found", body["error"]["code"])
		}
	})
	t.Run("configured CORS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("Origin", "http://localhost:8081")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8081" {
			t.Fatalf("missing configured CORS header")
		}
	})
}

func TestPatchDraft(t *testing.T) {
	db := testDatabase(t)
	firstID := seedDraftForReview(t, db, "draft_first", "Premier League", "Post Facebook original", "Post X original")
	secondID := seedDraftForReview(t, db, "draft_second", "Deuxième titre", "Deuxième Facebook", "Deuxième X")
	handler := newAPI(db, []string{"http://localhost:8081"}, testLogger(), nil)

	t.Run("partial edit preserves fields, sources, and conversation headline", func(t *testing.T) {
		response := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"headline":"Titre modifié"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		draft := decodePatchedDraft(t, response.Body.Bytes())
		if draft.ID != firstID || draft.AgentID != "premier_league" || draft.StoryID != "story_draft_first" || draft.RunID != "run_draft_first" || draft.CreatedAt != "2026-01-01T00:00:00Z" || draft.Headline != "Titre modifié" || draft.FacebookText != "Post Facebook original" || draft.XText != "Post X original" || draft.ReviewStatus != "pending" || len(draft.Sources) != 1 {
			t.Fatalf("unexpected partial update: %+v", draft)
		}
		messages := request(t, handler, http.MethodGet, "/api/v1/agents/premier_league/messages", "")
		if !strings.Contains(messages.Body.String(), `"text":"Titre modifié"`) || !strings.Contains(messages.Body.String(), `"headline":"Titre modifié"`) {
			t.Fatalf("conversation did not expose the current headline: %s", messages.Body.String())
		}
	})

	t.Run("approval and rejection change pending count and are live in conversation", func(t *testing.T) {
		approved := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"reviewStatus":"approved"}`)
		if approved.Code != http.StatusOK || decodePatchedDraft(t, approved.Body.Bytes()).ReviewStatus != "approved" {
			t.Fatalf("approval failed: %d %s", approved.Code, approved.Body.String())
		}
		if got := pendingDraftCount(t, handler); got != 1 {
			t.Fatalf("pending count after approval = %d, want 1", got)
		}
		conversation := request(t, handler, http.MethodGet, "/api/v1/agents/premier_league/messages", "")
		if !strings.Contains(conversation.Body.String(), `"id":"draft_first"`) || !strings.Contains(conversation.Body.String(), `"reviewStatus":"approved"`) {
			t.Fatalf("approved draft not current in conversation: %s", conversation.Body.String())
		}
		rejected := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+secondID, `{"reviewStatus":"rejected"}`)
		if rejected.Code != http.StatusOK || decodePatchedDraft(t, rejected.Body.Bytes()).ReviewStatus != "rejected" {
			t.Fatalf("rejection failed: %d %s", rejected.Code, rejected.Body.String())
		}
		if got := pendingDraftCount(t, handler); got != 0 {
			t.Fatalf("pending count after rejection = %d, want 0", got)
		}
	})

	t.Run("identical approval is idempotent and text edit returns approved draft to pending", func(t *testing.T) {
		before := decodePatchedDraft(t, request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"reviewStatus":"approved"}`).Body.Bytes())
		messageCount := countDraftMessages(t, db, firstID)
		again := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"reviewStatus":"approved"}`)
		after := decodePatchedDraft(t, again.Body.Bytes())
		if before.UpdatedAt != after.UpdatedAt || pendingDraftCount(t, handler) != 0 || countDraftMessages(t, db, firstID) != messageCount {
			t.Fatalf("identical approval changed state: before=%+v after=%+v", before, after)
		}
		edited := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"facebookText":"Post Facebook corrigé"}`)
		draft := decodePatchedDraft(t, edited.Body.Bytes())
		if draft.ReviewStatus != "pending" || draft.FacebookText != "Post Facebook corrigé" {
			t.Fatalf("text edit did not return draft to pending: %+v", draft)
		}
		if got := pendingDraftCount(t, handler); got != 1 {
			t.Fatalf("pending count after text edit = %d, want 1", got)
		}
		explicit := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"xText":"Post X corrigé","reviewStatus":"rejected"}`)
		if draft := decodePatchedDraft(t, explicit.Body.Bytes()); draft.ReviewStatus != "rejected" || draft.XText != "Post X corrigé" {
			t.Fatalf("explicit final status was not applied: %+v", draft)
		}
	})

	t.Run("invalid requests are rejected atomically and unknown drafts are 404", func(t *testing.T) {
		before := decodePatchedDraft(t, request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"reviewStatus":"rejected"}`).Body.Bytes())
		for _, body := range []string{
			`{}`,
			`{"headline":null}`,
			`{"facebookText":"   "}`,
			`{"reviewStatus":"published"}`,
			`{"unknown":"value"}`,
			`{"xText":"` + strings.Repeat("x", 281) + `"}`,
		} {
			response := request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid request status = %d for %s", response.Code, body)
			}
		}
		after := decodePatchedDraft(t, request(t, handler, http.MethodPatch, "/api/v1/drafts/"+firstID, `{"reviewStatus":"rejected"}`).Body.Bytes())
		if before.Headline != after.Headline || before.FacebookText != after.FacebookText || before.XText != after.XText || before.UpdatedAt != after.UpdatedAt {
			t.Fatalf("invalid request changed draft: before=%+v after=%+v", before, after)
		}
		missing := request(t, handler, http.MethodPatch, "/api/v1/drafts/missing", `{"reviewStatus":"approved"}`)
		if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"code":"not_found"`) {
			t.Fatalf("unknown draft response: %d %s", missing.Code, missing.Body.String())
		}
	})
}

type patchedDraft struct {
	ID           string                `json:"id"`
	AgentID      string                `json:"agentId"`
	StoryID      string                `json:"storyId"`
	RunID        string                `json:"runId"`
	Headline     string                `json:"headline"`
	FacebookText string                `json:"facebookText"`
	XText        string                `json:"xText"`
	ReviewStatus string                `json:"reviewStatus"`
	CreatedAt    string                `json:"createdAt"`
	UpdatedAt    string                `json:"updatedAt"`
	Sources      []struct{ ID string } `json:"sources"`
}

func decodePatchedDraft(t *testing.T, body []byte) patchedDraft {
	t.Helper()
	var response struct {
		Draft patchedDraft `json:"draft"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	return response.Draft
}

func pendingDraftCount(t *testing.T, handler http.Handler) int {
	t.Helper()
	response := request(t, handler, http.MethodGet, "/api/v1/agents", "")
	var body struct {
		Agents []struct {
			ID      string `json:"id"`
			Pending int    `json:"pendingDraftCount"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, agent := range body.Agents {
		if agent.ID == "premier_league" {
			return agent.Pending
		}
	}
	t.Fatal("premier_league agent missing")
	return 0
}

func countDraftMessages(t *testing.T, db *sql.DB, draftID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE draft_id=?`, draftID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func seedDraftForReview(t *testing.T, db *sql.DB, draftID, headline, facebookText, xText string) string {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	storyID, runID, sourceID := "story_"+draftID, "run_"+draftID, "source_"+draftID
	if _, err := db.Exec(`INSERT INTO runs (id, agent_id, status, started_at, ended_at) VALUES (?, 'premier_league', 'completed', ?, ?)`, runID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO stories (id, deduplication_key, title, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, storyID, storyID, headline, headline, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sources (id, story_id, url, title, retrieved_at) VALUES (?, ?, ?, 'Source de test', ?)`, sourceID, storyID, "https://example.test/"+draftID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO drafts (id, agent_id, story_id, run_id, headline, claim_status, facebook_text, x_text, review_status, created_at, updated_at) VALUES (?, 'premier_league', ?, ?, ?, 'reported', ?, ?, 'pending', ?, ?)`, draftID, storyID, runID, headline, facebookText, xText, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages (id, agent_id, role, message_type, text, draft_id, run_id, created_at) VALUES (?, 'premier_league', 'assistant', 'draft', ?, ?, ?, ?)`, "message_"+draftID, headline, draftID, runID, now); err != nil {
		t.Fatal(err)
	}
	return draftID
}

func assertAgentCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("agent count = %d, want %d", got, want)
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
