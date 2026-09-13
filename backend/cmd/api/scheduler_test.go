package main

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSchedulerDueEnabledAssignmentEntersExistingQueue(t *testing.T) {
	db := schedulerTestDatabase(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	release := make(chan struct{})
	queue := newWorkflowQueue(t, db, fakeResearcher{block: release}, fakeWriter{})
	queue.enableScheduling()
	queue.now = func() time.Time { return now }
	s := testScheduler(queue, db, now)
	setSchedule(t, db, "premier_league", now.Add(-time.Second))

	s.runOnce(now)
	waitForActiveRun(t, db, "premier_league")
	close(release)
}

func TestSchedulerSkipsDisabledAndAlreadyActiveAssignments(t *testing.T) {
	db := schedulerTestDatabase(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	release := make(chan struct{})
	queue := newWorkflowQueue(t, db, fakeResearcher{block: release}, fakeWriter{})
	queue.enableScheduling()
	queue.now = func() time.Time { return now }
	s := testScheduler(queue, db, now)

	if _, err := db.Exec(`UPDATE agents SET enabled=0 WHERE id='bundesliga'`); err != nil {
		t.Fatal(err)
	}
	setSchedule(t, db, "bundesliga", now.Add(-time.Second))
	s.runOnce(now)
	assertRunCount(t, db, "bundesliga", 0)

	if _, err := queue.enqueue("premier_league"); err != nil {
		t.Fatal(err)
	}
	waitForActiveRun(t, db, "premier_league")
	setSchedule(t, db, "premier_league", now.Add(-time.Second))
	s.runOnce(now)
	assertRunCount(t, db, "premier_league", 1)
	close(release)
}

func TestSchedulerFailureSchedulesOneIntervalAhead(t *testing.T) {
	db := schedulerTestDatabase(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`UPDATE agents SET research_interval_seconds=60 WHERE id='premier_league'`); err != nil {
		t.Fatal(err)
	}
	queue := newWorkflowQueue(t, db, fakeResearcher{err: context.DeadlineExceeded}, fakeWriter{})
	queue.enableScheduling()
	queue.now = func() time.Time { return now }
	s := testScheduler(queue, db, now)
	setSchedule(t, db, "premier_league", now.Add(-time.Second))

	s.runOnce(now)
	var runID string
	if err := db.QueryRow(`SELECT id FROM runs WHERE agent_id='premier_league'`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	waitForRun(t, db, runID, "failed")
	if got := scheduleTime(t, db, "premier_league"); !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("next attempt = %s, want %s", got, now.Add(time.Minute))
	}
	s.runOnce(now.Add(time.Second))
	assertRunCount(t, db, "premier_league", 1)
}

func TestScheduleStateSurvivesRestartAndFirstRunIsOneIntervalAhead(t *testing.T) {
	db := schedulerTestDatabase(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`UPDATE agents SET enabled=0 WHERE id!='premier_league'; UPDATE agents SET research_interval_seconds=90 WHERE id='premier_league'`); err != nil {
		t.Fatal(err)
	}
	repo := repository{db: db}
	if err := repo.ensureSchedules(now); err != nil {
		t.Fatal(err)
	}
	first := scheduleTime(t, db, "premier_league")
	if !first.Equal(now.Add(90 * time.Second)) {
		t.Fatalf("first schedule = %s, want %s", first, now.Add(90*time.Second))
	}
	if err := repo.ensureSchedules(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if afterRestart := scheduleTime(t, db, "premier_league"); !afterRestart.Equal(first) {
		t.Fatalf("restart changed persisted schedule: got %s, want %s", afterRestart, first)
	}
}

func TestStoppedSchedulerDoesNotEnqueueNewRuns(t *testing.T) {
	db := schedulerTestDatabase(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	queue := newWorkflowQueue(t, db, fakeResearcher{}, fakeWriter{})
	ctx, cancel := context.WithCancel(context.Background())
	s := &scheduler{repo: repository{db: db}, queue: queue, log: testLogger(), now: func() time.Time { return now }, ctx: ctx, cancel: cancel}
	s.stop()
	setSchedule(t, db, "premier_league", now.Add(-time.Second))
	s.runOnce(now)
	assertRunCount(t, db, "premier_league", 0)
}

func schedulerTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := testDatabase(t)
	if _, err := db.Exec(`UPDATE agents SET enabled=0 WHERE id!='premier_league'`); err != nil {
		t.Fatal(err)
	}
	return db
}

func testScheduler(queue *runQueue, db *sql.DB, now time.Time) *scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &scheduler{repo: repository{db: db}, queue: queue, log: testLogger(), now: func() time.Time { return now }, ctx: ctx, cancel: cancel}
}

func setSchedule(t *testing.T, db *sql.DB, agentID string, next time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO agent_schedules (agent_id,next_run_at,updated_at) VALUES (?,?,?) ON CONFLICT(agent_id) DO UPDATE SET next_run_at=excluded.next_run_at, updated_at=excluded.updated_at`, agentID, next.Format(time.RFC3339Nano), next.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func scheduleTime(t *testing.T, db *sql.DB, agentID string) time.Time {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT next_run_at FROM agent_schedules WHERE agent_id=?`, agentID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func assertRunCount(t *testing.T, db *sql.DB, agentID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runs WHERE agent_id=?`, agentID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("run count for %s = %d, want %d", agentID, got, want)
	}
}

func waitForActiveRun(t *testing.T, db *sql.DB, agentID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var active int
		if err := db.QueryRow(`SELECT COUNT(*) FROM runs WHERE agent_id=? AND status IN ('queued','running')`, agentID).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for active run")
}
