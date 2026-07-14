DELETE FROM idempotency_keys;
DELETE FROM sync_candidates;
DELETE FROM user_media_tags;

DROP TABLE episode_progress;
DROP TABLE watch_event_participants;
DROP TABLE watch_events;
DROP TABLE user_media_states;

CREATE TABLE user_media_profiles (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('none', 'wishlist', 'watching', 'completed', 'dropped')),
    version INTEGER NOT NULL CHECK (version > 0),
    share_rating INTEGER NOT NULL DEFAULT 0 CHECK (share_rating IN (0, 1)),
    share_review INTEGER NOT NULL DEFAULT 0 CHECK (share_review IN (0, 1)),
    shared_review TEXT CHECK (shared_review IS NULL OR length(shared_review) <= 500),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, media_id)
) STRICT;

CREATE INDEX user_media_profiles_status
ON user_media_profiles (user_id, status, updated_at DESC, media_id);

CREATE TABLE watch_rounds (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    season_number INTEGER CHECK (season_number IS NULL OR season_number > 0),
    round_number INTEGER NOT NULL CHECK (round_number > 0),
    status TEXT NOT NULL CHECK (status IN ('none', 'wishlist', 'watching', 'completed', 'dropped')),
    rating INTEGER CHECK (rating BETWEEN 0 AND 100),
    note TEXT,
    viewing_method TEXT CHECK (viewing_method IS NULL OR length(viewing_method) <= 80),
    started_at TEXT,
    completed_at TEXT,
    archived_at TEXT,
    version INTEGER NOT NULL CHECK (version > 0),
    status_source TEXT NOT NULL CHECK (status_source IN ('external_default', 'confirmed_sync', 'confirmed_import', 'manual')),
    rating_source TEXT NOT NULL CHECK (rating_source IN ('external_default', 'confirmed_sync', 'confirmed_import', 'manual')),
    note_source TEXT NOT NULL CHECK (note_source IN ('external_default', 'confirmed_sync', 'confirmed_import', 'manual')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE UNIQUE INDEX watch_rounds_scope_number
ON watch_rounds (user_id, media_id, COALESCE(season_number, 0), round_number);

CREATE UNIQUE INDEX watch_rounds_current_scope
ON watch_rounds (user_id, media_id, COALESCE(season_number, 0))
WHERE archived_at IS NULL;

CREATE INDEX watch_rounds_history
ON watch_rounds (user_id, media_id, season_number, archived_at DESC, round_number DESC);

CREATE TABLE watch_events (
    id TEXT PRIMARY KEY,
    round_id TEXT NOT NULL REFERENCES watch_rounds(id) ON DELETE CASCADE,
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

CREATE INDEX watch_events_round_time
ON watch_events (round_id, watched_at, id);

CREATE TABLE watch_event_participants (
    event_id TEXT NOT NULL REFERENCES watch_events(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (event_id, user_id)
) STRICT;

CREATE INDEX watch_event_participants_user
ON watch_event_participants (user_id, event_id);

CREATE TABLE round_episode_progress (
    round_id TEXT NOT NULL REFERENCES watch_rounds(id) ON DELETE CASCADE,
    episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    watched_at TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('external_default', 'confirmed_sync', 'confirmed_import', 'manual')),
    watch_event_id TEXT NOT NULL REFERENCES watch_events(id) ON DELETE CASCADE,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (round_id, episode_id),
    UNIQUE (round_id, watch_event_id)
) STRICT;

CREATE INDEX round_episode_progress_time
ON round_episode_progress (round_id, watched_at, episode_id);
