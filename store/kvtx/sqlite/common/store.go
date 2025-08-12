// Package common provides shared implementations for SQLite kvtx stores.
package common

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
)

// ValidateTableName validates that a table name is safe to use in SQL queries.
// It only allows alphanumeric characters and underscores, and must start with a letter or underscore.
func ValidateTableName(table string) error {
	if table == "" {
		return errors.New("table name cannot be empty")
	}

	// Table name must match: start with letter/underscore, followed by letters/digits/underscores
	matched, err := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, table)
	if err != nil {
		return err
	}
	if !matched {
		return errors.New("invalid table name: must start with letter or underscore and contain only alphanumeric characters and underscores")
	}

	return nil
}

// SQLiteDriverConfig defines the interface for SQLite driver configuration.
type SQLiteDriverConfig interface {
	// DriverName returns the name to use with sql.Open()
	DriverName() string
	// Description returns a human-readable description of the driver
	Description() string
	// IsBusyError checks if the error is a SQLITE_BUSY error for this driver
	IsBusyError(err error) bool
}

// Store represents a generic SQLite store that can work with any driver.
type Store[T SQLiteDriverConfig] struct {
	db     *sql.DB
	table  string
	config T
}

// NewStore constructs a new key-value store from a SQLite database.
func NewStore[T SQLiteDriverConfig](db *sql.DB, table string, config T) (*Store[T], error) {
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}
	return &Store[T]{db: db, table: table, config: config}, nil
}

// Open opens a SQLite database store using the configured driver.
func Open[T SQLiteDriverConfig](path string, table string, config T) (*Store[T], error) {
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}

	// Set WAL mode and a default busy_timeout in DSN for basic waiting on non-transactional ops.
	// For transaction-level waiting, we handle retries in NewTransaction.
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open(config.DriverName(), dsn)
	if err != nil {
		return nil, err
	}

	store, err := NewStore(db, table, config)
	if err != nil {
		db.Close()
		return nil, err
	}

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
	return err
}

// NewTransaction returns a new transaction against the store.
func (s *Store[T]) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		// Read-only tx: allows multiple concurrent readers.
		opts := &sql.TxOptions{ReadOnly: true}
		txn, err := s.db.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}
		return NewTx(txn, s.table, write), nil
	}

	// Write tx: acquire RESERVED lock early to serialize writers.
	// Retry on SQLITE_BUSY
	backoff := 10 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		default:
		}

		txn, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}

		// Dummy non-modifying DELETE to acquire RESERVED lock early.
		_, err = txn.ExecContext(ctx, "DELETE FROM "+s.table+" WHERE 1=0")
		if err == nil {
			// Success: RESERVED acquired, proceed.
			return NewTx(txn, s.table, write), nil
		}

		// Rollback on failure.
		txn.Rollback()

		// Check if SQLITE_BUSY; if so, backoff and retry.
		if !s.config.IsBusyError(err) {
			return nil, err
		}

		// Constant-time backoff.
		time.Sleep(backoff)
	}
}

// Execute executes the given store.
func (s *Store[T]) Execute(ctx context.Context) error {
	return nil
}

// Close closes the database.
func (s *Store[T]) Close() error {
	return s.db.Close()
}

// _ is a type assertion
var _ kvtx.Store = ((*Store[SQLiteDriverConfig])(nil))
