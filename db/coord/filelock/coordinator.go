package filelock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

const (
	// lockDirName is the directory holding one lock file per keyed scope.
	lockDirName = ".coord-locks"
	// crossProcessLockRetryDelay paces the non-blocking file lock probe:
	// external processes cannot broadcast their release here.
	crossProcessLockRetryDelay = 250 * time.Millisecond
)

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
	if scope.Key == "" {
		return c.inner.Snapshot(ctx, scope)
	}
	return c.keyed.Snapshot(ctx, scope)
}

// Watch delegates ObjectStore scopes; keyed scopes carry no event stream.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	if scope.Key == "" {
		return c.inner.Watch(ctx, scope, afterGeneration)
	}
	return c.keyed.Watch(ctx, scope, afterGeneration)
}

// TryAcquireWriteLease attempts to acquire the keyed file lock without blocking.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	if scope.Key == "" {
		return c.inner.TryAcquireWriteLease(ctx, scope)
	}

	inner, ok, err := c.keyed.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		return nil, ok, err
	}
	if !lockFilesSupported {
		return &lease{inner: inner}, true, nil
	}

	file, locked, err := c.openLockedFile(ctx, scope)
	if err != nil || !locked {
		_ = inner.Release(context.Background())
		return nil, locked, err
	}
	return &lease{inner: inner, file: file}, true, nil
}

// WaitAcquireWriteLease waits until the keyed file lock is available.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	if scope.Key == "" {
		return c.inner.WaitAcquireWriteLease(ctx, scope)
	}

	for {
		inner, err := c.keyed.WaitAcquireWriteLease(ctx, scope)
		if err != nil {
			return nil, err
		}
		if !lockFilesSupported {
			return &lease{inner: inner}, nil
		}

		file, locked, err := c.openLockedFile(ctx, scope)
		if err != nil {
			_ = inner.Release(context.Background())
			return nil, err
		}
		if locked {
			return &lease{inner: inner, file: file}, nil
		}
		_ = inner.Release(context.Background())

		timer := time.NewTimer(crossProcessLockRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// openLockedFile opens and try-locks the lock file for scope, returning the
// locked file, whether the lock was acquired, and any error.
func (c *Coordinator) openLockedFile(ctx context.Context, scope coord.Scope) (*os.File, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c.dir == "" {
		return nil, false, errors.New("filelock: lock directory cannot be empty")
	}
	if c.storeID == "" {
		return nil, false, errors.New("filelock: backing store identity cannot be empty")
	}

	lockDir := filepath.Join(c.dir, lockDirName)
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, false, pkgerrors.Wrap(err, "create lock directory")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	path := filepath.Join(lockDir, lockDigest(c.storeID, scope)+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, pkgerrors.Wrap(err, "open lock file")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, false, err
	}

	locked, err := tryLockFile(file)
	if err != nil {
		_ = file.Close()
		return nil, false, pkgerrors.Wrap(err, "acquire lock file")
	}
	if !locked {
		_ = file.Close()
		return nil, false, nil
	}
	return file, true, nil
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
