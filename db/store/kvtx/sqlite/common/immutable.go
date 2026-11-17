package common

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
)

// Immutable reads a checkpointed SQLite capture without a writable store API.
// Source bytes and metadata stay unchanged: no source write, free, rebuild,
// repair, pin, stage or maintenance call is reachable through this type.
// The caller guarantees the capture is isolated and immutable for its lifetime.
// SQLite's immutable mode bypasses recovery, locking and journal creation.
type Immutable struct {
	// db owns the read-only pool, with one bounded connection and page cache.
	db *sql.DB
	// table is the validated key/value table identifier.
	table string
	// limit bounds a single encoded record before allocating its value.
	limit int
}

// OpenImmutable opens an existing, checkpointed capture without startup work.
// It rejects WAL and rollback-journal data rather than silently omitting it.
func OpenImmutable(ctx context.Context, path, table, driver string, limit int) (*Immutable, error) {
	// Require a bounded value size and an existing regular source file.
	if limit <= 0 {
		return nil, errors.New("immutable record limit must be positive")
	}
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("immutable source must be a regular file")
	}

	// Refuse captures that need journal recovery or checkpointing.
	for _, suffix := range []string{"-wal", "-journal"} {
		info, err := os.Stat(abs + suffix)
		if err == nil && info.Size() != 0 {
			return nil, errors.New("immutable source has an uncheckpointed journal")
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	// Open the native driver physically read-only with URI path escaping.
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: "mode=ro&immutable=1"}
	db, err := sql.Open(driver, u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	reader := &Immutable{db: db, table: table, limit: limit}

	// Configure the sole connection before reading the capture schema.
	if _, err := db.ExecContext(ctx, "PRAGMA cache_size=-2048; PRAGMA mmap_size=0; PRAGMA query_only=ON"); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	var present int
	if err := db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&present); err != nil {
		return nil, errors.Join(err, db.Close())
	}

	// Require the native single-key index before relying on bounded keyset seeks.
	if err := validateImmutableTable(ctx, db, table); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return reader, nil
}

// Get copies one bounded value, reporting absence separately from empty bytes.
// One query reads the value's length and selects the value only within the
// limit, so the driver never allocates an oversized value.
func (r *Immutable) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Read the length, and the value only within the limit.
	var size int
	var data []byte
	err := r.db.QueryRowContext(ctx, "SELECT length(value),CASE WHEN length(value)<=? THEN value END FROM "+r.table+" WHERE key=?", r.limit, key).Scan(&size, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if size > r.limit {
		return nil, false, errors.New("immutable source record exceeds decoder bound")
	}
	if data == nil {
		data = []byte{}
	}
	return data, true, nil
}

// Next copies the first key after after, or the first key when after is nil.
// Each call closes its SQL row before the caller processes or writes scratch.
// One query seeks through the primary key and selects the key and value only
// when both are BLOBs within the limit, so the driver never allocates an
// oversized or mistyped record.
func (r *Immutable) Next(ctx context.Context, after []byte) ([]byte, []byte, error) {
	// Seek through the primary key without loading or retaining the inventory.
	const ok = "typeof(key)='blob' AND typeof(value)='blob' AND length(key)<=?1 AND length(value)<=?1"
	query := "SELECT length(key),length(value),typeof(key),typeof(value)," +
		"CASE WHEN " + ok + " THEN key END,CASE WHEN " + ok + " THEN value END FROM " + r.table
	args := []any{r.limit}
	if after != nil {
		query += " WHERE key>?2"
		args = append(args, after)
	}
	query += " ORDER BY key LIMIT 1"

	// Check the types and sizes the query bounded the copy by.
	var keySize, size int
	var keyType, valueType string
	var key, data []byte
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&keySize, &size, &keyType, &valueType, &key, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	// Refuse a record the query did not copy.
	if keyType != "blob" || valueType != "blob" {
		return nil, nil, errors.New("immutable table requires BLOB keys and values")
	}
	if keySize > r.limit || size > r.limit {
		return nil, nil, errors.New("immutable source record exceeds decoder bound")
	}
	if key == nil {
		key = []byte{}
	}
	return key, data, nil
}

// Close releases only the read-only connection; it runs no store upkeep.
func (r *Immutable) Close() error {
	return r.db.Close()
}

// validateImmutableTable requires the native single-key index using SQLite's stable schema pragma.
func validateImmutableTable(ctx context.Context, db *sql.DB, table string) error {
	// Stream the bounded table schema rather than relying on version-specific catalog aliases.
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	primary, keyPrimary := 0, 0
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if pk > 0 {
			primary++
		}
		if name == "key" {
			keyPrimary = pk
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if primary != 1 || keyPrimary != 1 {
		return errors.New("immutable table requires a single primary key")
	}
	return nil
}
