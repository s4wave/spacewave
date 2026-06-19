package s4wave_kv_world

import "errors"

// ErrCommitPersisted reports that KVTX committed before the world root update failed.
var ErrCommitPersisted = errors.New("kv/store: commit persisted before world root update failed")

// CommitPersistedError wraps the failed world update after the inner commit succeeded.
type CommitPersistedError struct {
	Err error
}

// Error returns the error string.
func (e *CommitPersistedError) Error() string {
	if e == nil || e.Err == nil {
		return ErrCommitPersisted.Error()
	}
	return ErrCommitPersisted.Error() + ": " + e.Err.Error()
}

// Unwrap returns the underlying world update error.
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
