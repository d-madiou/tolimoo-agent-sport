CREATE TABLE IF NOT EXISTS agent_schedules (
    agent_id TEXT PRIMARY KEY REFERENCES agents(id),
    next_run_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_schedules_next_run
ON agent_schedules(next_run_at);
