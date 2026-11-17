//go:build darwin || linux || windows

package s4db

import (
	"context"
	"errors"
	"os"
)

// osFile is a database file on the operating system's file system, which
// processes share through byte locks.
type osFile struct {
	*os.File
	// path is the file's path.
	path string
	// closeFile closes the descriptor and releases its process claim.
	closeFile func() error
}

// Open opens or creates the database at path. With Options.ReadOnly it opens
// an existing file only. The file is still opened for writing: the byte locks
// and the reader slot need it.
func Open(path string, opts Options) (*DB, error) {
	return openNative(context.Background(), path, opts, false)
}

// OpenWait opens path after its previous writable handle closes. ctx bounds
// the wait. A cancelled OS wait retains its descriptor until the lock arrives,
// then closes it without recovering or modifying the file.
func OpenWait(ctx context.Context, path string, opts Options) (*DB, error) {
	return openNative(ctx, path, opts, true)
}

// openNative registers the descriptor and optionally waits for writer ownership.
func openNative(ctx context.Context, path string, opts Options, wait bool) (*DB, error) {
	// Do not create or retain a file for a request already cancelled.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Open the file once per process.
	flags := os.O_RDWR | os.O_CREATE
	if opts.ReadOnly {
		flags = os.O_RDWR
	}
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return openNativeFile(name, flags)
	})
	if err != nil {
		return nil, err
	}
	s := &osFile{File: f, path: path, closeFile: closeFile}
	if !wait && !opts.ReadOnly {
		// A normal open refuses ownership before reading or recovering bytes.
		ok, err := s.lock(lockWriterOwner, false)
		if err != nil || !ok {
			_ = s.close()
			if err != nil {
				return nil, err
			}
			return nil, ErrWriterOwned
		}
	}
	if wait && !opts.ReadOnly {
		// A single OS wait observes close and process death without polling.
		done := make(chan error, 1)
		go func() {
			ok, err := s.lock(lockWriterOwner, true)
			if err == nil && !ok {
				err = ErrWriterOwned
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				_ = s.close()
				return nil, err
			}
		case <-ctx.Done():
			go func() { <-done; _ = s.close() }()
			return nil, ctx.Err()
		}
	}

	// A cancelled grant closes its descriptor before any recovery writes.
	if err := ctx.Err(); err != nil {
		_ = s.close()
		return nil, err
	}

	// Both writable paths already hold ownership. Validate their descriptor
	// before recovery; Windows rejects acquiring the lifetime byte twice.
	if !opts.ReadOnly {
		// A failed builder may remove its partial file before releasing ownership.
		// Refuse that stale descriptor rather than returning an unlinked writer.
		if _, err := s.claimedInfo(); err != nil {
			_ = s.close()
			return nil, err
		}
	}
	return openOwned(s, opts)
}

// Build creates the database at path and writes the pairs source yields, in
// ascending key order, straight into its final shape: values over inlineMax
// into value chunks in stream order, entries into runs of runTarget at the
// lowest level whose capacity holds them, then one manifest, one superblock
// and a durable flush. No log, memtable or merge is involved, so the file
// opens with an empty memtable and nothing to merge. A key not above the one
// before it fails with ErrOutOfOrder. Build refuses an existing path and
// removes the file it created when it fails, so a partial build never passes
// for a database.
func Build(ctx context.Context, path string, opts Options, source func(yield func(key, value []byte) error) error) error {
	// Create the file and register it as open in this process.
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return openNativeFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL)
	})
	if err != nil {
		return err
	}

	s := &osFile{File: f, path: path, closeFile: closeFile}
	return buildNative(ctx, s, opts, source)
}

// buildNative owns the descriptor from its ownership claim through build cleanup.
func buildNative(ctx context.Context, s *osFile, opts Options, source func(func([]byte, []byte) error) error) (err error) {
	// Return ownership after cleanup on every exit, including a source panic.
	defer func() { err = errors.Join(err, s.close()) }()

	// Exclusive creation precedes the byte claim; another opener may have won it.
	if ok, err := s.lock(lockWriterOwner, false); err != nil || !ok {
		if err != nil {
			return err
		}
		return ErrWriterOwned
	}
	info, err := s.claimedInfo()
	if err != nil {
		return err
	}
	if info.Size() != 0 {
		// A writer completed between creation and our claim. Preserve its database.
		return &os.PathError{Op: "build", Path: s.path, Err: os.ErrExist}
	}

	// Only this owner can have written the partial build that failure removes.
	err = build(ctx, s, opts, runTarget, source)
	if err != nil {
		// Remove the partial file before a waiting owner can recover or write it.
		// Never retry removal after close: that could unlink the successor's file.
		err = errors.Join(err, os.Remove(s.path)) // #nosec G703 -- remove only the partial file this Build created.
	}
	return err
}

// claimedInfo verifies that a claimed writable descriptor still names its path.
// Call it before recovery/build writes; inode locks cannot protect a removed name.
func (f *osFile) claimedInfo() (os.FileInfo, error) {
	// Compare the name and descriptor after ownership has been acquired.
	info, err := os.Stat(f.path)
	if err != nil {
		return nil, err
	}
	held, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, held) {
		return nil, &os.PathError{Op: "open", Path: f.path, Err: errors.New("file replaced before writer ownership")}
	}
	return held, nil
}

// size returns the file length.
func (f *osFile) size() (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// truncate sets the file length.
func (f *osFile) truncate(n int64) error {
	return f.Truncate(n)
}

// watch returns a watcher of the file for the reader in slot.
func (f *osFile) watch(slot int, readOnly bool) (watcher, error) {
	w, err := newFileWatcher(f.File, f.path, slot, readOnly)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// close closes the file and releases its registration.
func (f *osFile) close() error {
	return f.closeFile()
}
