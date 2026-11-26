package filelock

import (
	"os"
	"sync"
)

// claimed holds the canonical paths TryOpen has locked in this process on a
// platform without advisory file locks.
var claimed = &pathClaims{paths: make(map[string]struct{})}

// pathClaims is a set of paths claimed in this process.
type pathClaims struct {
	// mtx guards paths.
	mtx sync.Mutex
	// paths holds the claimed paths.
	paths map[string]struct{}
}

// TryOpen creates or opens path and tries to lock it exclusively without
// waiting. A nil release with no error means another opener holds the lock.
// The caller must release a held lock once and leave the lock file in place.
// On a platform without advisory file locks, such as a JavaScript runtime
// that is the only process on its filesystem, the lock excludes openers in
// this process only.
func TryOpen(path string) (release func(), err error) {
	// Claim the path in this process where the kernel cannot exclude.
	unclaim := func() {}
	if !lockFilesSupported {
		var ok bool
		if unclaim, ok = claimed.claim(canonicalLockPath(path)); !ok {
			return nil, nil
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G703 -- the caller chooses the lock file.
	if err != nil {
		unclaim()
		return nil, err
	}

	// Close a failed attempt immediately. Explicitly unlock before closing
	// a successful attempt so Windows can grant the next lock at once.
	locked, err := tryLockFile(file)
	if err != nil || !locked {
		_ = file.Close()
		unclaim()
		return nil, err
	}
	return func() {
		_ = unlockFile(file)
		_ = file.Close()
		unclaim()
	}, nil
}

// claim claims path, returning false if a claim holds it, and otherwise the
// func that drops the claim.
func (c *pathClaims) claim(path string) (func(), bool) {
	// Refuse a path another opener holds.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if _, held := c.paths[path]; held {
		return nil, false
	}

	// Hold the path until the returned func drops it.
	c.paths[path] = struct{}{}
	return func() {
		c.mtx.Lock()
		delete(c.paths, path)
		c.mtx.Unlock()
	}, true
}
