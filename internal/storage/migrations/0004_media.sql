CREATE TABLE media_items (
    id TEXT PRIMARY KEY,
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    external_title TEXT NOT NULL,
    original_title TEXT NOT NULL,
    release_date TEXT NOT NULL,
    external_overview TEXT NOT NULL,
    poster_path TEXT NOT NULL,
    backdrop_path TEXT NOT NULL,
    custom_title TEXT,
    custom_overview TEXT,
    custom_year TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE media_external_ids (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    source_id TEXT NOT NULL,
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    PRIMARY KEY (source, source_id, media_type),
    UNIQUE (media_id, source)
) STRICT;

CREATE TABLE seasons (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    source_id TEXT,
    season_number INTEGER NOT NULL,
    name TEXT NOT NULL,
    overview TEXT NOT NULL,
    poster_path TEXT NOT NULL,
    air_date TEXT NOT NULL,
    UNIQUE (media_id, season_number)
) STRICT;

CREATE TABLE episodes (
    id TEXT PRIMARY KEY,
    season_id TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    source_id TEXT,
    episode_number INTEGER NOT NULL,
    name TEXT NOT NULL,
    overview TEXT NOT NULL,
    still_path TEXT NOT NULL,
    air_date TEXT NOT NULL,
    runtime INTEGER,
    UNIQUE (season_id, episode_number)
) STRICT;

CREATE TABLE genres (
    source TEXT NOT NULL,
    source_id TEXT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (source, source_id)
) STRICT;

CREATE TABLE media_genres (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    source_id TEXT NOT NULL,
    PRIMARY KEY (media_id, source, source_id),
    FOREIGN KEY (source, source_id) REFERENCES genres(source, source_id) ON DELETE CASCADE
) STRICT;

CREATE TABLE media_credits (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    person_id TEXT NOT NULL,
    name TEXT NOT NULL,
    character_name TEXT NOT NULL,
    job TEXT NOT NULL,
    department TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY (media_id, source, person_id, job, character_name)
) STRICT;
