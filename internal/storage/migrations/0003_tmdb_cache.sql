CREATE TABLE tmdb_cache (
    cache_key TEXT PRIMARY KEY CHECK (length(cache_key) = 64),
    response_json BLOB NOT NULL,
    expires_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX tmdb_cache_expiry_idx ON tmdb_cache(expires_at);
