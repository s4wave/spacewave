//go:build !js && !wasip1

package volume_s4db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/s4db/s4dbv1"
	"github.com/s4wave/spacewave/db/util/diskfree"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// convertingSuffix and the other constants are suffixes for files a conversion
// from the previous format keeps beside the Volume file.
const (
	// convertingSuffix is the suffix of the new file while the file is
	// built and verified.
	convertingSuffix = ".converting"
	// convertedMarkerSuffix is the suffix of the marker holding the new
	// file's digest, written once every pair is read back.
	convertedMarkerSuffix = ".converting.ok"
	// preRewriteSuffix is the suffix of the hard link that keeps the
	// previous file until a person removes the link.
	preRewriteSuffix = ".pre-rewrite"
)

// convertProgressInterval is the least time between progress log lines.
const convertProgressInterval = 10 * time.Second

// Open opens the s4db file at path with opts, first converting in place a file
// the previous engine wrote, as convertPrevious describes. Every caller that
// opens a file a previous release may have written opens it here, so upgrading
// a host needs no manual step. A read-only open of such a file fails with
// volume.ErrReadOnlyUpgrade instead.
func Open(ctx context.Context, le *logrus.Entry, path string, opts s4db.Options) (*s4db.DB, error) {
	return openDatabase(ctx, le, path, opts, false)
}

// OpenWait is Open with a cancellable wait for the previous writable opener.
func OpenWait(ctx context.Context, le *logrus.Entry, path string, opts s4db.Options) (*s4db.DB, error) {
	return openDatabase(ctx, le, path, opts, true)
}

// openDatabase opens or converts the file with the caller's writer wait policy.
func openDatabase(ctx context.Context, le *logrus.Entry, path string, opts s4db.Options, wait bool) (*s4db.DB, error) {
	// Follow a symbolic link at path, so a conversion replaces its target
	// and the directory synced below is the one holding the file.
	path, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	// Sync the directory after a writable open binds its file and before
	// the file takes a write: a bolt conversion's rename, made by this
	// process or another, may not be durable yet and keeps no claim that
	// says so, and until it is a crash brings the bolt file back. A sync
	// before the open would miss a rename between the two.
	open := func() (*s4db.DB, error) {
		// Bind the file with the caller's writer wait policy.
		openFile := s4db.Open
		if wait {
			openFile = func(path string, opts s4db.Options) (*s4db.DB, error) {
				return s4db.OpenWait(ctx, path, opts)
			}
		}
		db, err := openFile(path, opts)
		if err != nil || opts.ReadOnly || runtime.GOOS == "windows" {
			return db, err
		}

		// Make the name that bound it durable, resolving path again: the
		// open created the target of a symbolic link that named no file.
		bound, err := resolvePath(path)
		if err == nil {
			err = syncDir(filepath.Dir(bound))
		}
		if err != nil {
			return nil, errors.Join(err, db.Close())
		}
		return db, nil
	}

	// Finish an installation whose directory sync failed before the file
	// takes a write.
	if !opts.ReadOnly {
		if err := finishInstalled(path); err != nil {
			return nil, err
		}
	}
	db, err := open()
	if !errors.Is(err, s4db.ErrPreviousFormat) {
		return db, err
	}

	// Convert a previous-format file, which a read-only open cannot write.
	if opts.ReadOnly {
		return nil, volume.Permanent(errors.Join(volume.ErrReadOnlyUpgrade, err))
	}
	if runtime.GOOS == "windows" {
		return nil, volume.Permanent(errors.Join(ErrConvertUnsupported, err))
	}
	fence, err := convertPrevious(ctx, le, path)
	processFences.hold(fence)
	if err != nil {
		return nil, err
	}
	return open()
}

// resolvePath follows the symbolic links of path, returning path itself when
// it names no file yet.
func resolvePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	return resolved, err
}

// fenceSet holds the fenced previous files of a process's conversions, as
// Reader.Fence describes, until the process exits. A previous-engine process
// that opened a file before its conversion replaced the name could otherwise
// write the backup after the fence closed.
type fenceSet struct {
	// mu guards held.
	mu sync.Mutex
	// held are the fences, never closed.
	held []io.Closer
}

