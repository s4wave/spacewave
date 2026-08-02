package filelock

import (
	"context"
	"os"
	"sync"

	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/coord"
)

// lease joins an in-memory keyed lease with its advisory lock file. file is
// nil on platforms without advisory file locks.
type lease struct {
	inner coord.WriteLease
	file  *os.File

	// mtx guards released and releaseErr.
	mtx        sync.Mutex
	released   bool
	releaseErr error
}

// Done returns the inner lease channel, closed when Release completes the
// inner release. The file lock lease cannot report involuntary loss.
func (l *lease) Done() <-chan struct{} {
	return l.inner.Done()
}

// Err returns the inner lease error, nil for a held or cleanly released lease.
func (l *lease) Err() error {
	return l.inner.Err()
}

// Refresh returns the inner result: keyed scopes carry no generations.
func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	return l.inner.Refresh(ctx)
}

// Publish returns the inner result: keyed scopes carry no generations.
func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	return l.inner.Publish(ctx, event)
}

// Release unlocks and closes the lock file, then releases the in-memory
// lease so a woken local waiter finds the file lock free.
func (l *lease) Release(context.Context) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.released {
		return l.releaseErr
	}
	l.released = true

	if l.file != nil {
		if err := unlockFile(l.file); err != nil {
			l.releaseErr = pkgerrors.Wrap(err, "release lock file")
		}
		if err := l.file.Close(); err != nil && l.releaseErr == nil {
			l.releaseErr = pkgerrors.Wrap(err, "close lock file")
		}
	}
	if err := l.inner.Release(context.Background()); err != nil && l.releaseErr == nil {
		l.releaseErr = err
	}
	return l.releaseErr
}

// _ is a type assertion
var _ coord.WriteLease = (*lease)(nil)
