// Package database stores the record of imported camera objects in a SQLite
// database (pure-Go driver, no CGO) so that repeat imports skip files that
// are already on disk.
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ImportRecord is one row in the imports table.
type ImportRecord struct {
	ID           int64
	ObjectHandle uint32
	Filename     string
	Size         int64
	CaptureDate  time.Time
	SizeCheck    int64
	ImportedAt   time.Time
	DestPath     string
}

// DB wraps the SQLite store of imported objects.
type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS imports (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	object_handle INTEGER NOT NULL,
	filename      TEXT    NOT NULL,
	size          INTEGER NOT NULL,
	capture_date  TEXT    NOT NULL,
	size_check    INTEGER NOT NULL,
	imported_at   TEXT    NOT NULL,
	dest_path     TEXT    NOT NULL,
	UNIQUE(object_handle, filename, size)
);
CREATE INDEX IF NOT EXISTS idx_imports_handle ON imports(object_handle);
`

// Open opens (creating if necessary) the database at path and ensures the
// schema exists. The parent directory is created when missing.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("database: create dir: %w", err)
		}
	}
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}
	// modernc.org/sqlite handles one writer at a time; a single connection
	// keeps things simple and avoids SQLITE_BUSY under concurrency.
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("database: init schema: %w", err)
	}
	return &DB{sql: sqlDB}, nil
}

// Close closes the underlying database.
func (d *DB) Close() error { return d.sql.Close() }

// IsImported reports whether an object with the given handle, filename and
// size has already been imported.
func (d *DB) IsImported(handle uint32, filename string, size int64) (bool, error) {
	var one int
	err := d.sql.QueryRow(
		`SELECT 1 FROM imports WHERE object_handle = ? AND filename = ? AND size = ? LIMIT 1`,
		int64(handle), filename, size,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("database: query import: %w", err)
	}
	return true, nil
}

// ImportedSet returns the set of (handle,filename,size) keys already
// imported, for fast in-memory lookups when listing many files.
func (d *DB) ImportedSet() (map[string]bool, error) {
	rows, err := d.sql.Query(`SELECT object_handle, filename, size FROM imports`)
	if err != nil {
		return nil, fmt.Errorf("database: list imports: %w", err)
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var handle, size int64
		var name string
		if err := rows.Scan(&handle, &name, &size); err != nil {
			return nil, fmt.Errorf("database: scan import: %w", err)
		}
		set[Key(uint32(handle), name, size)] = true
	}
	return set, rows.Err()
}

// Key builds the dedupe key used by ImportedSet.
func Key(handle uint32, filename string, size int64) string {
	return fmt.Sprintf("%d|%s|%d", handle, filename, size)
}

// Record inserts an import record. Recording the same object twice is a
// no-op thanks to the UNIQUE constraint.
func (d *DB) Record(r ImportRecord) error {
	_, err := d.sql.Exec(
		`INSERT OR IGNORE INTO imports
		 (object_handle, filename, size, capture_date, size_check, imported_at, dest_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		int64(r.ObjectHandle), r.Filename, r.Size,
		r.CaptureDate.UTC().Format(time.RFC3339),
		r.SizeCheck,
		time.Now().UTC().Format(time.RFC3339),
		r.DestPath,
	)
	if err != nil {
		return fmt.Errorf("database: record import: %w", err)
	}
	return nil
}

// ImportedToday counts records imported since local midnight.
func (d *DB) ImportedToday() (int, error) {
	midnight := time.Now().Truncate(24 * time.Hour)
	// imported_at is stored in UTC; compare against local midnight in UTC.
	var n int
	err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM imports WHERE imported_at >= ?`,
		midnight.UTC().Format(time.RFC3339),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database: count today: %w", err)
	}
	return n, nil
}

// DestPath returns the recorded destination path for a handle, or "" when
// the object has not been imported.
func (d *DB) DestPath(handle uint32, filename string, size int64) (string, error) {
	var p string
	err := d.sql.QueryRow(
		`SELECT dest_path FROM imports WHERE object_handle = ? AND filename = ? AND size = ? LIMIT 1`,
		int64(handle), filename, size,
	).Scan(&p)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("database: query dest path: %w", err)
	}
	return p, nil
}
