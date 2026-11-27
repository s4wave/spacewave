//go:build !js

package checkoutroot

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/coord/filelock"
)

// uploadStagePrefix reserves internal names for checkout-root publication.
const uploadStagePrefix = ".spacewave-upload-"

// uploadLeaseSuffix distinguishes the owner marker from the staged bytes.
const uploadLeaseSuffix = ".lease"

// uploadLeaseMarker identifies a payload created by the checkout-root publisher.
const uploadLeaseMarker = "spacewave checkout-root upload v1\n"

// uploadStage holds the payload and its cross-process cleanup lease.
type uploadStage struct {
	// dir pins the common directory of the lease and payload.
	dir *os.Root
	// name is the unpublished payload's name.
	name string
	// lease protects the payload until its publisher finishes with it.
	lease *os.File
	// file receives the staged bytes.
	file *os.File
	// journal records how to find an interrupted stage.
	journal *uploadJournal
}

// openNamedStage registers the locked stage before creating its payload.
func openNamedStage(ctx context.Context, journal *uploadJournal, dir *os.Root, directory string) (*uploadStage, error) {
	// The lease marks only files in a freshly allocated namespace.
	name := uploadStagePrefix + rand.Text()
	leaseName := name + uploadLeaseSuffix
	lease, err := dir.OpenFile(leaseName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}

	// Remove the unused lease and its record if staging never starts.
	var stage *uploadStage
	var recorded bool
	defer func() {
		if stage != nil {
			return
		}
		_ = lease.Close()
		if err := dir.Remove(leaseName); err == nil && recorded {
			_ = journal.forget(context.WithoutCancel(ctx), name)
		}
	}()
	locked, err := filelock.TryLockExclusive(lease)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errors.New("upload staging lease is already locked")
	}

	// Keep the lease locked across registration and every payload write.
	if err := journal.record(ctx, name, directory); err != nil {
		return nil, err
	}
	recorded = true

	// Claim the payload exclusively before marking it as the publisher's.
	file, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := lease.WriteString(uploadLeaseMarker); err != nil {
		_ = file.Close()
		_ = dir.Remove(name)
		return nil, err
	}
	stage = &uploadStage{dir: dir, name: name, lease: lease, file: file, journal: journal}
	return stage, nil
}

// prepare validates the payload and closes it with its publication metadata.
func (s *uploadStage) prepare(src io.Reader, size int64, mode fs.FileMode, ts time.Time) error {
	// Finish the bytes before applying the publication timestamp.
	if err := writeStage(s.file, src, size, mode); err != nil {
		return err
	}
	if err := s.dir.Chtimes(s.name, ts, ts); err != nil {
		return err
	}
	return s.file.Close()
}

// discard releases the stage and leaves its lease marker if the payload cannot be removed.
func (s *uploadStage) discard(ctx context.Context) error {
	// Close the payload so that it can be removed on Windows as well.
	_ = s.file.Close()
	if err := removeLeasedStage(s.dir, s.name, s.lease); err != nil {
		return err
	}

	// A cancelled stream still owns its stage until cleanup finishes.
	return s.journal.forget(context.WithoutCancel(ctx), s.name)
}

// removeLeasedStage removes the stage while its lease excludes other publishers.
func removeLeasedStage(dir *os.Root, name string, lease *os.File) error {
	// Keep the marker available for recovery when payload cleanup fails.
	err := dir.Remove(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(err, lease.Close())
	}

	// The payload is gone, so another collector can only find an empty lease.
	return errors.Join(lease.Close(), dir.Remove(name+uploadLeaseSuffix))
}

// hiddenUploadName keeps staging names out of live folders, including on case-insensitive mounts.
func hiddenUploadName(name string) bool {
	return len(name) >= len(uploadStagePrefix) && strings.EqualFold(name[:len(uploadStagePrefix)], uploadStagePrefix)
}