// processFences holds this process's fences.
var processFences = &fenceSet{}

// hold keeps fence until the process exits. A nil fence is ignored.
func (s *fenceSet) hold(fence io.Closer) {
	if fence == nil {
		return
	}
	s.mu.Lock()
	s.held = append(s.held, fence)
	s.mu.Unlock()
}

// convertPrevious replaces the previous-format database at path with one in
// the current format holding the same pairs, and keeps the previous file as
// <path>.pre-rewrite, so going back is a rename and the previous binary.
//
// It holds the previous file's writer lock and a slot from the stream through
// the replace, so no process writes the previous file meanwhile, and returns
// the fenced previous file once the replace happened. The caller holds that
// fence while a previous-engine process may still wait on the writer lock,
// since the fence is what keeps such a process off the backup. A file
// another process has open fails with ErrMigrateShared.
//
// The conversion's files keep these invariants:
//
//  1. Nothing is removed that a claim of this conversion does not account
//     for: <path>.converting.ok claims the conversion's files before
//     <path>.converting exists, a file of either name without that claim is
//     refused, and each removal checks the file's identity first.
//  2. The new file is installed only after every pair read back from it
//     matches the digest of the locked source.
//  3. At every point there is at least one complete copy of the data: the
//     source stays at path until <path>.pre-rewrite names it durably.
//  4. A write reaches the new file only after its rename is durable: a
//     failed directory sync leaves the claim, and the next open syncs again
//     before it opens the file.
//  5. The backup holds exactly the pairs the new file started with: the
//     fence refuses previous-engine processes that opened the source before
//     the replace.
//
// A crash before the digest leaves the source in place with an unverified
// claim the next open rebuilds. A crash after it leaves a copy the next open
// checks against the current source. A crash between the link and the
// rename leaves the source under both names, and the next open keeps the
// link.
func convertPrevious(ctx context.Context, le *logrus.Entry, path string) (io.Closer, error) {
	// Open the previous file, taking its locks for the build.
	src, err := s4dbv1.OpenExclusive(path)
	switch {
	case errors.Is(err, s4dbv1.ErrUnsupported), errors.Is(err, s4dbv1.ErrNotDatabase):
		return nil, nil
	case errors.Is(err, s4dbv1.ErrInUse):
		return nil, ErrMigrateShared
	case err != nil:
		return nil, err
	}

	// Convert, then fence the source once its name was replaced, else
	// release it. A source that fails to fence keeps its writer lock.
	replaced, err := convertSource(ctx, le, path, src)
	if !replaced {
		return nil, errors.Join(err, src.Close())
	}
	if ferr := src.Fence(); ferr != nil {
		le.WithError(ferr).Warn("previous volume file keeps its writer lock until the process exits")
	}
	return src, err
}

// convertSource converts the previous file src, which OpenExclusive opened at
// path, and reports whether it replaced the name path, after which src must
// stay open as convertPrevious describes.
func convertSource(ctx context.Context, le *logrus.Entry, path string, src *s4dbv1.Reader) (bool, error) {
	// Log with the path and time the conversion.
	le = le.WithField("path", path)
	start := time.Now()

	// Refuse when the disk cannot hold the new file beside the old. A person
	// has to free disk or move the file, so the failure is permanent.
	free, err := diskfree.Bytes(filepath.Dir(path))
	if err == nil {
		err = checkConvertSpace(src.LiveBytes(), free)
	}
	if err != nil {
		return false, volume.Permanent(err)
	}

	// Name the source for the claim.
	fi, err := src.Stat()
	if err != nil {
		return false, err
	}
	id, err := fileIdentity(fi)
	if err != nil {
		return false, err
	}

	// Reuse a verified new file an earlier attempt left, else build one.
	tmp, marker := path+convertingSuffix, path+convertedMarkerSuffix
	want, resumed, err := resumeConverted(ctx, src, id, tmp, marker)
	if err == nil && !resumed {
		le.Info("converting volume to the current database format")
		want, err = buildConverted(ctx, src, id, tmp, marker, logConvertProgress(le))
	}
	if err != nil {
		return false, err
	}

	// Put it in place.
	replaced, err := installConverted(path)
	if err != nil {
		return replaced, err
	}
	le.WithFields(logrus.Fields{"keys": want.n, "elapsed": time.Since(start).Round(time.Millisecond)}).Info("converted volume to the current database format")
	return replaced, nil
}

