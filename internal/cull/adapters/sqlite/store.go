// Package sqlite is the feature store adapter. SQLite is authoritative for
// analysis results; XMP sidecars are a projection of what is kept here.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// schema follows the architecture's design: normalized observations rather
// than one opaque JSON blob, so results stay queryable and a photograph's
// tagging can be explained from the numbers behind it.
const schema = `
CREATE TABLE IF NOT EXISTS images (
    id               TEXT PRIMARY KEY,
    path             TEXT NOT NULL UNIQUE,
    file_size        INTEGER NOT NULL,
    mtime_ns         INTEGER NOT NULL,
    fingerprint      TEXT,
    analysis_version TEXT,
    perceptual_hash  INTEGER,
    capture_time     TEXT,
    width            INTEGER,
    height           INTEGER,
    camera           TEXT,
    lens             TEXT,
    focal_length     REAL,
    aperture         REAL,
    shutter_seconds  REAL,
    iso              INTEGER
);

CREATE TABLE IF NOT EXISTS analyzers (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    version       TEXT NOT NULL,
    model_name    TEXT,
    model_version TEXT,
    model_sha256  TEXT NOT NULL DEFAULT '',
    UNIQUE(name, version, model_sha256)
);

CREATE TABLE IF NOT EXISTS observations (
    image_id    TEXT NOT NULL,
    analyzer_id INTEGER NOT NULL,
    key         TEXT NOT NULL,
    value       REAL NOT NULL,
    confidence  REAL,
    -- Empty string rather than NULL: SQLite does not enforce uniqueness over
    -- NULL primary key columns, so a NULL here would let duplicates through.
    region_id   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    PRIMARY KEY (image_id, analyzer_id, key, region_id)
);

CREATE TABLE IF NOT EXISTS "groups" (
    id             TEXT PRIMARY KEY,
    kind           TEXT,
    representative TEXT
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id      TEXT NOT NULL,
    image_id      TEXT NOT NULL,
    similarity    REAL,
    rank_score    REAL,
    rank_position INTEGER,
    PRIMARY KEY (group_id, image_id)
);

CREATE TABLE IF NOT EXISTS tags (
    image_id TEXT NOT NULL,
    tag      TEXT NOT NULL,
    source   TEXT NOT NULL,
    value    TEXT,
    PRIMARY KEY (image_id, tag, source)
);

CREATE INDEX IF NOT EXISTS observations_by_image ON observations(image_id);
CREATE INDEX IF NOT EXISTS group_members_by_image ON group_members(image_id);
`

// Store persists images, observations, groups and tags.
type Store struct {
	db *sql.DB
}

// Open connects to the database, creating it and its schema when absent.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// WAL keeps a reader from blocking the writer; the busy timeout absorbs
	// the brief contention when another ansel process holds the lock.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}
	// Writes are serialized by the application, and SQLite takes a single
	// writer lock regardless; one connection makes that explicit.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema in %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
