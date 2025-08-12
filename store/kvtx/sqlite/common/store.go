// Package common provides shared implementations for SQLite kvtx stores.
package common

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"github.com/aperturerobotics/hydra/kvtx"
)

// SQLiteDriverConfig defines the interface for SQLite driver configuration.
type SQLiteDriverConfig interface {
	// DriverName returns the name to use with sql.Open()
	DriverName() string
	// Description returns a human-readable description of the driver
	Description() string
}

// Store represents a generic SQLite store that can work with any driver.
type Store[T SQLiteDriverConfig] struct {
	db     *sql.DB
	table  string
	config T
}

// NewStore constructs a new key-value store from a SQLite database.
func NewStore[T SQLiteDriverConfig](db *sql.DB, table string, config T) *Store[T] {
	return &Store[T]{db: db, table: table, config: config}
}

// Open opens a SQLite database store using the configured driver.
func Open[T SQLiteDriverConfig](path string, table string, config T) (*Store[T], error) {
	if table == "" {
		return nil, errors.New("table name cannot be empty")
	}

	db, err := sql.Open(config.DriverName(), path)
	if err != nil {
		return nil, err
	}

	store := NewStore(db, table, config)
	if err := store.initTable(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode[T SQLiteDriverConfig](path string, mode os.FileMode, table string, config T) (*Store[T], error) {
	// For SQLite, we can create the file with the specified mode before opening
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if file, err := os.OpenFile(path, os.O_CREATE, mode); err == nil {
			file.Close()
		}
	}
	return Open(path, table, config)
}

// GetDB returns the SQL DB.
func (s *Store[T]) GetDB() *sql.DB {
	return s.db
}

// initTable creates the key-value table if it doesn't exist.
func (s *Store[T]) initTable() error {
	query := `CREATE TABLE IF NOT EXISTS ` + s.table + ` (
		key BLOB PRIMARY KEY,
		value BLOB
	)`
	_, err := s.db.Exec(query)
	if err == nil {
		_, err = s.db.Exec("PRAGMA journal_mode=WAL;")
	}
	return err
}

// NewTransaction returns a new transaction against the store.
func (s *Store[T]) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	var opts *sql.TxOptions
	if !write {
		opts = &sql.TxOptions{ReadOnly: true}
	}

	txn, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return NewTx(txn, s.table, write), nil
}

// Execute executes the given store.
func (s *Store[T]) Execute(ctx context.Context) error {
	return nil
}

// Close closes the database.
func (s *Store[T]) Close() error {
	return s.db.Close()
}
