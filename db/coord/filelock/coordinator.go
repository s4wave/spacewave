package filelock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

// lockDirName is the directory holding one lock file per keyed scope.
const lockDirName = ".coord-locks"

// Coordinator combines an inner coordinator for ObjectStore scopes with one
// advisory lock file per keyed scope for cross-process keyed exclusion.
type Coordinator struct {
	inner   coord.Coordinator
	keyed   *coord_inmem.Coordinator
	dir     string
	storeID string
}

// NewCoordinator builds a file lock coordinator over dir. Scopes without a
// Key delegate to inner. storeID must identify the complete backing store so
// sibling volumes sharing dir do not contend. On platforms without advisory
// file locks the keyed scopes fall back to in-memory exclusion.
func NewCoordinator(dir, storeID string, inner coord.Coordinator) *Coordinator {
	// Canonicalize the store identity before constructing keyed state.
	canonicalStoreID := canonicalLockStoreID(storeID)
	return &Coordinator{
		inner:   inner,
		keyed:   coord_inmem.ForVolume("filelock\x00" + canonicalStoreID),
		dir:     canonicalLockPath(dir),
		storeID: canonicalStoreID,
	}
}

// Capability reports keyed file lock support, delegating ObjectStore scopes.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	// Delegate root scopes and validate keyed capability requests.
	if scope.Key == "" {
		return c.inner.Capability(ctx, scope)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &coord.Capability{
		Supported:     true,
		Backend:       coord.BackendKindFileLock,
		VolumeID:      scope.VolumeID,
		ObjectStoreID: scope.ObjectStoreID,
	}, nil
}

// Snapshot delegates ObjectStore scopes; keyed scopes carry no generations.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	// Delegate snapshots to the coordinator selected by scope kind.
	if scope.Key == "" {
		return c.inner.Snapshot(ctx, scope)
	}
	return c.keyed.Snapshot(ctx, scope)
}

// Watch delegates ObjectStore scopes; keyed scopes carry no event stream.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	// Delegate watches to the coordinator selected by scope kind.
	if scope.Key == "" {
		return c.inner.Watch(ctx, scope, afterGeneration)
	}
	return c.keyed.Watch(ctx, scope, afterGeneration)
}

// TryAcquireWriteLease attempts to acquire the keyed file lock without blocking.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	// Delegate root scopes before acquiring the keyed lease.
	if scope.Key == "" {
		return c.inner.TryAcquireWriteLease(ctx, scope)
	}

	// Claim the in-memory keyed lease before probing the cross-process lock.
	inner, ok, err := c.keyed.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		return nil, ok, err
	}
	if !lockFilesSupported {
		return &lease{inner: inner}, true, nil
	}

	// Probe the lock file and release the keyed lease if acquisition fails.
	file, err := c.openLockFile(ctx, scope)
	if err != nil {
		_ = inner.Release(context.Background())
		return nil, false, err
	}
	locked, err := tryLockFile(file)
	if err != nil || !locked {
		_ = file.Close()
		_ = inner.Release(context.Background())
		return nil, false, wrapLockError(err)
	}
	return &lease{inner: inner, file: file}, true, nil
}

// WaitAcquireWriteLease waits until the keyed file lock is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	// Delegate root scopes before waiting for the keyed lease.
	if scope.Key == "" {
		return c.inner.WaitAcquireWriteLease(ctx, scope)
	}

	// Hold the in-memory keyed lease while waiting for the file lock.
	inner, err := c.keyed.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}
	if !lockFilesSupported {
		return &lease{inner: inner}, nil
	}

	// Open the lock file and wait for other processes to unlock it.
	file, err := c.openLockFile(ctx, scope)
	if err != nil {
		_ = inner.Release(context.Background())
		return nil, err
	}
	if err := waitLockFile(ctx, file, func() { _ = inner.Release(context.Background()) }); err != nil {
		return nil, err
	}
	return &lease{inner: inner, file: file}, nil
}

// openLockFile creates the lock directory and opens the lock file for scope.
func (c *Coordinator) openLockFile(ctx context.Context, scope coord.Scope) (*os.File, error) {
	// Validate context and lock identity before creating the lock directory.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.dir == "" {
		return nil, errors.New("filelock: lock directory cannot be empty")
	}
	if c.storeID == "" {
		return nil, errors.New("filelock: backing store identity cannot be empty")
	}

	// Create the private lock directory before opening this scope's file.
	lockDir := filepath.Join(c.dir, lockDirName)

	// #nosec G703 -- lockDir is the coordinator's configured root directory joined with a constant name.
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, pkgerrors.Wrap(err, "create lock directory")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Open the scope's lock file.
	path := filepath.Join(lockDir, lockDigest(c.storeID, scope)+".lock")

	// #nosec G703 -- path is the managed lock directory joined with a hex digest filename.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "open lock file")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// waitLockFile blocks in the kernel until file holds its advisory lock. On
// failure it closes file and calls releaseTurn. If ctx ends first, it returns
// ctx.Err() and a goroutine unlocks and closes file and calls releaseTurn once
// the blocked lock returns.
func waitLockFile(ctx context.Context, file *os.File, releaseTurn func()) error {
	// Lock in the background so the wait can follow ctx.
	locked := make(chan error, 1)
	go func() { locked <- lockFile(file) }()
	select {
	case err := <-locked:
		if err != nil {
			_ = file.Close()
			releaseTurn()
		}
		return wrapLockError(err)
	case <-ctx.Done():
	}

	// Release the file and the turn once the abandoned lock returns.
	go func() {
		if err := <-locked; err == nil {
			_ = unlockFile(file)
		}
		_ = file.Close()
		releaseTurn()
	}()
	return ctx.Err()
}

// wrapLockError labels a file lock failure, passing nil through.
func wrapLockError(err error) error {
	return pkgerrors.Wrap(err, "acquire lock file")
}

// lockDigest names the lock file for one backing store and scope.
func lockDigest(storeID string, scope coord.Scope) string {
	digest := sha256.Sum256([]byte(
		storeID + "\x00" + scope.VolumeID + "\x00" + scope.ObjectStoreID + "\x00" + scope.Key,
	))
	return hex.EncodeToString(digest[:])
}

// canonicalLockStoreID canonicalizes the path portion of a storeID that may
// carry a NUL-separated suffix distinguishing stores inside one file.
func canonicalLockStoreID(storeID string) string {
	path, suffix, hasSuffix := strings.Cut(storeID, "\x00")
	path = canonicalLockPath(path)
	if !hasSuffix {
		return path
	}
	return path + "\x00" + suffix
}

// canonicalLockPath resolves path to an absolute symlink-free form so every
// spelling of one backing store contends on the same lock files.
func canonicalLockPath(path string) string {
	// Resolve the path to an absolute symlink-free form.
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	absolute = filepath.Clean(absolute)
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return filepath.Clean(resolved)
	}
	return absolute
}

// _ is a type assertion
var _ coord.Coordinator = (*Coordinator)(nil)
