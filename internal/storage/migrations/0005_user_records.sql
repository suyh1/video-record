CREATE TABLE user_media_states (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('none', 'wishlist', 'watching', 'completed', 'dropped')),
    rating INTEGER CHECK (rating BETWEEN 0 AND 100),
    note TEXT,
    started_at TEXT,
    completed_at TEXT,
    version INTEGER NOT NULL CHECK (version > 0),
    status_source TEXT NOT NULL,
    rating_source TEXT NOT NULL,
    note_source TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, media_id)
) STRICT;

CREATE TABLE tags (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    UNIQUE (user_id, name)
) STRICT;

CREATE TABLE user_media_tags (
    user_id TEXT NOT NULL,
    media_id TEXT NOT NULL,
    tag_id TEXT NOT NULL,
    PRIMARY KEY (user_id, media_id, tag_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media_items(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
) STRICT;

CREATE TABLE collections (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    UNIQUE (user_id, name)
) STRICT;

CREATE TABLE collection_items (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (collection_id, media_id),
    UNIQUE (collection_id, position)
) STRICT;
