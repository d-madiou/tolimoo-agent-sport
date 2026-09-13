package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"newsroom/internal/agent"
	"newsroom/internal/domain"
)

var (
	errActiveRun     = errors.New("agent already has an active run")
	errQueueFull     = errors.New("research queue is full")
	errNotConfigured = errors.New("research providers are not configured")
	errUnknownAgent  = errors.New("agent not found")
)

type repository struct{ db *sql.DB }

func (r repository) createQueuedRun(agentID string) (runRecord, error) {
	var exists int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id = ?`, agentID).Scan(&exists); err != nil {
		return runRecord{}, err
	}
	if exists == 0 {
		return runRecord{}, errUnknownAgent
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	run := runRecord{ID: newID("run"), AgentID: agentID, Status: "queued", StartedAt: now}
	_, err := r.db.Exec(`INSERT INTO runs (id, agent_id, status, started_at) VALUES (?, ?, 'queued', ?)`, run.ID, run.AgentID, run.StartedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return runRecord{}, errActiveRun
		}
		return runRecord{}, err
	}
	return run, nil
}

func (r repository) recoverInterruptedRuns() error {
	_, err := r.db.Exec(`UPDATE runs SET status = 'failed', ended_at = ?, error = ? WHERE status IN ('queued', 'running')`, time.Now().UTC().Format(time.RFC3339Nano), "Interrupted by a previous process shutdown.")
	return err
}

func (r repository) markRunning(runID string) error {
	_, err := r.db.Exec(`UPDATE runs SET status='running' WHERE id=? AND status='queued'`, runID)
	return err
}
func (r repository) markFailed(runID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	var agentID, status string
	if err := tx.QueryRow(`SELECT agent_id, status FROM runs WHERE id=?`, runID).Scan(&agentID, &status); err != nil {
		_ = tx.Rollback()
		return err
	}
	if status != "queued" && status != "running" {
		return tx.Commit()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE runs SET status='failed', ended_at=?, error=? WHERE id=?`, now, "Research could not be completed.", runID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO messages (id,agent_id,role,message_type,text,run_id,created_at) VALUES (?,?,'assistant','status',?,?,?)`, newID("msg"), agentID, "Research could not be completed. Please try again later.", runID, now); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r repository) assignment(agentID string) (string, error) {
	var assignment string
	err := r.db.QueryRow(`SELECT assignment FROM agents WHERE id=?`, agentID).Scan(&assignment)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errUnknownAgent
	}
	return assignment, err
}

func (r repository) historyAndURLs() ([]domain.StoryHistory, map[string]bool, error) {
	rows, err := r.db.Query(`SELECT title, summary FROM stories ORDER BY updated_at DESC LIMIT 20`)
	if err != nil {
		return nil, nil, err
	}
	history := []domain.StoryHistory{}
	for rows.Next() {
		var item domain.StoryHistory
		if err := rows.Scan(&item.Title, &item.Summary); err != nil {
			return nil, nil, err
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	rows, err = r.db.Query(`SELECT url FROM sources ORDER BY retrieved_at DESC LIMIT 100`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	urls := map[string]bool{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, nil, err
		}
		urls[normalizeURL(raw)] = true
	}
	return history, urls, rows.Err()
}

func (r repository) completeRun(run runRecord, result agent.Result) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	rollback := func(cause error) error { _ = tx.Rollback(); return cause }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if result.NothingNew {
		if _, err := tx.Exec(`INSERT INTO messages (id,agent_id,role,message_type,text,run_id,created_at) VALUES (?,?, 'assistant','status',?,?,?)`, newID("msg"), run.AgentID, "No sufficiently supported new development was found.", run.ID, now); err != nil {
			return rollback(err)
		}
		_, err = tx.Exec(`UPDATE runs SET status='completed', ended_at=?, error=NULL WHERE id=? AND status='running'`, now, run.ID)
		if err != nil {
			return rollback(err)
		}
		return tx.Commit()
	}
	persisted := 0
	for _, draft := range result.Drafts {
		key := deduplicationKey(draft, result.Sources)
		var storyID string
		err := tx.QueryRow(`SELECT id FROM stories WHERE deduplication_key=?`, key).Scan(&storyID)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return rollback(err)
		}
		storyID = newID("story")
		summary := draft.Headline
		if len(summary) > 500 {
			summary = summary[:500]
		}
		if _, err := tx.Exec(`INSERT INTO stories (id,deduplication_key,title,summary,created_at,updated_at) VALUES (?,?,?,?,?,?)`, storyID, key, draft.Headline, summary, now, now); err != nil {
			return rollback(err)
		}
		for _, sourceID := range draft.SourceIDs {
			source := result.Sources[sourceID]
			if _, err := tx.Exec(`INSERT INTO sources (id,story_id,url,title,published_at,retrieved_at) VALUES (?,?,?,?,?,?)`, newID("src"), storyID, source.URL, source.Title, nullableTime(source.PublishedAt), source.RetrievedAt.UTC().Format(time.RFC3339Nano)); err != nil {
				return rollback(err)
			}
		}
		draftID := newID("draft")
		if _, err := tx.Exec(`INSERT INTO drafts (id,agent_id,story_id,run_id,headline,claim_status,facebook_text,x_text,review_status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?, 'pending',?,?)`, draftID, run.AgentID, storyID, run.ID, draft.Headline, draft.ClaimStatus, draft.FacebookText, draft.XText, now, now); err != nil {
			return rollback(err)
		}
		if _, err := tx.Exec(`INSERT INTO messages (id,agent_id,role,message_type,text,draft_id,run_id,created_at) VALUES (?,?, 'assistant','draft',?,?,?,?)`, newID("msg"), run.AgentID, draft.Headline, draftID, run.ID, now); err != nil {
			return rollback(err)
		}
		persisted++
	}
	if persisted == 0 {
		if _, err := tx.Exec(`INSERT INTO messages (id,agent_id,role,message_type,text,run_id,created_at) VALUES (?,?, 'assistant','status',?,?,?)`, newID("msg"), run.AgentID, "No sufficiently supported new development was found.", run.ID, now); err != nil {
			return rollback(err)
		}
	}
	if _, err := tx.Exec(`UPDATE runs SET status='completed', ended_at=?, error=NULL WHERE id=? AND status='running'`, now, run.ID); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
func normalizeURL(raw string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "/")
}
func deduplicationKey(draft domain.DraftCandidate, sources map[string]domain.SourceEvidence) string {
	source := ""
	if len(draft.SourceIDs) > 0 {
		source = normalizeURL(sources[draft.SourceIDs[0]].URL)
	}
	sum := sha256.Sum256([]byte(source + "\n" + strings.ToLower(strings.TrimSpace(draft.Headline))))
	return hex.EncodeToString(sum[:])
}
func newID(prefix string) string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}

type runRecord struct{ ID, AgentID, Status, StartedAt string }
type runQueue struct {
	repo       repository
	workflow   agent.Workflow
	timeout    time.Duration
	jobs       chan runRecord
	slots      chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	log        *slog.Logger
	configured bool
	mu         sync.Mutex
	stopped    bool
}

func newRunQueue(parent context.Context, repo repository, workflow agent.Workflow, timeout time.Duration, capacity int, configured bool, log *slog.Logger) *runQueue {
	ctx, cancel := context.WithCancel(parent)
	q := &runQueue{repo: repo, workflow: workflow, timeout: timeout, jobs: make(chan runRecord, capacity), slots: make(chan struct{}, capacity), ctx: ctx, cancel: cancel, log: log, configured: configured}
	q.wg.Add(1)
	go q.worker()
	return q
}

func (q *runQueue) enqueue(agentID string) (runRecord, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return runRecord{}, errQueueFull
	}
	if !q.configured {
		return runRecord{}, errNotConfigured
	}
	select {
	case q.slots <- struct{}{}:
	default:
		return runRecord{}, errQueueFull
	}
	run, err := q.repo.createQueuedRun(agentID)
	if err != nil {
		<-q.slots
		return runRecord{}, err
	}
	q.jobs <- run
	return run, nil
}
func (q *runQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case run := <-q.jobs:
			q.process(run)
			<-q.slots
		}
	}
}
func (q *runQueue) process(run runRecord) {
	if err := q.repo.markRunning(run.ID); err != nil {
		q.log.Error("mark run running", "run_id", run.ID, "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(q.ctx, q.timeout)
	defer cancel()
	assignment, err := q.repo.assignment(run.AgentID)
	if err == nil {
		var history []domain.StoryHistory
		var urls map[string]bool
		history, urls, err = q.repo.historyAndURLs()
		if err == nil {
			var result agent.Result
			result, err = q.workflow.Run(ctx, assignment, history, urls)
			if err == nil {
				err = q.repo.completeRun(run, result)
			}
		}
	}
	if err != nil {
		q.log.Error("research run failed", "run_id", run.ID, "error", err)
		if markErr := q.repo.markFailed(run.ID); markErr != nil {
			q.log.Error("mark run failed", "run_id", run.ID, "error", markErr)
		}
	}
}
func (q *runQueue) stop(ctx context.Context) {
	q.mu.Lock()
	q.stopped = true
	q.cancel()
	q.mu.Unlock()
	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	for {
		select {
		case run := <-q.jobs:
			_ = q.repo.markFailed(run.ID)
		case <-time.After(1 * time.Millisecond):
			return
		}
	}
}