// checkConvertSpace refuses a conversion when free, the bytes free on the
// disk, is below the live bytes of the previous file plus a tenth.
func checkConvertSpace(live int64, free uint64) error {
	need := uint64(live) + uint64(live)/10 // #nosec G115 -- live bytes are not negative.
	if free < need {
		return &NoSpaceError{Need: need, Free: free}
	}
	return nil
}

// resumeConverted reuses the new file an earlier attempt left only when the
// claim names the source id, its digest matches the current locked source,
// and every pair of the new file matches it. A claimed attempt that does not
// is removed, since the source is authoritative. Files no claim accounts for
// are refused.
func resumeConverted(ctx context.Context, src *s4dbv1.Reader, id, tmp, marker string) (digest, bool, error) {
	// Find the files the claim accounts for.
	files, err := inspectClaim(id, tmp, marker)
	if err != nil {
		return digest{}, false, err
	}

	// Compare the digest with the current source, which may have changed
	// after an interrupted attempt released its lock, and read the new
	// file back.
	if c := files.claim; c.verified && files.tmp != nil {
		ok, err := matchesSource(ctx, src, c.want)
		if err != nil {
			return digest{}, false, err
		}
		if ok && verifyConverted(ctx, tmp, c.want) == nil {
			return c.want, true, nil
		}
		if err := ctx.Err(); err != nil {
			return digest{}, false, err
		}
	}

	// Remove the claimed attempt while the complete source is locked.
	return digest{}, false, files.clear(tmp, marker)
}

// matchesSource reports whether the pairs of src have digest want.
func matchesSource(ctx context.Context, src *s4dbv1.Reader, want digest) (bool, error) {
	var current digest
	h := blake3.New()
	err := src.Stream(ctx, func(key, value []byte) error {
		current.add(h, key, value)
		return nil
	})
	return current == want, err
}

// buildConverted claims the conversion files for the source id, streams the
// previous file src into a new file at tmp, reads every pair of the new file
// back against the stream's digest, and records the digest in the claim. A
// new file that fails is removed.
func buildConverted(ctx context.Context, src *s4dbv1.Reader, id, tmp, marker string, progress func(keys, bytes int64)) (digest, error) {
	// Claim the files, then build, digesting the stream. Build creates tmp
	// exclusively.
	if err := writeClaim(marker, id); err != nil {
		return digest{}, err
	}
	want, err := streamConverted(ctx, src, tmp, progress)
	if err != nil {
		return digest{}, err
	}
	built, err := os.Lstat(tmp)
	if err != nil {
		return digest{}, err
	}

	// Read it back and record the digest.
	if err := verifyConverted(ctx, tmp, want); err != nil {
		return digest{}, errors.Join(err, removeSame(tmp, built))
	}
	return want, appendDigest(marker, want)
}

// streamConverted builds the new file at tmp from the pairs of src in key
// order and returns their digest, calling progress as it goes.
func streamConverted(ctx context.Context, src *s4dbv1.Reader, tmp string, progress func(keys, bytes int64)) (digest, error) {
	// Build from the stream, digesting each pair on the way.
	var want digest
	var bytes int64
	h := blake3.New()
	err := s4db.Build(ctx, tmp, s4db.Options{}, func(yield func(key, value []byte) error) error {
		return src.Stream(ctx, func(key, value []byte) error {
			want.add(h, key, value)
			bytes += int64(len(key) + len(value))
			if progress != nil {
				progress(want.n, bytes)
			}
			return yield(key, value)
		})
	})
	return want, err
}

// verifyConverted reads every pair of the new file at tmp and compares the
// digest with want.
func verifyConverted(ctx context.Context, tmp string, want digest) error {
	// Open the file read-only and walk every key.
	db, err := s4db.Open(tmp, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()

	// Digest each pair and compare.
	var got digest
	h := blake3.New()
	for it.Next() {
		v, err := it.Value()
		if err != nil {
			return err
		}
		got.add(h, it.Key(), v)
	}
	if err := it.Err(); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("converted volume holds %d keys that do not match the %d previous keys", got.n, want.n)
	}
	return nil
}

