package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"newsroom/internal/agent"
	"newsroom/internal/domain"
)

type fakeResearcher struct {
	sources []domain.SourceEvidence
	err     error
	block   <-chan struct{}
}

func (f fakeResearcher) Search(ctx context.Context, _ string, _ int) ([]domain.SourceEvidence, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.sources, f.err
}

type fakeWriter struct {
	assessment          domain.Assessment
	output              domain.FinalOutput
	assessErr, draftErr error
}

func (f fakeWriter) Assess(context.Context, string, []domain.SourceEvidence, []domain.StoryHistory) (domain.Assessment, error) {
	return f.assessment, f.assessErr
}
func (f fakeWriter) Draft(context.Context, string, []domain.SourceEvidence, []domain.StoryHistory) (domain.FinalOutput, error) {
	return f.output, f.draftErr
}

func newWorkflowQueue(t *testing.T, db *sql.DB, researcher agent.Researcher, writer agent.Writer) *runQueue {
	t.Helper()
	queue := newRunQueue(context.Background(), repository{db: db}, agent.Workflow{Researcher: researcher, Writer: writer}, time.Second, 2, true, testLogger())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		queue.stop(ctx)
	})
	return queue
}
func testSource() domain.SourceEvidence {
	return domain.SourceEvidence{URL: "https://example.com/report", Title: "Official report", RetrievedAt: time.Now().UTC(), Content: "A supported development."}
}
func testDraft(sourceID string) domain.FinalOutput {
	return domain.FinalOutput{Outcome: "drafts", Drafts: []domain.DraftCandidate{{Headline: "Titre confirmé", ClaimStatus: "reported", FacebookText: "Texte Facebook sourcé.", XText: "Texte X sourcé.", SourceIDs: []string{sourceID}}}}
}

