// Package common provides shared implementations for SQLite kvtx stores.
package common

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// ValidateTableName validates that a table name is safe to use in SQL queries.
// It only allows alphanumeric characters and underscores, and must start with a letter or underscore.
func ValidateTableName(table string) error {
	// Require a nonempty SQLite table name before validating its characters.
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

// Store represents a generic SQLite store that can work with any driver.
type Store[T SQLiteDriverConfig] struct {
	// db owns the SQL connection pool and its transaction lifetimes.
	db *sql.DB
	// table is the validated key-value table name.
	table string
	// config classifies driver errors for bounded recovery.
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
func Open[T SQLiteDriverConfig](ctx context.Context, path string, table string, config T) (*Store[T], error) {
	return OpenWithPragmas(ctx, path, table, Pragmas{}, config)
}

// OpenWithPragmas opens a SQLite database store using the configured driver
// and applies the supplied tunable pragmas. Zero-valued pragma fields leave
// the SQLite default in place.
func OpenWithPragmas[T SQLiteDriverConfig](ctx context.Context, path string, table string, pragmas Pragmas, config T) (*Store[T], error) {
	// Reject an invalid table before opening the database.
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}

	// Set WAL mode, synchronous=NORMAL, and busy_timeout in DSN when supported.
	// Explicit PRAGMAs below remain a safety net for drivers that ignore DSN params.
	dsn := config.OpenDSN(path)
	db, err := sql.Open(config.DriverName(), dsn)
	if err != nil {
		return nil, err
	}
	if poolConfigurator, ok := any(config).(SQLiteDriverPoolConfigurator); ok {
		poolConfigurator.ConfigureDBPool(db)
	}

	// page_size must be applied before the WAL header is written and before
	// any tables are created on a fresh database. On an existing database it
	// has no effect unless followed by VACUUM, which we do not run here.
	if pragmas.PageSize != 0 {
		if _, err := db.ExecContext(ctx, "PRAGMA page_size="+strconv.FormatInt(int64(pragmas.PageSize), 10)); err != nil {
			db.Close()
			return nil, err
		}
	}

	// Configure the initial connection. Connectors own settings on later connections.
	synchronous := "NORMAL"
	if pragmas.FullSync {
		synchronous = "FULL"
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=" + synchronous,
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, err
		}
	}

	// Tunable pragmas. Each applies only when the caller supplied a non-zero
	// value. cache_size and temp_store are connection-scoped but cover the
	// initial connection used for setup; mmap_size is database-level.
	if pragmas.CacheSize != 0 {
		if _, err := db.ExecContext(ctx, "PRAGMA cache_size="+strconv.FormatInt(int64(pragmas.CacheSize), 10)); err != nil {
			db.Close()
			return nil, err
		}
	}
	if pragmas.MmapSize != 0 {
		if _, err := db.ExecContext(ctx, "PRAGMA mmap_size="+strconv.FormatInt(pragmas.MmapSize, 10)); err != nil {
			db.Close()
			return nil, err
		}
	}
	if pragmas.TempStore != 0 {
		if _, err := db.ExecContext(ctx, "PRAGMA temp_store="+strconv.FormatInt(int64(pragmas.TempStore), 10)); err != nil {
			db.Close()
			return nil, err
		}
	}

	// Initialize the table through the same path used by injected SQL pools.
	return OpenDB(ctx, db, table, config)
}

// OpenDB takes ownership of an already configured SQL pool and opens its KV table.
// It closes the pool on failure; the caller selects durability on every connection.
func OpenDB[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, config T) (*Store[T], error) {
	// Bind the validated table to its supplied database pool.
	store, err := NewStore(db, table, config)
	if err != nil {
		db.Close()
		return nil, err
	}

	// Initialize table with retry on SQLITE_BUSY.
	// This can happen when another process is closing the database and holds
	// an exclusive lock during WAL cleanup, or when recovering from a crash.
	if err := store.initTableWithRetry(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode[T SQLiteDriverConfig](ctx context.Context, path string, mode os.FileMode, table string, config T) (*Store[T], error) {
	return OpenWithModeAndPragmas(ctx, path, mode, table, Pragmas{}, config)
}

// OpenWithModeAndPragmas opens a SQLite database store with file mode and the
// supplied tunable pragmas.
func OpenWithModeAndPragmas[T SQLiteDriverConfig](ctx context.Context, path string, mode os.FileMode, table string, pragmas Pragmas, config T) (*Store[T], error) {
	// For SQLite, we can create the file with the specified mode before opening
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if file, err := os.OpenFile(path, os.O_CREATE, mode); err == nil {
			file.Close()
		}
	}

	return OpenWithPragmas(ctx, path, table, pragmas, config)
}

// GetDB returns the SQL DB.
func (s *Store[T]) GetDB() *sql.DB {
	return s.db
}

// initTable creates the key-value table if it doesn't exist.
func (s *Store[T]) initTable() error {
	query := `CREATE TABLE IF NOT EXISTS ` + s.table + ` ( ` + //nolint:gosec
		`key BLOB PRIMARY KEY, value BLOB)`
	_, err := s.db.Exec(query)
	return err
}

// initTableWithRetry creates the key-value table with retry on SQLITE_BUSY.
func (s *Store[T]) initTableWithRetry(ctx context.Context) error {
	// Bound SQLite table initialization retries and their backoff.
	backoff := 50 * time.Millisecond
	maxBackoff := 2 * time.Second
	maxRetries := 10

	// Retry SQLite table initialization while retaining the last busy error.
	var lastErr error
	for range maxRetries {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := s.initTable()
		if err == nil {
			return nil
		}

		lastErr = err
		if !s.config.IsBusyError(err) {
			return err
		}

		// Backoff before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential backoff with cap
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return errors.Join(errors.New("sqlite: database busy during initialization after retries"), lastErr)
}

// NewTransaction returns a new transaction against the store.
func (s *Store[T]) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	backoff := 10 * time.Millisecond
	maxRetries := 5

	for range maxRetries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		tx, err := s.tryNewTransaction(ctx, write)
		if err == nil {
			return tx, nil
		}

		// Retry on nested transaction error - this can happen when the connection
		// pool returns a connection that still has an active transaction due to
		// driver issues or connection pool timing.
		if !s.config.IsNestedTxError(err) {
			return nil, err
		}

		// Backoff before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, errors.New("sqlite: failed to start transaction after retries (nested transaction error)")
}

// tryNewTransaction attempts to create a new transaction.
func (s *Store[T]) tryNewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		return NewReadTx(s.db, s.table), nil
	}

	// Write tx: acquire RESERVED lock early to serialize writers.
	// Retry on SQLITE_BUSY
	backoff := 10 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		txn, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}

		// Dummy non-modifying DELETE to acquire RESERVED lock early.
		_, err = txn.ExecContext(ctx, "DELETE FROM "+s.table+" WHERE 1=0") //nolint:gosec
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

		// Backoff before retry.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
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
var _ kvtx.Store = (*Store[SQLiteDriverConfig])(nil)
