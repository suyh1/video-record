CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member')),
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    csrf_token_hash BLOB NOT NULL CHECK (length(csrf_token_hash) = 32),
    expires_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    revoked_at INTEGER
) STRICT;

CREATE INDEX sessions_user_active_idx ON sessions(user_id, revoked_at, expires_at);

CREATE TABLE login_attempts (
    bucket_key TEXT PRIMARY KEY,
    failures INTEGER NOT NULL,
    window_started INTEGER NOT NULL,
    blocked_until INTEGER NOT NULL
) STRICT;
