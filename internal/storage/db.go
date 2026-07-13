package storage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const maxReaderConnections = 4

var ErrNotMigrated = errors.New("database migrations have not been applied")

type DB struct {
	mu     sync.RWMutex
	writer *sql.DB
	reader *sql.DB
	path   string
	gate   *maintenanceGate
}

func Open(ctx context.Context, path string) (*DB, error) {
	return openWithGate(ctx, path, newMaintenanceGate())
}

func openWithGate(ctx context.Context, path string, gate *maintenanceGate) (*DB, error) {
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

	return &DB{writer: writer, reader: reader, path: path, gate: gate}, nil
}

func (db *DB) Writer() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.writer
}

func (db *DB) Reader() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.reader
}

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	writer, reader := db.writer, db.reader
	err := db.closeConnectionsLocked()
	db.writer, db.reader = writer, reader
	return err
}

func (db *DB) Path() string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.path
}

func (db *DB) BeginRequest() bool {
	return db.gate.beginRequest()
}

func (db *DB) EndRequest() {
	db.gate.endRequest()
}

func (db *DB) IsMaintenance() bool {
	return db.gate.isMaintenance()
}

func (db *DB) BeginMaintenance(ctx context.Context) error {
	return db.gate.begin(ctx)
}

func (db *DB) EndMaintenance() {
	db.gate.endMaintenance()
}

func (db *DB) closeConnectionsLocked() error {
	var readerErr, writerErr error
	if db.reader != nil {
		readerErr = db.reader.Close()
		db.reader = nil
	}
	if db.writer != nil {
		writerErr = db.writer.Close()
		db.writer = nil
	}
	return errors.Join(readerErr, writerErr)
}

func (db *DB) reopen(ctx context.Context) error {
	opened, err := openWithGate(ctx, db.path, db.gate)
	if err != nil {
		return err
	}
	db.mu.Lock()
	db.writer, db.reader = opened.writer, opened.reader
	db.mu.Unlock()
	opened.writer, opened.reader = nil, nil
	return nil
}

func (db *DB) Ready(ctx context.Context) error {
	db.mu.RLock()
	reader, writer := db.reader, db.writer
	db.mu.RUnlock()
	if reader == nil || writer == nil {
		return sql.ErrConnDone
	}
	var applied int
	if err := reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		return err
	}
	if applied == 0 {
		return ErrNotMigrated
	}

	tx, err := writer.BeginTx(ctx, nil)
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

type maintenanceGate struct {
	mu          sync.Mutex
	active      int
	maintenance bool
	zero        chan struct{}
	restore     chan struct{}
}

func newMaintenanceGate() *maintenanceGate {
	gate := &maintenanceGate{restore: make(chan struct{}, 1)}
	gate.restore <- struct{}{}
	return gate
}

func (gate *maintenanceGate) beginRestore(ctx context.Context) error {
	select {
	case <-gate.restore:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (gate *maintenanceGate) endRestore() {
	gate.restore <- struct{}{}
}

func (gate *maintenanceGate) beginRequest() bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.maintenance {
		return false
	}
	gate.active++
	return true
}

func (gate *maintenanceGate) endRequest() {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.active > 0 {
		gate.active--
	}
	if gate.active == 0 && gate.zero != nil {
		close(gate.zero)
		gate.zero = nil
	}
}

func (gate *maintenanceGate) begin(ctx context.Context) error {
	gate.mu.Lock()
	if gate.maintenance {
		gate.mu.Unlock()
		return ErrMaintenance
	}
	gate.maintenance = true
	if gate.active == 0 {
		gate.mu.Unlock()
		return nil
	}
	gate.zero = make(chan struct{})
	wait := gate.zero
	gate.mu.Unlock()
	select {
	case <-wait:
		return nil
	case <-ctx.Done():
		gate.endMaintenance()
		return ctx.Err()
	}
}

func (gate *maintenanceGate) endMaintenance() {
	gate.mu.Lock()
	gate.maintenance = false
	gate.mu.Unlock()
}

func (gate *maintenanceGate) isMaintenance() bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.maintenance
}
