CREATE TABLE IF NOT EXISTS agent_runs (
    run_id TEXT PRIMARY KEY,
    bot_name TEXT NOT NULL,
    runtime_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'done')),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    completed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_status_updated_at
ON agent_runs(status, updated_at);

CREATE INDEX IF NOT EXISTS idx_agent_runs_bot_runtime_status
ON agent_runs(bot_name, runtime_type, status);
