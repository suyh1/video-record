package storage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const maxReaderConnections = 4

var ErrNotMigrated = errors.New("database migrations have not been applied")

type DB struct {
	writer *sql.DB
	reader *sql.DB
}

func Open(ctx context.Context, path string) (*DB, error) {
	dataDir := filepath.Dir(path)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}

	writer, err := sql.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	if err := writer.PingContext(ctx); err != nil {
		_ = writer.Close()
		return nil, err
	}

	reader, err := sql.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(maxReaderConnections)
	reader.SetMaxIdleConns(maxReaderConnections)
	if err := reader.PingContext(ctx); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}

	return &DB{writer: writer, reader: reader}, nil
}

func (db *DB) Writer() *sql.DB {
	return db.writer
}

func (db *DB) Reader() *sql.DB {
	return db.reader
}

func (db *DB) Close() error {
	return errors.Join(db.reader.Close(), db.writer.Close())
}

func (db *DB) Ready(ctx context.Context) error {
	var applied int
	if err := db.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		return err
	}
	if applied == 0 {
		return ErrNotMigrated
	}

	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE schema_migrations SET applied_at = applied_at WHERE 0"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Rollback()
}

func sqliteDSN(path string, readOnly bool) string {
	dsn := &url.URL{Scheme: "file", Path: path}
	query := dsn.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	if readOnly {
		query.Add("_pragma", "query_only(1)")
	} else {
		query.Add("_pragma", "journal_mode(WAL)")
	}
	dsn.RawQuery = query.Encode()
	return dsn.String()
}
