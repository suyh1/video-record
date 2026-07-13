CREATE TABLE watch_events (
    id TEXT PRIMARY KEY,
    created_by_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    watched_at TEXT NOT NULL,
    viewing_method TEXT,
    source TEXT NOT NULL CHECK (source IN ('external_default', 'confirmed_sync', 'confirmed_import', 'manual')),
    external_event_id TEXT,
    completion INTEGER NOT NULL CHECK (completion BETWEEN 0 AND 100),
    note TEXT,
    created_at INTEGER NOT NULL
) STRICT;

CREATE UNIQUE INDEX watch_events_external_event
ON watch_events (created_by_user_id, source, external_event_id)
WHERE external_event_id IS NOT NULL;

CREATE INDEX watch_events_media_time
ON watch_events (media_id, watched_at, id);

CREATE TABLE watch_event_participants (
    event_id TEXT NOT NULL REFERENCES watch_events(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (event_id, user_id)
) STRICT;

CREATE INDEX watch_event_participants_user
ON watch_event_participants (user_id, event_id);

CREATE TABLE idempotency_keys (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    etag TEXT NOT NULL,
    response_body BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, key)
) STRICT;

CREATE INDEX idempotency_keys_expiry ON idempotency_keys (expires_at);
