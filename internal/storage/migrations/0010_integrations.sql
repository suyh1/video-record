CREATE TABLE external_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('jellyfin', 'emby', 'plex')),
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
    base_url TEXT NOT NULL CHECK (length(trim(base_url)) BETWEEN 1 AND 2048),
    credential_ciphertext BLOB NOT NULL CHECK (length(credential_ciphertext) > 0),
    credential_nonce BLOB NOT NULL CHECK (length(credential_nonce) > 0),
    credential_version INTEGER NOT NULL CHECK (credential_version > 0),
    credential_fingerprint TEXT NOT NULL CHECK (length(credential_fingerprint) BETWEEN 8 AND 128),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX external_accounts_user_provider ON external_accounts (user_id, provider, id);

CREATE TABLE external_media_mappings (
    account_id TEXT NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    provider_item_id TEXT NOT NULL,
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'episode')),
    media_id TEXT REFERENCES media_items(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES episodes(id) ON DELETE CASCADE,
    confirmed INTEGER NOT NULL CHECK (confirmed IN (0, 1)),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (account_id, provider_item_id, media_type),
    CHECK ((media_type = 'movie' AND media_id IS NOT NULL AND episode_id IS NULL) OR
           (media_type = 'episode' AND episode_id IS NOT NULL))
) STRICT;

CREATE TABLE sync_jobs (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('incremental', 'compensation')),
    next_run_at INTEGER NOT NULL,
    cursor TEXT,
    lease_owner TEXT,
    lease_expires_at INTEGER,
    last_error_code TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (account_id, kind),
    CHECK ((lease_owner IS NULL) = (lease_expires_at IS NULL))
) STRICT;

CREATE INDEX sync_jobs_due ON sync_jobs (next_run_at, lease_expires_at, id);

CREATE TABLE sync_runs (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    job_kind TEXT NOT NULL CHECK (job_kind IN ('incremental', 'compensation')),
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    cursor TEXT,
    summary_json TEXT NOT NULL DEFAULT '{}',
    started_at INTEGER NOT NULL,
    finished_at INTEGER
) STRICT;

CREATE INDEX sync_runs_account_started ON sync_runs (account_id, started_at DESC, id);

CREATE TABLE sync_candidates (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    external_event_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('exact', 'possible', 'unmatched', 'conflict', 'confirmed', 'ignored')),
    payload_json TEXT NOT NULL,
    media_id TEXT REFERENCES media_items(id) ON DELETE SET NULL,
    episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (account_id, external_event_id)
) STRICT;
