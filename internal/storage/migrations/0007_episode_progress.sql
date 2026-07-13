CREATE TABLE episode_progress (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    watched_at TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('external_default', 'confirmed_sync', 'confirmed_import', 'manual')),
    watch_event_id TEXT NOT NULL REFERENCES watch_events(id) ON DELETE CASCADE,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, episode_id),
    UNIQUE (user_id, watch_event_id)
) STRICT;

CREATE INDEX episode_progress_user_media
ON episode_progress (user_id, media_id, watched_at, episode_id);
