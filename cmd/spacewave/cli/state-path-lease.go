//go:build !js && !wasip1

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"sync"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	"github.com/sirupsen/logrus"
)

// statePathLeaseStorageID identifies the cross-process runtime coordination store.
const statePathLeaseStorageID = "runtime-lease"

// localStatePathLeases complements process-scoped OS locks for same-process callers.
var localStatePathLeases = struct {
	sync.Mutex
	// paths holds canonical lease paths while acquisition or a runtime is active.
	paths map[string]struct{}
}{paths: make(map[string]struct{})}

// statePathLease excludes writable runtimes until their bus has fully stopped.
type statePathLease struct {
	// db holds the cross-process coordination lock until release.
	db *bdb.DB
	// path identifies the process-local lease reservation.
	path string
	// mtx serializes release and its recorded result.
	mtx sync.Mutex
	// released prevents closing an already released lock.
	released bool
	// relErr retains the first release result.
	relErr error
}

// prepareDaemonRuntime acquires writable-state exclusion before inspecting or
// removing the socket. Explicit takeover completes before acquiring the lease.
func prepareDaemonRuntime(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	sockPath string,
	takeover bool,
) (*statePathLease, error) {
	// Only an explicit serve takeover may ask the previous runtime to yield.
	if takeover {
		if err := takeoverDaemonSocket(ctx, le, sockPath); err != nil {
			return nil, err
		}
	}

	// Socket cleanup runs under the lease so simultaneous starters cannot unlink
	// the winning runtime's new listener after observing its stale predecessor.
	lease, err := acquireStatePathLease(statePath)
	if err != nil {
		return nil, err
	}
	if err := listener_control.EnsureSocketAvailable(ctx, le, sockPath); err != nil {
		return nil, stderrors.Join(err, lease.release())
	}
	return lease, nil
}

// acquireStatePathLease reserves a canonical root locally and with bbolt's
// kernel coordination lock, which the OS releases when the process exits.
func acquireStatePathLease(statePath string) (*statePathLease, error) {
	// Create only the requested root before canonicalizing its filesystem identity.
	statePath, err := filepath.Abs(statePath)
	if err != nil {
		return nil, errors.Wrap(err, "resolve writable state path")
	}
	if err := os.MkdirAll(statePath, 0o700); err != nil {
		return nil, err
	}

	// Canonicalize aliases before reserving a process-scoped file lock.
	statePath, err = filepath.EvalSymlinks(statePath)
	if err != nil {
		return nil, err
	}
	leasePath, err := storage_native.BoltDBPath(statePath, statePathLeaseStorageID)
	if err != nil {
		return nil, err
	}

	// Reserve the root locally before taking its cross-process lease.
	localStatePathLeases.Lock()
	if _, held := localStatePathLeases.paths[leasePath]; held {
		localStatePathLeases.Unlock()
		return nil, &StatePathLeaseHeldError{
			StatePath: statePath,
			HolderPID: os.Getpid(),
			StorePath: leasePath,
		}
	}
	localStatePathLeases.paths[leasePath] = struct{}{}
	localStatePathLeases.Unlock()
	claimed := true
	defer func() {
		if !claimed {
			return
		}
		localStatePathLeases.Lock()
		delete(localStatePathLeases.paths, leasePath)
		localStatePathLeases.Unlock()
	}()

	// Open the coordination store and attempt its nonblocking runtime lock.
	db, err := bdb.Open(leasePath, 0o600, &bdb.Options{
		Timeout:        0,
		NoFreelistSync: false,
		NoGrowSync:     false,
		FreelistType:   bdb.FreelistMapType,
		NoSync:         false,
	})
	if err != nil {
		return nil, errors.Wrap(err, "open writable state path lease store")
	}
	acquired, err := db.TryAcquireCoordinationLock()
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			err = stderrors.Join(err, closeErr)
		}
		return nil, errors.Wrap(err, "acquire writable state path lease")
	}
	if !acquired {
		return nil, stderrors.Join(&StatePathLeaseHeldError{
			StatePath: statePath,
			StorePath: leasePath,
		}, db.Close())
	}

	// Transfer the local reservation and kernel lock to the runtime. Store
	// sidecar PIDs can be reused after a crash and do not establish ownership.
	claimed = false
	return &statePathLease{db: db, path: leasePath}, nil
}

// release relinquishes the coordination lock after all writable bus state closes.
func (l *statePathLease) release() error {
	// Allow setup cleanup before a lease has been acquired.
	if l == nil {
		return nil
	}

	// Serialize repeated cleanup from setup failure and the bus release path.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if !l.released {
		l.released = true
		releaseErr := l.db.ReleaseCoordinationLock()
		closeErr := l.db.Close()
		localStatePathLeases.Lock()
		delete(localStatePathLeases.paths, l.path)
		localStatePathLeases.Unlock()
		l.relErr = stderrors.Join(releaseErr, closeErr)
	}
	return l.relErr
}
