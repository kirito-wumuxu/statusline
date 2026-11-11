-- 1. 节点表 (nodes)
CREATE TABLE IF NOT EXISTS nodes (
    node_id         TEXT PRIMARY KEY,
    hostname        TEXT,
    os_type         TEXT NOT NULL,
    os_arch         TEXT,
    ip_address      TEXT,
    client_version  TEXT,
    first_seen_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_seen_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen_at DESC);

-- 2. 会话维度表 (sessions)
CREATE TABLE IF NOT EXISTS sessions (
    session_id              TEXT PRIMARY KEY,
    node_id                 TEXT NOT NULL REFERENCES nodes(node_id),
    product_type            TEXT NOT NULL,
    conversation_id         TEXT,
    session_name            TEXT,
    account_email           TEXT,
    plan_tier               TEXT,
    tool_version            TEXT,
    cwd                     TEXT NOT NULL,
    project_dir             TEXT,
    transcript_path         TEXT,
    git_repo_host           TEXT,
    git_repo_owner          TEXT,
    git_repo_name           TEXT,
    git_branch              TEXT,
    git_worktree            TEXT,
    latest_model_id         TEXT,
    total_input_tokens      INTEGER DEFAULT 0,
    total_output_tokens     INTEGER DEFAULT 0,
    context_used_pct        REAL DEFAULT 0.0,
    total_cost_usd          REAL DEFAULT 0.0,
    total_duration_ms       INTEGER DEFAULT 0,
    total_lines_added       INTEGER DEFAULT 0,
    total_lines_removed     INTEGER DEFAULT 0,
    started_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_active_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_node_active ON sessions(node_id, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_product ON sessions(product_type, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_email ON sessions(account_email) WHERE account_email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(git_repo_owner, git_repo_name) WHERE git_repo_name IS NOT NULL;

-- 3. 对话状态上报明细表 (telemetry_events)
CREATE TABLE IF NOT EXISTS telemetry_events (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id                     TEXT NOT NULL,
    session_id                  TEXT NOT NULL,
    prompt_id                   TEXT,
    product_type                TEXT NOT NULL,
    reported_at                 TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    model_id                    TEXT NOT NULL,
    model_display_name          TEXT,
    model_effort                TEXT,
    agent_state                 TEXT,
    context_window_size         INTEGER,
    total_input_tokens          INTEGER,
    total_output_tokens         INTEGER,
    current_input_tokens        INTEGER,
    current_output_tokens       INTEGER,
    cache_creation_tokens       INTEGER DEFAULT 0,
    cache_read_tokens           INTEGER DEFAULT 0,
    used_percentage             REAL,
    remaining_percentage        REAL,
    exceeds_200k_tokens         INTEGER DEFAULT 0,
    total_cost_usd              REAL,
    total_duration_ms           INTEGER,
    total_api_duration_ms       INTEGER,
    cache_hit_ratio             REAL,
    quota_snapshot              TEXT,
    prompt_cache_detail         TEXT,
    raw_payload                 TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_session_time ON telemetry_events(session_id, reported_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_node_time ON telemetry_events(node_id, reported_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_reported_at ON telemetry_events(reported_at DESC);

-- 4. 账号配额实时状态表 (account_quotas)
CREATE TABLE IF NOT EXISTS account_quotas (
    account_email       TEXT NOT NULL,
    quota_key           TEXT NOT NULL,
    remaining_fraction  REAL NOT NULL,
    reset_time          TEXT NOT NULL,
    reset_in_seconds    INTEGER NOT NULL,
    last_node_id        TEXT,
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (account_email, quota_key)
);