// installConverted keeps the previous file at path as <path>.pre-rewrite,
// renames the verified new file over path and drops the claim, reporting
// whether it replaced the name. A link an earlier attempt made is kept. The
// caller holds the source's writer lock throughout.
func installConverted(path string) (bool, error) {
	// Make the new file and the claim durable before any name refers to
	// them.
	dir := filepath.Dir(path)
	if err := syncDir(dir); err != nil {
		return false, err
	}

	// Keep the previous file under its pre-rewrite name.
	pre := path + preRewriteSuffix
	if err := os.Link(path, pre); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return false, err
		}
		same, err := sameFile(path, pre)
		if err != nil {
			return false, err
		}
		if !same {
			return false, fmt.Errorf("%s exists and is not the volume file; move it aside to convert", pre)
		}
	}

	// Make the backup name durable before replacing the active source name.
	if err := syncDir(dir); err != nil {
		return false, err
	}

	// Put the new file in place, then make that durable and drop the claim.
	if err := os.Rename(path+convertingSuffix, path); err != nil {
		return false, err
	}
	_, _, markerInfo, err := readClaim(path + convertedMarkerSuffix)
	if err != nil {
		return true, err
	}
	return true, completeInstall(dir, path+convertedMarkerSuffix, markerInfo)
}

// completeInstall makes a rename into dir durable, then removes the claim
// marker that markerInfo describes and makes that durable. Until the claim is
// gone, finishInstalled repeats it at the next open.
func completeInstall(dir, marker string, markerInfo os.FileInfo) error {
	if err := syncDir(dir); err != nil {
		return err
	}
	if markerInfo != nil {
		if err := removeSame(marker, markerInfo); err != nil {
			return err
		}
	}
	return syncDir(dir)
}

// finishInstalled completes an installation at path whose last steps failed
// or did not run: a claim naming the file at <path>.pre-rewrite, beside a
// different file at path, belongs to a conversion that renamed its new file
// into place. Any other state is left alone.
func finishInstalled(path string) error {
	// Find a claim that names the backup.
	marker := path + convertedMarkerSuffix
	state, c, markerInfo, err := readClaim(marker)
	if err != nil || state != claimHeld {
		return err
	}
	pre, err := os.Stat(path + preRewriteSuffix)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if id, err := fileIdentity(pre); err != nil || id != c.src {
		return err
	}

	// Leave a conversion that has not renamed yet.
	cur, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || os.SameFile(cur, pre) {
		return err
	}
	return completeInstall(filepath.Dir(path), marker, markerInfo)
}

// sameFile reports whether the paths a and b name the same file.
func sameFile(a, b string) (bool, error) {
	// Compare the two files' identities.
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(fa, fb), nil
}

// logConvertProgress returns a progress function that logs the keys and
// bytes streamed at most once per convertProgressInterval.
func logConvertProgress(le *logrus.Entry) func(keys, bytes int64) {
	last := time.Now()
	return func(keys, bytes int64) {
		if time.Since(last) < convertProgressInterval {
			return
		}
		last = time.Now()
		le.WithFields(logrus.Fields{"keys": keys, "bytes": bytes}).Info("converting volume")
	}
}

// CopyPreviousFormat copies the stopped previous-format database at src into
// a new file at dst in the current format, reading every pair back before it
// returns, and calls progress with the keys and bytes streamed so far. It
// leaves src unchanged and removes dst when the copy fails.
func CopyPreviousFormat(ctx context.Context, src, dst string, progress func(keys, bytes int64)) error {
	// Open the source read-only, refusing one a writer or a conversion holds.
	r, err := s4dbv1.Open(src)
	if errors.Is(err, s4dbv1.ErrInUse) {
		return fmt.Errorf("%s is open in another process; stop the daemon first", src)
	}
	if err != nil {
		return err
	}
	defer r.Close()

	// Build and verify the copy.
	want, err := streamConverted(ctx, r, dst, progress)
	if err != nil {
		return err
	}
	if err := verifyConverted(ctx, dst, want); err != nil {
		return errors.Join(err, os.Remove(dst))
	}
	return nil
}
