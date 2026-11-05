package common

import "database/sql"

// SQLiteDriverConfig defines the interface for SQLite driver configuration.
type SQLiteDriverConfig interface {
	// DriverName returns the name to use with sql.Open().
	DriverName() string
	// OpenDSN returns the DSN to use with sql.Open() for a given database path.
	OpenDSN(path string) string
	// Description returns a human-readable description of the driver.
	Description() string
	// IsBusyError checks if the error is a SQLITE_BUSY error for this driver.
	IsBusyError(err error) bool
	// SyncWAL makes durable every commit the WAL of the database at path
	// holds, where path is the file SQLite reports for the main database. A
	// driver behind a bridge returns nil.
	SyncWAL(path string) error
}

// SQLiteDriverPoolConfigurator optionally constrains the sql.DB pool created by
// common.Open. Drivers with connection-bound semantics can use this to align
// database/sql pooling with the underlying engine.
type SQLiteDriverPoolConfigurator interface {
	// ConfigureDBPool mutates the sql.DB pool settings after sql.Open.
	ConfigureDBPool(db *sql.DB)
}
