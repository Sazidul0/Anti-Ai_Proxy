-- CTF Anti-AI Proxy Gateway
-- Initial Database Schema
-- ========================

BEGIN;

-- ── Users ──
CREATE TABLE IF NOT EXISTS users (
    id              SERIAL PRIMARY KEY,
    username        VARCHAR(255) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    role            VARCHAR(50) NOT NULL DEFAULT 'user',
    suspicion_score INTEGER NOT NULL DEFAULT 0,
    is_suspicious   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Sessions ──
CREATE TABLE IF NOT EXISTS sessions (
    id                 SERIAL PRIMARY KEY,
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token      VARCHAR(512) UNIQUE NOT NULL,
    ip_address         INET NOT NULL,
    device_fingerprint VARCHAR(512),
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    connection_start   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_activity      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    connection_end     TIMESTAMPTZ
);

-- ── Request Logs ──
CREATE TABLE IF NOT EXISTS request_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    session_id   INTEGER REFERENCES sessions(id) ON DELETE SET NULL,
    ip_address   INET NOT NULL,
    domain       VARCHAR(512) NOT NULL,
    url_path     TEXT,
    method       VARCHAR(10),
    status       VARCHAR(20) NOT NULL,   -- 'allowed' | 'blocked'
    block_reason TEXT,
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Flag Submissions ──
CREATE TABLE IF NOT EXISTS flag_submissions (
    id              SERIAL PRIMARY KEY,
    user_id         INTEGER REFERENCES users(id) ON DELETE SET NULL,
    session_id      INTEGER REFERENCES sessions(id) ON DELETE SET NULL,
    challenge_id    INTEGER,
    proxy_connected BOOLEAN NOT NULL,
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Suspicion Events ──
CREATE TABLE IF NOT EXISTS suspicion_events (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type  VARCHAR(100) NOT NULL,
    score_delta INTEGER NOT NULL,
    details     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Indexes ──
CREATE INDEX idx_request_logs_user_id    ON request_logs(user_id);
CREATE INDEX idx_request_logs_timestamp  ON request_logs(timestamp);
CREATE INDEX idx_request_logs_status     ON request_logs(status);
CREATE INDEX idx_request_logs_domain     ON request_logs(domain);

CREATE INDEX idx_sessions_user_id  ON sessions(user_id);
CREATE INDEX idx_sessions_active   ON sessions(is_active);
CREATE INDEX idx_sessions_token    ON sessions(session_token);

CREATE INDEX idx_suspicion_events_user_id ON suspicion_events(user_id);

CREATE INDEX idx_flag_submissions_user_id ON flag_submissions(user_id);

-- ── Default admin user ──
-- password: admin (bcrypt hash)
INSERT INTO users (username, password_hash, role)
VALUES ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'admin')
ON CONFLICT (username) DO NOTHING;

COMMIT;
