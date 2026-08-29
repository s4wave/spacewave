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

const statePathLeaseStorageID = "runtime-lease"

var localStatePathLeases = struct {
	sync.Mutex
	paths map[string]struct{}
}{paths: make(map[string]struct{})}

// StatePathLeaseHeldError reports the process that owns a writable state path.
type StatePathLeaseHeldError struct {
	StatePath string
	HolderPID int
	StorePath string
}

// Error returns the terminal writable-state conflict.
func (e *StatePathLeaseHeldError) Error() string {
	if e.HolderPID > 0 && e.StorePath != "" {
		return errors.Errorf("writable state path %s is held by PID %d through store %s", e.StatePath, e.HolderPID, e.StorePath).Error()
	}
	if e.HolderPID > 0 {
		return errors.Errorf("writable state path %s is held by PID %d", e.StatePath, e.HolderPID).Error()
	}
	if e.StorePath != "" {
		return errors.Errorf("writable state path %s is held by a writer-capable process through store %s", e.StatePath, e.StorePath).Error()
	}
	return errors.Errorf("writable state path %s is held by another process", e.StatePath).Error()
}

type statePathLease struct {
	db     *bdb.DB
	path   string
	once   sync.Once
	relErr error
}

func prepareDaemonRuntime(
	ctx context.Context,
	le *logrus.Entry,
	statePath string,
	sockPath string,
	takeover bool,
) (*statePathLease, error) {
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		return nil, err
	}
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	var err error
	if takeover {
		err = takeoverDaemonSocket(ctx, le, sockPath)
	} else {
		err = listener_control.EnsureSocketAvailable(ctx, le, sockPath)
	}
	if err != nil {
		return nil, err
	}
	return acquireStatePathLease(statePath)
}

func acquireStatePathLease(statePath string) (*statePathLease, error) {
	statePath, err := filepath.Abs(statePath)
	if err != nil {
		return nil, errors.Wrap(err, "resolve writable state path")
	}
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		return nil, err
	}
	holderPID, holderStore, err := findWritableStoreLeaseHolder(statePath)
	if err != nil {
		return nil, errors.Wrap(err, "inspect writable provider-store leases")
	}
	if holderStore != "" {
		return nil, &StatePathLeaseHeldError{
			StatePath: statePath,
			HolderPID: holderPID,
			StorePath: holderStore,
		}
	}
	leasePath, err := storage_native.BoltDBPath(statePath, statePathLeaseStorageID)
	if err != nil {
		return nil, err
	}

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
		holderPID, holderErr := findWritableStoreLeaseHolderAt(leasePath, os.Getpid())
		if closeErr := db.Close(); closeErr != nil {
			holderErr = stderrors.Join(holderErr, closeErr)
		}
		if holderErr != nil {
			return nil, errors.Wrap(holderErr, "read writable state path lease holder")
		}
		return nil, &StatePathLeaseHeldError{
			StatePath: statePath,
			HolderPID: holderPID,
			StorePath: leasePath,
		}
	}

	claimed = false
	return &statePathLease{db: db, path: leasePath}, nil
}

func (l *statePathLease) release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		releaseErr := l.db.ReleaseCoordinationLock()
		closeErr := l.db.Close()
		localStatePathLeases.Lock()
		delete(localStatePathLeases.paths, l.path)
		localStatePathLeases.Unlock()
		l.relErr = stderrors.Join(releaseErr, closeErr)
	})
	return l.relErr
}
