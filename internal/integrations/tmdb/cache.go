package tmdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"video-record/internal/storage"
)

type Cache struct {
	db  *storage.DB
	now func() time.Time
}

func NewCache(db *storage.DB, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{db: db, now: now}
}

func (cache *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var response []byte
	var expiresAt int64
	err := cache.db.Reader().QueryRowContext(ctx, `
		SELECT response_json, expires_at FROM tmdb_cache WHERE cache_key = ?
	`, key).Scan(&response, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !time.UnixMilli(expiresAt).After(cache.now()) {
		if _, err := cache.db.Writer().ExecContext(ctx, "DELETE FROM tmdb_cache WHERE cache_key = ?", key); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	return response, true, nil
}

func (cache *Cache) Put(ctx context.Context, key string, response []byte, ttl time.Duration) error {
	now := cache.now().UTC()
	_, err := cache.db.Writer().ExecContext(ctx, `
		INSERT INTO tmdb_cache (cache_key, response_json, expires_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			response_json = excluded.response_json,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at
	`, key, response, now.Add(ttl).UnixMilli(), now.UnixMilli())
	return err
}

func (cache *Cache) Delete(ctx context.Context, key string) error {
	_, err := cache.db.Writer().ExecContext(ctx, "DELETE FROM tmdb_cache WHERE cache_key = ?", key)
	return err
}
