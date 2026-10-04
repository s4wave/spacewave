//go:build !js && !wasip1

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"net"
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

// runtimeClaim selects how a starting runtime treats a live runtime that holds
// its state path.
type runtimeClaim int

const (
	// claimFree requires the state path to be free.
	claimFree runtimeClaim = iota
	// claimYield asks a runtime started on demand for a command to yield.
	claimYield
	// claimTakeover asks any runtime to yield.
	claimTakeover
)

// prepareDaemonRuntime acquires writable-state exclusion before inspecting or
// removing the socket. Explicit takeover completes before acquiring the lease.
// A runtime that yields releases its socket before its lease, so the claimant
// waits for the lease after a handoff.
func prepareDaemonRuntime(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	sockPath string,
	claim runtimeClaim,
) (*statePathLease, error) {
	// An explicit takeover asks the previous runtime to yield first.
	var handedOff bool
	if claim == claimTakeover {
		var err error
		handedOff, err = takeoverDaemonSocket(ctx, le, sockPath)
		if err != nil {
			return nil, err
		}
	}

	// A manual start asks a runtime started for a command to yield the lease.
	lease, err := acquireStatePathLease(ctx, statePath, handedOff)
	var held *StatePathLeaseHeldError
	if claim == claimYield && errors.As(err, &held) {
		if yieldErr := requestDaemonYield(ctx, sockPath); yieldErr != nil {
			var denyErr *listener_control.DenyError
			if !errors.As(yieldErr, &denyErr) {
				le.WithError(yieldErr).Debug("could not ask the running daemon to yield")
				return nil, err
			}
			return nil, errors.Wrap(
				err,
				"the running daemon was not started by a command; "+
					"use serve --takeover to replace it",
			)
		}
		lease, err = acquireStatePathLease(ctx, statePath, true)
	}
	if err != nil {
		return nil, err
	}

	// Socket cleanup runs under the lease so simultaneous starters cannot unlink
	// the winning runtime's new listener after observing its stale predecessor.
	if err := listener_control.EnsureSocketAvailable(ctx, le, sockPath); err != nil {
		return nil, stderrors.Join(err, lease.release())
	}
	return lease, nil
}

// requestDaemonYield asks the runtime listening on sockPath to yield to a
// manual start. It never removes the socket: a runtime still starting under
// the lease may be about to bind it.
func requestDaemonYield(ctx context.Context, sockPath string) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	return listener_control.RequestYield(ctx, conn)
}

// acquireStatePathLease reserves a canonical root locally and with bbolt's
// kernel coordination lock, which the OS releases when the process exits.
// When wait is set it blocks until another process releases the lock.
func acquireStatePathLease(
	ctx context.Context,
	statePath string,
	wait bool,
) (*statePathLease, error) {
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
		if claimed {
			releaseLocalStatePathLease(leasePath)
		}
	}()

	// Open the coordination store and take its runtime lock.
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
	acquired := true
	if wait {
		var abandoned bool
		abandoned, err = waitCoordinationLock(ctx, db, leasePath)
		if abandoned {
			// The waiting goroutine now owns the store and reservation.
			claimed = false
			return nil, err
		}
	} else {
		acquired, err = db.TryAcquireCoordinationLock()
	}
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

// waitCoordinationLock blocks until db holds its coordination lock. If ctx
// ends first it reports abandoned with ctx.Err() and hands db and the local
// reservation of leasePath to a goroutine that releases both once the
// blocked acquire returns; closing db earlier would block on that acquire.
func waitCoordinationLock(
	ctx context.Context,
	db *bdb.DB,
	leasePath string,
) (abandoned bool, err error) {
	// Acquire in the background so the wait can follow ctx.
	acquired := make(chan error, 1)
	go func() { acquired <- db.AcquireCoordinationLock() }()
	select {
	case err := <-acquired:
		return false, err
	case <-ctx.Done():
	}

	// Release the lock if it arrives after cancellation.
	go func() {
		if err := <-acquired; err == nil {
			_ = db.ReleaseCoordinationLock()
		}
		_ = db.Close()
		releaseLocalStatePathLease(leasePath)
	}()
	return true, ctx.Err()
}

// releaseLocalStatePathLease drops the process-local reservation of leasePath.
func releaseLocalStatePathLease(leasePath string) {
	localStatePathLeases.Lock()
	delete(localStatePathLeases.paths, leasePath)
	localStatePathLeases.Unlock()
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
		releaseLocalStatePathLease(l.path)
		l.relErr = stderrors.Join(releaseErr, closeErr)
	}
	return l.relErr
}
