-- Agen8 MVP baseline schema.
-- This is the fresh-database schema, not an upgrade chain.

--------------------------------------------------------------------------------
-- space_runtimes
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS space_runtimes (
    space_id TEXT PRIMARY KEY,
    title TEXT,
    current_goal TEXT,
    space_runtime_json TEXT NOT NULL,
    manifest_json TEXT DEFAULT '',
    created_at TEXT,
    updated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_space_runtimes_updated_at ON space_runtimes(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_space_runtimes_created_at ON space_runtimes(created_at DESC);

--------------------------------------------------------------------------------
-- runs
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    goal TEXT NOT NULL,
    run_json TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    created_at TEXT,
    updated_at TEXT,
    parent_run_id TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_runs_space_id ON runs(space_id);
CREATE INDEX IF NOT EXISTS idx_runs_parent_run_id ON runs(parent_run_id);

--------------------------------------------------------------------------------
-- run_snapshots
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS run_snapshots (
    snapshot_id  TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL,
    space_id      TEXT NOT NULL,
    version      INTEGER NOT NULL,
    fingerprint  TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL DEFAULT '',
    runtime_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    UNIQUE(run_id, version)
);

CREATE INDEX IF NOT EXISTS idx_run_snapshots_run_id ON run_snapshots(run_id);
CREATE INDEX IF NOT EXISTS idx_run_snapshots_space_id ON run_snapshots(space_id);

--------------------------------------------------------------------------------
-- events
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS events (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    ts TEXT NOT NULL,
    type TEXT NOT NULL,
    message TEXT NOT NULL,
    data_json TEXT,
    event_json TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    category TEXT NOT NULL DEFAULT 'system',
    origin TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_events_run_seq ON events(run_id, seq);
CREATE INDEX IF NOT EXISTS idx_events_run_type ON events(run_id, type);
CREATE INDEX IF NOT EXISTS idx_events_type_ts ON events(type, ts);
CREATE INDEX IF NOT EXISTS idx_events_severity_seq ON events(severity, seq);
CREATE INDEX IF NOT EXISTS idx_events_category_seq ON events(category, seq);

--------------------------------------------------------------------------------
-- history
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS history (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL,
    space_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    ts TEXT NOT NULL,
    origin TEXT NOT NULL,
    kind TEXT NOT NULL,
    message TEXT NOT NULL,
    model TEXT,
    data_json TEXT,
    line_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_history_space_seq ON history(space_id, seq);

--------------------------------------------------------------------------------
-- constructor_state / constructor_manifest
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS constructor_state (
    run_id TEXT PRIMARY KEY,
    updated_at TEXT,
    state_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS constructor_manifest (
    run_id TEXT PRIMARY KEY,
    updated_at TEXT,
    manifest_json TEXT NOT NULL
);

--------------------------------------------------------------------------------
-- agent space entries
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_space_entries (
    entry_id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    turn_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    tool_call_id TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    surface TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT,
    live INTEGER NOT NULL DEFAULT 0,
    data_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_agent_space_entries_run_created ON agent_space_entries(run_id, created_at, entry_id);
CREATE INDEX IF NOT EXISTS idx_agent_space_entries_run_turn ON agent_space_entries(run_id, turn_id);
CREATE INDEX IF NOT EXISTS idx_agent_space_entries_tool_call ON agent_space_entries(tool_call_id);

--------------------------------------------------------------------------------
-- message conversation activities
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS message_conversation_activities (
    activity_id  TEXT PRIMARY KEY,
    channel_id   TEXT NOT NULL,
    space_id     TEXT NOT NULL,
    member_id    TEXT NOT NULL,
    session_id   TEXT NOT NULL,
    turn_id      TEXT NOT NULL,
    tool_call_id TEXT NOT NULL,
    sequence     INTEGER NOT NULL DEFAULT 0,
    kind         TEXT NOT NULL,
    title        TEXT NOT NULL,
    status       TEXT NOT NULL,
    text         TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    completed_at TEXT,
    data_json    TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_message_conversation_activities_channel_created
    ON message_conversation_activities(channel_id, sequence, created_at, activity_id);
CREATE INDEX IF NOT EXISTS idx_message_conversation_activities_tool_call
    ON message_conversation_activities(session_id, turn_id, tool_call_id);

--------------------------------------------------------------------------------
-- run_conversations
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS run_conversations (
    run_id TEXT PRIMARY KEY,
    messages_json TEXT NOT NULL
);

--------------------------------------------------------------------------------
-- project_spaces
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS project_spaces (
    user_id TEXT NOT NULL DEFAULT 'local',
    project_root TEXT NOT NULL,
    project_id TEXT,
    space_id TEXT NOT NULL,
    coordinator_run_id TEXT,
    status TEXT NOT NULL,
    desired_enabled INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE(user_id, project_root, space_id)
);

CREATE INDEX IF NOT EXISTS idx_project_spaces_root_status ON project_spaces(project_root, status);
CREATE INDEX IF NOT EXISTS idx_project_spaces_root_updated ON project_spaces(project_root, updated_at DESC);

--------------------------------------------------------------------------------
-- project_registry
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS project_registry (
    user_id TEXT NOT NULL DEFAULT 'local',
    project_root TEXT NOT NULL,
    project_id TEXT,
    manifest_path TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    name TEXT,
    goal TEXT,
    yaml_migrated INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_root)
);

CREATE INDEX IF NOT EXISTS idx_project_registry_enabled_updated ON project_registry(enabled, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_registry_user_enabled_updated ON project_registry(user_id, enabled, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_registry_project_id ON project_registry(project_id);

--------------------------------------------------------------------------------
-- spaces
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS spaces (
    space_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT 'local',
    project_id TEXT DEFAULT '',
    plan_mode TEXT DEFAULT '',
    status TEXT NOT NULL,
    title TEXT DEFAULT '',
    space_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_spaces_user_id ON spaces(user_id);
CREATE INDEX IF NOT EXISTS idx_spaces_project_updated ON spaces(project_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id TEXT PRIMARY KEY,
    profile_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

--------------------------------------------------------------------------------
-- channels
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS channels (
    channel_id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL,
    project_id TEXT DEFAULT '',
    run_id TEXT DEFAULT '',
    member_id TEXT DEFAULT '',
    member_label TEXT DEFAULT '',
    title TEXT DEFAULT '',
    status TEXT NOT NULL,
    channel_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    -- last_message_at: timestamp of the most recent message published
    -- into this member address. Bumped by the message publish hook;
    -- used (with channel_reads.last_seen_at) to compute per-user
    -- unread state on channel.list. Nullable for member addresses
    -- that have never received a message.
    last_message_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_space_member_run
    ON channels(space_id, member_label, run_id, member_id);
CREATE INDEX IF NOT EXISTS idx_channels_space_id ON channels(space_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_channels_member_id ON channels(member_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_channels_run_id ON channels(run_id) WHERE run_id != '';

-- Per-user "I have read this channel up to here" markers. Replaces the
-- prior client-side localStorage approach that lost state on refresh
-- and over-fired due to noisy ch.updated_at touches.
--
-- One row per (user, channel). Missing row = user has never read the
-- channel; combined with channels.last_message_at this gives the
-- canonical "is this channel unread for me right now" answer.
CREATE TABLE IF NOT EXISTS channel_reads (
    user_id      TEXT NOT NULL,
    channel_id   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    PRIMARY KEY (user_id, channel_id)
);
CREATE INDEX IF NOT EXISTS idx_channel_reads_user ON channel_reads(user_id);

--------------------------------------------------------------------------------
-- memories / memory_instructions
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL DEFAULT '',
    date TEXT NOT NULL,
    clock TEXT NOT NULL,
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memories_project_date ON memories(project_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_memories_project_agent_date ON memories(project_id, agent_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);

CREATE TABLE IF NOT EXISTS memory_instructions (
    project_id TEXT PRIMARY KEY DEFAULT '',
    content TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

--------------------------------------------------------------------------------
-- plans v2
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS plans (
    id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL DEFAULT '',
    mission_id TEXT NOT NULL,
    kr_refs_json TEXT NOT NULL DEFAULT '[]',
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    abandoned_reason TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_plans_space_status_updated ON plans(space_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_plans_mission ON plans(mission_id, updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_plans_space_nonterminal
    ON plans(space_id)
    WHERE status NOT IN ('completed', 'abandoned', 'deleted');

CREATE TABLE IF NOT EXISTS plan_phases (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    title TEXT NOT NULL,
    phase_order INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(plan_id) REFERENCES plans(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_phases_plan_order ON plan_phases(plan_id, phase_order);
CREATE INDEX IF NOT EXISTS idx_plan_phases_plan_status_order ON plan_phases(plan_id, status, phase_order);

CREATE TABLE IF NOT EXISTS plan_todos (
    id TEXT PRIMARY KEY,
    phase_id TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    text TEXT NOT NULL,
    done INTEGER NOT NULL DEFAULT 0,
    todo_order INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(plan_id) REFERENCES plans(id) ON DELETE CASCADE,
    FOREIGN KEY(phase_id) REFERENCES plan_phases(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_todos_phase_order ON plan_todos(phase_id, todo_order);
CREATE INDEX IF NOT EXISTS idx_plan_todos_plan_phase ON plan_todos(plan_id, phase_id);

CREATE TABLE IF NOT EXISTS plan_comments (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    phase_id TEXT,
    todo_id TEXT,
    author_type TEXT NOT NULL,
    author_id TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(plan_id) REFERENCES plans(id) ON DELETE CASCADE,
    FOREIGN KEY(phase_id) REFERENCES plan_phases(id) ON DELETE SET NULL,
    FOREIGN KEY(todo_id) REFERENCES plan_todos(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_plan_comments_plan_created ON plan_comments(plan_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_plan_comments_plan_phase ON plan_comments(plan_id, phase_id, created_at);

CREATE TABLE IF NOT EXISTS plan_comment_reads (
    comment_id TEXT NOT NULL,
    reader_id TEXT NOT NULL,
    read_at TEXT NOT NULL,
    PRIMARY KEY(comment_id, reader_id),
    FOREIGN KEY(comment_id) REFERENCES plan_comments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_plan_comment_reads_reader ON plan_comment_reads(reader_id, read_at DESC);

CREATE TABLE IF NOT EXISTS plan_amendments (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    proposed_by TEXT NOT NULL,
    rationale TEXT NOT NULL,
    diff_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL,
    veto_deadline TEXT NOT NULL,
    vetoed_by TEXT NOT NULL DEFAULT '',
    veto_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    resolved_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(plan_id) REFERENCES plans(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_plan_amendments_plan_status_deadline ON plan_amendments(plan_id, status, veto_deadline);
CREATE INDEX IF NOT EXISTS idx_plan_amendments_pending_deadline ON plan_amendments(status, veto_deadline);

CREATE TABLE IF NOT EXISTS plan_reviews (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL,
    decision TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    FOREIGN KEY(plan_id) REFERENCES plans(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_plan_reviews_plan_created
    ON plan_reviews(plan_id, created_at DESC);

-- notifications / notification_rules
--------------------------------------------------------------------------------
-- notifications use a typed identity model:
--   user_id     -- routing key (who sees the notification)
--   project_id  -- workspace scope (filter / list)
--   subject_*   -- typed reference to the underlying domain entity, used
--                 for auto-dismiss on resolve (DismissBySubject) and for
--                 deep-linking. Subject is optional — heartbeat summaries
--                 don't always have a single underlying entity.
CREATE TABLE IF NOT EXISTS notifications (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    project_id   TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL,
    trigger_name TEXT NOT NULL,
    severity     TEXT NOT NULL,
    subject_kind TEXT NOT NULL DEFAULT '',
    subject_id   TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL,
    body         TEXT,
    link_surface TEXT,
    link_url     TEXT,
    metadata     TEXT,
    throttle_key TEXT,
    created_at   DATETIME NOT NULL,
    read_at      DATETIME,
    dismissed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications(user_id, read_at) WHERE dismissed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_throttle
    ON notifications(user_id, source, trigger_name, throttle_key, created_at DESC);
-- Subject lookups feed DismissBySubject — narrow to active rows since
-- dismissed ones aren't candidates for further dismissal.
CREATE INDEX IF NOT EXISTS idx_notifications_subject
    ON notifications(user_id, source, subject_kind, subject_id) WHERE dismissed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_created
    ON notifications(created_at);

CREATE TABLE IF NOT EXISTS notification_rules (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL,
    source           TEXT NOT NULL,
    trigger_name     TEXT NOT NULL,
    min_severity     TEXT NOT NULL DEFAULT 'info',
    channels         TEXT NOT NULL,
    cooldown_minutes INTEGER NOT NULL DEFAULT 30,
    enabled          BOOLEAN NOT NULL DEFAULT 1,
    webhook_url      TEXT
);

CREATE INDEX IF NOT EXISTS idx_notification_rules_user
    ON notification_rules(user_id, enabled);

--------------------------------------------------------------------------------
-- schedule_entries
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS schedule_entries (
    entry_id          TEXT PRIMARY KEY,
    space_id          TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT '',
    target_kind       TEXT NOT NULL DEFAULT '',
    next_run_at       TEXT,
    expires_at        TEXT,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    dedupe_key        TEXT NOT NULL DEFAULT '',
    entry_json        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_schedule_entries_space_status
    ON schedule_entries(space_id, status);
CREATE INDEX IF NOT EXISTS idx_schedule_entries_next_run
    ON schedule_entries(status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_schedule_entries_dedupe
    ON schedule_entries(space_id, dedupe_key, status);

CREATE TABLE IF NOT EXISTS schedule_runs (
    run_id           TEXT PRIMARY KEY,
    entry_id         TEXT NOT NULL,
    space_id         TEXT NOT NULL DEFAULT '',
    due_at           TEXT NOT NULL,
    started_at       TEXT NOT NULL,
    finished_at      TEXT,
    status           TEXT NOT NULL DEFAULT '',
    target_kind      TEXT NOT NULL DEFAULT '',
    target_object_id TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    run_json         TEXT NOT NULL,
    UNIQUE(entry_id, due_at)
);

CREATE INDEX IF NOT EXISTS idx_schedule_runs_entry_due
    ON schedule_runs(entry_id, due_at DESC);
CREATE INDEX IF NOT EXISTS idx_schedule_runs_space_started
    ON schedule_runs(space_id, started_at DESC);

--------------------------------------------------------------------------------
-- tool_sources / project_tool_source_attachments
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tool_sources (
    source_id         TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL,
    owner_kind        TEXT NOT NULL DEFAULT 'project',
    type              TEXT NOT NULL,
    config_json       TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'disconnected',
    tool_count        INTEGER NOT NULL DEFAULT 0,
    last_connected_at DATETIME,
    last_error        TEXT,
    fingerprint       TEXT,
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_sources_project ON tool_sources(project_id);

CREATE TABLE IF NOT EXISTS project_tool_source_attachments (
    project_id        TEXT NOT NULL,
    source_id         TEXT NOT NULL,
    alias             TEXT NOT NULL DEFAULT '',
    enabled           INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (project_id, source_id)
);

--------------------------------------------------------------------------------
-- mcp_login_sessions
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mcp_login_sessions (
    state         TEXT PRIMARY KEY,
    source_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL DEFAULT '',
    redirect_url  TEXT NOT NULL,
    code_verifier TEXT NOT NULL,
    authorize_url TEXT NOT NULL,
    token_url     TEXT NOT NULL,
    client_id     TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_mcp_login_sessions_source_id
    ON mcp_login_sessions(source_id);

--------------------------------------------------------------------------------
-- tool_audit_log
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tool_audit_log (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    space_id        TEXT NOT NULL,
    member_label   TEXT NOT NULL,
    tool_name      TEXT NOT NULL,
    source_type    TEXT NOT NULL,
    source_id      TEXT NOT NULL,
    status         TEXT NOT NULL,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    request_json   TEXT,
    response_json  TEXT,
    approval_id    TEXT,
    error_code     TEXT,
    created_at     DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_audit_log_project ON tool_audit_log(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_audit_log_tool ON tool_audit_log(tool_name, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_audit_log_member ON tool_audit_log(member_label, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_audit_log_status ON tool_audit_log(status, created_at);

--------------------------------------------------------------------------------
-- models
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS models (
    model_id           TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    provider           TEXT NOT NULL,
    source             TEXT NOT NULL DEFAULT 'openrouter',
    input_price_per_m  REAL NOT NULL DEFAULT 0,
    input_cache_read_price_per_m REAL NOT NULL DEFAULT 0,
    output_price_per_m REAL NOT NULL DEFAULT 0,
    context_length     INTEGER NOT NULL DEFAULT 0,
    is_reasoning       INTEGER NOT NULL DEFAULT 0,
    description        TEXT,
    modality_json      TEXT,
    top_provider       TEXT,
    max_completion_tokens INTEGER,
    tokenizer          TEXT,
    instruct_type      TEXT,
    fetched_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider);
CREATE INDEX IF NOT EXISTS idx_models_source ON models(source);

--------------------------------------------------------------------------------
-- pending_human_inputs
--
-- Identity (space_id, member_id) is the asker; channel_id is the panel
-- delivery target. The two are decoupled so the same member can own
-- multiple in-flight questions across different channels, and so a
-- channel can render a panel for any member that asks within it.
--
-- Liveness: daemon_instance_id is set by the awaiter goroutine to the
-- daemon process's instance id. awaiter_heartbeat is refreshed every
-- ~30s while the awaiter is alive. On startup the daemon sweeps any row
-- whose daemon_instance_id != current or whose heartbeat is stale —
-- those are orphans from a previous process or a crashed awaiter, and
-- the panel they correspond to has no consumer.
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS pending_human_inputs (
    id                  TEXT PRIMARY KEY,
    space_id            TEXT NOT NULL,
    member_id           TEXT NOT NULL,
    channel_id          TEXT NOT NULL,
    tool_call_id        TEXT NOT NULL,
    tool_name           TEXT NOT NULL,
    primitive           TEXT NOT NULL,
    payload_json        TEXT NOT NULL,
    project_id          TEXT NOT NULL,
    daemon_instance_id  TEXT NOT NULL,
    awaiter_heartbeat   TEXT NOT NULL,
    expires_at          TEXT,
    created_at          TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pending_human_inputs_member_call
    ON pending_human_inputs(space_id, member_id, tool_call_id);
CREATE INDEX IF NOT EXISTS idx_pending_human_inputs_channel
    ON pending_human_inputs(channel_id);
CREATE INDEX IF NOT EXISTS idx_pending_human_inputs_instance
    ON pending_human_inputs(daemon_instance_id);

CREATE TABLE IF NOT EXISTS pending_human_input_submissions (
    space_id     TEXT NOT NULL,
    member_id    TEXT NOT NULL,
    tool_call_id TEXT NOT NULL,
    result_json  TEXT NOT NULL,
    submitted_at TEXT NOT NULL,
    PRIMARY KEY (space_id, member_id, tool_call_id)
);

--------------------------------------------------------------------------------
-- tool_call_results
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tool_call_results (
    idempotency_key TEXT PRIMARY KEY,
    tool_name       TEXT NOT NULL,
    result_json     TEXT NOT NULL,
    created_at      DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_call_results_created ON tool_call_results(created_at);

--------------------------------------------------------------------------------
-- integration_credentials
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS integration_credentials (
    user_id       TEXT NOT NULL DEFAULT 'local',
    project_id    TEXT NOT NULL,
    owner_type    TEXT NOT NULL,
    owner_id      TEXT NOT NULL,
    auth_type     TEXT NOT NULL,
    credentials   BLOB NOT NULL,
    scopes        TEXT,
    expires_at    TEXT,
    refresh_token BLOB,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (user_id, project_id, owner_type, owner_id)
);

CREATE INDEX IF NOT EXISTS idx_integration_credentials_project
    ON integration_credentials(project_id);
CREATE INDEX IF NOT EXISTS idx_integration_credentials_user_project
    ON integration_credentials(user_id, project_id);

--------------------------------------------------------------------------------
-- credentials
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS credentials (
    credential_id TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    label         TEXT NOT NULL,
    status        TEXT NOT NULL,
    fields_json   TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_credentials_kind
    ON credentials(kind);
CREATE INDEX IF NOT EXISTS idx_credentials_status
    ON credentials(status);

CREATE TABLE IF NOT EXISTS credential_material (
    credential_id TEXT PRIMARY KEY,
    storage_kind  TEXT NOT NULL,
    payload       BLOB,
    updated_at    TEXT NOT NULL,
    FOREIGN KEY (credential_id) REFERENCES credentials(credential_id) ON DELETE CASCADE
);

--------------------------------------------------------------------------------
-- integration_oauth_states
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS integration_oauth_states (
    state TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    project_root TEXT,
    integration_json TEXT NOT NULL,
    redirect_url TEXT NOT NULL,
    code_verifier TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

--------------------------------------------------------------------------------
-- missions / strategy map
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS missions (
    mission_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT,
    mission_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_missions_project ON missions(project_id);
CREATE INDEX IF NOT EXISTS idx_missions_status ON missions(status);
CREATE INDEX IF NOT EXISTS idx_missions_created_at ON missions(created_at);

CREATE TABLE IF NOT EXISTS key_results (
    key_result_id TEXT PRIMARY KEY,
    mission_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open',
    space_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT,
    key_result_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_key_results_mission ON key_results(mission_id);
CREATE INDEX IF NOT EXISTS idx_key_results_status ON key_results(status);
CREATE INDEX IF NOT EXISTS idx_key_results_space ON key_results(space_id);

CREATE TABLE IF NOT EXISTS key_result_progress_entries (
    progress_entry_id TEXT PRIMARY KEY,
    key_result_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    progress_entry_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_key_result_progress_key_result ON key_result_progress_entries(key_result_id);
CREATE INDEX IF NOT EXISTS idx_key_result_progress_created ON key_result_progress_entries(created_at);

CREATE TABLE IF NOT EXISTS decisions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    space_id TEXT DEFAULT '',
    run_id TEXT DEFAULT '',
    source TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT '',
    source_identity TEXT DEFAULT '',
    title TEXT NOT NULL,
    rationale TEXT NOT NULL,
    context TEXT DEFAULT '',
    questions_json TEXT DEFAULT '[]',
    answers_json TEXT DEFAULT '[]',
    cancelled INTEGER NOT NULL DEFAULT 0,
    alternatives_rejected TEXT DEFAULT '',
    invalidation_conditions_json TEXT DEFAULT '[]',
    outcome TEXT DEFAULT '',
    confidence REAL NOT NULL DEFAULT 1.0,
    mission_ref TEXT DEFAULT '',
    key_result_ref TEXT DEFAULT '',
    task_ref TEXT DEFAULT '',
    plan_ref TEXT DEFAULT '',
    operator_action_ref TEXT DEFAULT '',
    escalation_ref TEXT DEFAULT '',
    correlation_ref TEXT DEFAULT '',
    informed_by_ref TEXT DEFAULT '',
    tags_json TEXT DEFAULT '[]',
    metadata_json TEXT DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_dec_project ON decisions(project_id);
CREATE INDEX IF NOT EXISTS idx_dec_key_result ON decisions(key_result_ref);
CREATE INDEX IF NOT EXISTS idx_dec_operator_action ON decisions(operator_action_ref);
CREATE INDEX IF NOT EXISTS idx_dec_source ON decisions(source);
CREATE INDEX IF NOT EXISTS idx_dec_task ON decisions(task_ref);
CREATE INDEX IF NOT EXISTS idx_dec_mission ON decisions(mission_ref);
CREATE INDEX IF NOT EXISTS idx_dec_escalation ON decisions(escalation_ref);
CREATE INDEX IF NOT EXISTS idx_dec_space ON decisions(space_id);

CREATE TABLE IF NOT EXISTS context_links (
    id TEXT PRIMARY KEY,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    edge_type TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 1.0,
    metadata_json TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    created_by TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_cl_source ON context_links(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_cl_target ON context_links(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_cl_edge_type ON context_links(edge_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_unique_edge ON context_links(source_type, source_id, target_type, target_id, edge_type);

--------------------------------------------------------------------------------
-- operator actions
--------------------------------------------------------------------------------
-- Escalations table (formerly mis-named "operator_actions"; now correctly
-- named since the actual operator-action aggregate lives in op_actions).
-- Drops legacy run_id and delegated_to columns; renames space_id -> space_id.
CREATE TABLE IF NOT EXISTS escalations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    space_id TEXT DEFAULT '',
    task_ref TEXT DEFAULT '',
    key_result_ref TEXT NOT NULL DEFAULT '',
    mission_ref TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    member_id TEXT DEFAULT '',
    category TEXT NOT NULL,
    urgency TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    recommendation TEXT DEFAULT '',
    confidence REAL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    resolution TEXT DEFAULT '',
    resolution_note TEXT DEFAULT '',
    deadline TEXT,
    escalated_at TEXT,
    original_urgency TEXT DEFAULT '',
    metadata_json TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    resolved_at TEXT,
    resolved_by TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_esc_project_status ON escalations(project_id, status);
CREATE INDEX IF NOT EXISTS idx_esc_task_ref ON escalations(task_ref);
CREATE INDEX IF NOT EXISTS idx_esc_status ON escalations(status);
CREATE INDEX IF NOT EXISTS idx_esc_deadline ON escalations(deadline);
CREATE INDEX IF NOT EXISTS idx_esc_space ON escalations(space_id);

CREATE TABLE IF NOT EXISTS op_actions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    space_id TEXT,
    task_ref TEXT,
    run_id TEXT,
    blocking INTEGER NOT NULL DEFAULT 0,
    key_result_ref TEXT NOT NULL DEFAULT '',
    mission_ref TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    member_id TEXT,
    escalation_ref TEXT,
    category TEXT NOT NULL,
    urgency TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    requires_verification INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    outcome_status TEXT,
    outcome_summary TEXT,
    outcome_pairs_json TEXT,
    attachments_json TEXT,
    progress_notes_json TEXT,
    comments_json TEXT,
    deadline TEXT,
    metadata_json TEXT,
    created_at TEXT NOT NULL,
    acknowledged_at TEXT,
    started_at TEXT,
    completed_at TEXT,
    verified_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_op_actions_project_status ON op_actions(project_id, status);
CREATE INDEX IF NOT EXISTS idx_op_actions_project_urgency ON op_actions(project_id, urgency);
CREATE INDEX IF NOT EXISTS idx_op_actions_task_ref ON op_actions(task_ref);
CREATE INDEX IF NOT EXISTS idx_op_actions_space_id ON op_actions(space_id);
CREATE INDEX IF NOT EXISTS idx_op_actions_created_at ON op_actions(created_at DESC);

--------------------------------------------------------------------------------
-- space members
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS members (
    member_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT 'local',
    project_id TEXT DEFAULT '',
    space_id TEXT NOT NULL,
    member_type TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL,
    harness_kind TEXT DEFAULT '',
    member_json TEXT NOT NULL,
    registered_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_seen_at TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_members_space_state
    ON members(space_id, lifecycle_state, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_members_project_space
    ON members(project_id, space_id, updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_members_one_active_coordinator
    ON members(space_id)
    WHERE lifecycle_state = 'active'
      AND member_type = 'coordinator';
