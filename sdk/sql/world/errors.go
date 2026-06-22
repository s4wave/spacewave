package s4wave_sql_world

import "errors"

// ErrCommitPersisted reports that SQL committed before the world root update failed.
var ErrCommitPersisted = errors.New("sql/db: commit persisted before world root update failed")

// CommitPersistedError wraps an error after the inner SQL root already committed.
type CommitPersistedError struct {
	Err error
}

// Error returns the commit persistence error message.
func (e *CommitPersistedError) Error() string {
	if e == nil || e.Err == nil {
		return ErrCommitPersisted.Error()
	}
	return ErrCommitPersisted.Error() + ": " + e.Err.Error()
}

// Unwrap returns the wrapped root update error.
func (e *CommitPersistedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is matches ErrCommitPersisted.
func (e *CommitPersistedError) Is(target error) bool {
	return target == ErrCommitPersisted
}
