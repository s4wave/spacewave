//go:build !js

package checkoutroot

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/s4wave/spacewave/db/coord/filelock"
	"github.com/sirupsen/logrus"
)

// sweepUploads reclaims interrupted named stages in bounded directory batches.
func sweepUploads(ctx context.Context, le *logrus.Entry, dir *os.Root) {
	// Hold a directory handle while walking this level of the checkout.
	reader, err := dir.Open(".")
	if err != nil {
		le.WithError(err).Debug("open checkout directory for upload cleanup")
		return
	}
	defer reader.Close()

	// Follow directories without following symlink entries.
	for ctx.Err() == nil {
		entries, err := reader.ReadDir(64)
		for _, entry := range entries {
			if ctx.Err() != nil {
				return
			}
			if entry.IsDir() {
				child, err := dir.OpenRoot(entry.Name())
				if err == nil {
					sweepUploads(ctx, le, child)
					_ = child.Close()
				}
				continue
			}
			if entry.Type().IsRegular() {
				sweepUploadLease(le, dir, entry.Name())
			}
		}

		// Stop on EOF and report unreadable directories without blocking requests.
		if err != nil {
			if !errors.Is(err, io.EOF) {
				le.WithError(err).Debug("read checkout directory for upload cleanup")
			}
			return
		}
	}
}

// sweepUploadLease removes a stage only when a recognized marker is unlocked.
func sweepUploadLease(le *logrus.Entry, dir *os.Root, entry string) {
	// Match only the publisher's own generated lease names.
	name, ok := strings.CutSuffix(entry, uploadLeaseSuffix)
	if !ok || !strings.HasPrefix(name, uploadStagePrefix) || len(name) != len(uploadStagePrefix)+26 {
		return
	}
	info, err := dir.Lstat(entry)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	lease, err := dir.OpenFile(entry, os.O_RDWR, 0)
	if err != nil {
		return
	}

	// An active upload owns the lease. An incomplete marker has no known payload.
	opened, err := lease.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = lease.Close()
		return
	}
	locked, err := filelock.TryLockExclusive(lease)
	if err != nil || !locked {
		_ = lease.Close()
		return
	}
	var marker [len(uploadLeaseMarker) + 1]byte
	n, _ := lease.ReadAt(marker[:], 0)
	if string(marker[:n]) != uploadLeaseMarker {
		_ = lease.Close()
		return
	}

	// Keep unexpected objects in the reserved namespace for inspection.
	info, err = dir.Lstat(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) || info != nil && !info.Mode().IsRegular() {
		_ = lease.Close()
		return
	}
	if err := removeLeasedStage(dir, name, lease); err != nil {
		le.WithError(err).Debug("reclaim interrupted upload")
	}
}
