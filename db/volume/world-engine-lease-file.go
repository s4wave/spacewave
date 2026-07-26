//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || solaris || windows

package volume

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	pkgerrors "github.com/pkg/errors"
)

const worldEngineLeaseDir = ".world-engine-leases"

// NewFileWorldEngineLeaseProvider constructs a kernel-backed lease provider
// using lock files beside the volume's local data directory. storeID must
// identify the complete backing store so sibling volumes do not contend.
func NewFileWorldEngineLeaseProvider(dir, storeID string) WorldEngineLeaseProvider {
	return &fileWorldEngineLeaseProvider{
		dir:     canonicalWorldEngineLeasePath(dir),
		storeID: canonicalWorldEngineLeaseStoreID(storeID),
	}
}

func canonicalWorldEngineLeaseStoreID(storeID string) string {
	path, suffix, hasSuffix := strings.Cut(storeID, "\x00")
	path = canonicalWorldEngineLeasePath(path)
	if !hasSuffix {
		return path
	}
	return path + "\x00" + suffix
}

func canonicalWorldEngineLeasePath(path string) string {
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

type fileWorldEngineLeaseProvider struct {
	dir     string
	storeID string
}

func (p *fileWorldEngineLeaseProvider) AcquireWorldEngineLease(
	ctx context.Context,
	key string,
) (WorldEngineLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrWorldEngineLeaseKeyEmpty
	}
	if p.dir == "" {
		return nil, errors.New("world engine lease directory cannot be empty")
	}
	if p.storeID == "" {
		return nil, errors.New("world engine lease backing store identity cannot be empty")
	}

	leaseDir := filepath.Join(p.dir, worldEngineLeaseDir)
	if err := os.MkdirAll(leaseDir, 0o700); err != nil {
		return nil, pkgerrors.Wrap(err, "create world engine lease directory")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	digest := worldEngineLeaseDigest(p.storeID, key)
	path := filepath.Join(leaseDir, digest+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "open world engine lease")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, err
	}

	locked, err := tryLockWorldEngineLease(file)
	if err != nil {
		_ = file.Close()
		return nil, pkgerrors.Wrap(err, "acquire world engine lease")
	}
	if !locked {
		_ = file.Close()
		return nil, &WorldEngineLeaseHeldError{Key: key}
	}

	return &fileWorldEngineLease{file: file, done: make(chan struct{})}, nil
}

type fileWorldEngineLease struct {
	file *os.File
	done chan struct{}
	once sync.Once
	err  error
}

func (l *fileWorldEngineLease) Done() <-chan struct{} {
	return l.done
}

func (*fileWorldEngineLease) Err() error {
	return nil
}

func (l *fileWorldEngineLease) Release() error {
	l.once.Do(func() {
		if err := unlockWorldEngineLease(l.file); err != nil {
			l.err = pkgerrors.Wrap(err, "release world engine lease")
		}
		if err := l.file.Close(); err != nil && l.err == nil {
			l.err = pkgerrors.Wrap(err, "close world engine lease")
		}
		close(l.done)
	})
	return l.err
}