func TestSuccessfulResearchRunPersistsReadableDraft(t *testing.T) {
	db := testDatabase(t)
	queue := newWorkflowQueue(t, db, fakeResearcher{sources: []domain.SourceEvidence{testSource()}}, fakeWriter{assessment: domain.Assessment{Outcome: "final"}, output: testDraft("source_1")})
	run, err := queue.enqueue("premier_league")
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, run.ID, "completed")
	handler := newAPI(db, nil, testLogger(), queue)
	response := request(t, handler, "GET", "/api/v1/agents/premier_league/messages", "")
	if response.Code != 200 {
		t.Fatalf("messages status = %d", response.Code)
	}
	var body struct {
		Messages []struct {
			Draft *struct {
				Headline string `json:"headline"`
				Sources  []struct {
					URL string `json:"url"`
				} `json:"sources"`
			} `json:"draft"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 1 || body.Messages[0].Draft == nil || body.Messages[0].Draft.Headline != "Titre confirmé" || len(body.Messages[0].Draft.Sources) != 1 {
		t.Fatalf("draft was not exposed: %s", response.Body.String())
	}
}

func TestRunRequestWithoutCredentialsDoesNotCreateRun(t *testing.T) {
	db := testDatabase(t)
	handler := newAPI(db, nil, testLogger(), nil)
	response := request(t, handler, "POST", "/api/v1/agents/premier_league/runs", "")
	if response.Code != 503 || !strings.Contains(response.Body.String(), `"configuration_error"`) {
		t.Fatalf("unexpected configuration response: %d %s", response.Code, response.Body.String())
	}
	var runs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("configuration error created %d runs", runs)
	}
}

func TestWorkflowRejectsInventedSourceID(t *testing.T) {
	workflow := agent.Workflow{Researcher: fakeResearcher{sources: []domain.SourceEvidence{testSource()}}, Writer: fakeWriter{assessment: domain.Assessment{Outcome: "final"}, output: testDraft("invented_source")}}
	_, err := workflow.Run(context.Background(), "Premier League news", nil, map[string]bool{})
	if err == nil {
		t.Fatal("expected unknown source ID to fail")
	}
}

func TestWorkflowRejectsOfficialSourceOutsideDraftEvidence(t *testing.T) {
	workflow := agent.Workflow{
		Researcher: fakeResearcher{sources: []domain.SourceEvidence{testSource(), {URL: "https://example.com/other", Title: "Other", RetrievedAt: time.Now().UTC(), Content: "Other evidence."}}},
		Writer:     fakeWriter{assessment: domain.Assessment{Outcome: "final"}, output: domain.FinalOutput{Outcome: "drafts", Drafts: []domain.DraftCandidate{{Headline: "Titre", ClaimStatus: "official", FacebookText: "Texte Facebook", XText: "Texte X", SourceIDs: []string{"source_1"}, OfficialSourceIDs: []string{"source_2"}}}}},
	}
	if _, err := workflow.Run(context.Background(), "Premier League news", nil, map[string]bool{}); err == nil {
		t.Fatal("expected official source outside draft evidence to fail")
	}
}

func TestDuplicateActiveRunsArePrevented(t *testing.T) {
	db := testDatabase(t)
	release := make(chan struct{})
	queue := newWorkflowQueue(t, db, fakeResearcher{block: release}, fakeWriter{})
	if _, err := queue.enqueue("premier_league"); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.enqueue("premier_league"); !errors.Is(err, errActiveRun) {
		t.Fatalf("error = %v, want active run conflict", err)
	}
	close(release)
}

func TestRunEndpointReturnsConflictForDuplicateActiveRun(t *testing.T) {
	db := testDatabase(t)
	release := make(chan struct{})
	queue := newWorkflowQueue(t, db, fakeResearcher{block: release}, fakeWriter{})
	handler := newAPI(db, nil, testLogger(), queue)
	first := request(t, handler, "POST", "/api/v1/agents/premier_league/runs", "")
	if first.Code != 202 {
		t.Fatalf("first run status = %d", first.Code)
	}
	second := request(t, handler, "POST", "/api/v1/agents/premier_league/runs", "")
	if second.Code != 409 || !strings.Contains(second.Body.String(), `"conflict"`) {
		t.Fatalf("unexpected conflict response: %d %s", second.Code, second.Body.String())
	}
	close(release)
}

func TestProviderFailureMarksRunFailed(t *testing.T) {
	db := testDatabase(t)
	queue := newWorkflowQueue(t, db, fakeResearcher{err: context.DeadlineExceeded}, fakeWriter{})
	run, err := queue.enqueue("premier_league")
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, run.ID, "failed")
	var drafts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM drafts WHERE run_id=?`, run.ID).Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if drafts != 0 {
		t.Fatalf("failed run has %d drafts", drafts)
	}
}

func TestNothingNewCompletesWithoutDraft(t *testing.T) {
	db := testDatabase(t)
	queue := newWorkflowQueue(t, db, fakeResearcher{sources: []domain.SourceEvidence{testSource()}}, fakeWriter{assessment: domain.Assessment{Outcome: "nothing_new", Reason: "No new evidence."}})
	run, err := queue.enqueue("premier_league")
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, run.ID, "completed")
	var drafts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM drafts WHERE run_id=?`, run.ID).Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if drafts != 0 {
		t.Fatalf("nothing-new run has %d drafts", drafts)
	}
}

func TestInterruptedRunsAreRecoveredOnStartup(t *testing.T) {
	db := testDatabase(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for index, status := range []string{"queued", "running"} {
		agentID := []string{"premier_league", "bundesliga"}[index]
		if _, err := db.Exec(`INSERT INTO runs (id,agent_id,status,started_at) VALUES (?,?,?,?)`, newID("run"), agentID, status, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := (repository{db: db}).recoverInterruptedRuns(); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs WHERE status IN ('queued','running')`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active interrupted runs = %d", active)
	}
}

func waitForRun(t *testing.T, db *sql.DB, runID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := db.QueryRow(`SELECT status FROM runs WHERE id=?`, runID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM runs WHERE id=?`, runID).Scan(&status)
	t.Fatalf("run status = %q, want %q", status, want)
}
