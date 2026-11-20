//go:build !js

package checkoutroot

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord/filelock"
	"github.com/sirupsen/logrus"
)

// sweepUploads checks the recorded locations and leaves active stages to their publishers.
func (j *uploadJournal) sweepUploads(ctx context.Context, le *logrus.Entry, root *os.Root) {
	// Let foreground writers take precedence over journal cleanup.
	ctx = block.WithBackground(ctx)
	for key := ""; ctx.Err() == nil; {
		next, directory, err := j.next(ctx, key)
		if err != nil {
			le.WithError(err).Debug("read upload journal")
			return
		}
		if next == "" {
			return
		}

		// Look only in the recorded directory for the corresponding lease.
		key = next
		name := strings.TrimPrefix(key, j.prefix)
		if sweepUploadDirectory(le, root, directory, name) {
			if err := j.forget(ctx, name); err != nil {
				le.WithError(err).Debug("clear interrupted upload from journal")
			}
		}
	}
}

// sweepUploadDirectory resolves a journal entry within the pinned checkout root.
func sweepUploadDirectory(le *logrus.Entry, root *os.Root, directory, name string) bool {
	// Require a generated name and a directory relative to the serving root.
	suffix, ok := strings.CutPrefix(name, uploadStagePrefix)
	if !ok || len(suffix) != 26 || strings.Trim(suffix, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" {
		return false
	}
	path, err := filepath.Localize(directory)
	if err != nil {
		return false
	}
	dir, err := root.OpenRoot(path)
	if err != nil {
		return false
	}
	defer dir.Close()
	return sweepUploadLease(le, dir, name)
}

// sweepUploadLease removes a stage only when its recognized marker is unlocked.
// It reports whether this location can be dropped from the journal.
func sweepUploadLease(le *logrus.Entry, dir *os.Root, name string) bool {
	// A journal hint alone never authorizes removal of a host file.
	entry := name + uploadLeaseSuffix
	info, err := dir.Lstat(entry)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	lease, err := dir.OpenFile(entry, os.O_RDWR, 0)
	if err != nil {
		return false
	}

	// An active upload owns the lease. An incomplete marker has no known payload.
	opened, err := lease.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = lease.Close()
		return false
	}
	locked, err := filelock.TryLockExclusive(lease)
	if err != nil || !locked {
		_ = lease.Close()
		return false
	}
	var marker [len(uploadLeaseMarker) + 1]byte
	n, err := lease.ReadAt(marker[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = lease.Close()
		return false
	}
	if string(marker[:n]) != uploadLeaseMarker {
		_ = lease.Close()
		return true
	}

	// Keep unexpected objects in the reserved namespace for inspection.
	info, err = dir.Lstat(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) || info != nil && !info.Mode().IsRegular() {
		_ = lease.Close()
		return false
	}
	if err := removeLeasedStage(dir, name, lease); err != nil {
		le.WithError(err).Debug("reclaim interrupted upload")
		return false
	}
	return true
}
