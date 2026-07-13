ALTER TABLE user_media_states
ADD COLUMN share_rating INTEGER NOT NULL DEFAULT 0 CHECK (share_rating IN (0, 1));

ALTER TABLE user_media_states
ADD COLUMN share_review INTEGER NOT NULL DEFAULT 0 CHECK (share_review IN (0, 1));

ALTER TABLE user_media_states
ADD COLUMN shared_review TEXT CHECK (shared_review IS NULL OR length(shared_review) <= 500);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    metadata_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE INDEX audit_events_created ON audit_events (created_at, id);
