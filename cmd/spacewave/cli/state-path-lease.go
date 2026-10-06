//go:build !js && !wasip1

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// statePathLeaseFile names the database file holding the runtime lease in a
// writable state path.
const statePathLeaseFile = "runtime-lock.s4wave"

// statePathLeaseName names the runtime lease in its database file.
const statePathLeaseName = "runtime"

// statePathLease excludes writable runtimes until their bus has fully stopped.
type statePathLease struct {
	// db is the lease file, open until release.
	db *s4db.DB
	// held is the runtime lease.
	held *s4db.Lease
	// path is the lease file.
	path string
	// mtx serializes release and its recorded result.
	mtx sync.Mutex
	// released prevents closing an already released lease.
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

// acquireStatePathLease takes the runtime lease of a canonical root in its
// lease file. The system releases the lease when the process exits, and a
// process opens the file once, so a second runtime in the same process finds
// it held. When wait is set it blocks until another process releases it.
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

	// Canonicalize aliases so every runtime of the root opens one file.
	statePath, err = filepath.EvalSymlinks(statePath)
	if err != nil {
		return nil, err
	}
	leasePath := filepath.Join(statePath, statePathLeaseFile)

	// Open the lease file; this process already holding it open is a
	// same-process runtime.
	db, err := s4db.Open(leasePath, s4db.Options{})
	if errors.Is(err, s4db.ErrOpenInProcess) {
		return nil, &StatePathLeaseHeldError{
			StatePath: statePath,
			HolderPID: os.Getpid(),
			StorePath: leasePath,
		}
	}
	if err != nil {
		return nil, errors.Wrap(err, "open writable state path lease store")
	}

	// Take the lease.
	var held *s4db.Lease
	if wait {
		held, err = db.WaitLease(ctx, statePathLeaseName)
	} else {
		var ok bool
		held, ok, err = db.TryLease(statePathLeaseName)
		if err == nil && !ok {
			return nil, stderrors.Join(&StatePathLeaseHeldError{
				StatePath: statePath,
				StorePath: leasePath,
			}, db.Close())
		}
	}

	// Close the file on failure. Close waits for a wait abandoned with ctx
	// until the system grants its lock, so that close runs in the background.
	if err != nil {
		err = errors.Wrap(err, "acquire writable state path lease")
		if ctx.Err() != nil {
			go func() { _ = db.Close() }()
			return nil, err
		}
		return nil, stderrors.Join(err, db.Close())
	}
	return &statePathLease{db: db, held: held, path: leasePath}, nil
}

// release releases the runtime lease after all writable bus state closes.
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
		l.relErr = stderrors.Join(l.held.Release(), l.db.Close())
	}
	return l.relErr
}
