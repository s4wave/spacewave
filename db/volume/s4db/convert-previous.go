//go:build !js && !wasip1

package volume_s4db

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/s4db/s4dbv1"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// Files a conversion from the previous format keeps beside the Volume file.
const (
	// convertingSuffix names the new file while it is built and verified.
	convertingSuffix = ".converting"
	// convertedMarkerSuffix names the marker holding the new file's digest,
	// written once every pair read back.
	convertedMarkerSuffix = ".converting.ok"
	// preRewriteSuffix names the hard link that keeps the previous file
	// until a person removes it.
	preRewriteSuffix = ".pre-rewrite"
)

// convertProgressInterval is the least time between progress log lines.
const convertProgressInterval = 10 * time.Second

// convertPrevious replaces the previous-format database at path with one in
// the current format holding the same pairs, and keeps the previous file as
// <path>.pre-rewrite, so going back is a rename and the previous binary.
//
// It holds the previous file's writer lock and a slot for the whole build, so
// no process writes it meanwhile; a file another process has open fails with
// ErrMigrateShared. The new file is built as <path>.converting, read back
// whole and compared with the stream before <path>.converting.ok records its
// digest, then linked and renamed into place. A crash before the marker
// leaves the previous file in place and a stale file the next open removes;
// a crash after it leaves a verified file the next open installs; a crash
// between the link and the rename leaves the previous file under both names,
// and the next open keeps the link. At no point is there less than one
// complete copy of the data. A file another process converted meanwhile is
// left for the caller to reopen.
func convertPrevious(ctx context.Context, le *logrus.Entry, path string) error {
	// Open the previous file, taking its locks for the build.
	src, err := s4dbv1.Open(path)
	switch {
	case errors.Is(err, s4dbv1.ErrUnsupported), errors.Is(err, s4dbv1.ErrNotDatabase):
		return nil
	case errors.Is(err, s4dbv1.ErrInUse):
		return ErrMigrateShared
	case err != nil:
		return err
	}
	le = le.WithField("path", path)
	start := time.Now()

	// Refuse when the disk cannot hold the new file beside the old. A person
	// has to free disk or move the file, so the failure is permanent.
	free, err := freeDiskBytes(path)
	if err == nil {
		err = checkConvertSpace(src.LiveBytes(), free)
	}
	if err != nil {
		return volume.Permanent(errors.Join(err, src.Close()))
	}

	// Reuse a verified new file an earlier attempt left, else build one.
	tmp, marker := path+convertingSuffix, path+convertedMarkerSuffix
	want, resumed, err := resumeConverted(ctx, tmp, marker)
	if err == nil && !resumed {
		le.Info("converting volume to the current database format")
		want, err = buildConverted(ctx, src, tmp, marker, logConvertProgress(le))
	}
	if err := errors.Join(err, src.Close()); err != nil {
		return err
	}

	// Put it in place.
	if err := installConverted(path); err != nil {
		return err
	}
	le.WithFields(logrus.Fields{"keys": want.n, "elapsed": time.Since(start).Round(time.Millisecond)}).Info("converted volume to the current database format")
	return nil
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

// resumeConverted reports whether tmp is a verified new file whose marker
// holds its digest, and removes the pair when it is not.
func resumeConverted(ctx context.Context, tmp, marker string) (digest, bool, error) {
	// A verified file opens read-only and holds the marker's count.
	want, ok := readConvertMarker(marker)
	if ok {
		ok = convertedHolds(ctx, tmp, want)
	}
	if ok {
		return want, true, nil
	}

	// Remove what an unfinished attempt left.
	for _, name := range []string{tmp, marker} {
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return digest{}, false, err
		}
	}
	return digest{}, false, nil
}

// convertedHolds reports whether the file at tmp opens and holds want.n keys.
func convertedHolds(ctx context.Context, tmp string, want digest) bool {
	// Open it read-only and compare the key count.
	db, err := s4db.Open(tmp, s4db.Options{ReadOnly: true})
	if err != nil {
		return false
	}
	defer db.Close()
	st, err := db.Stats(ctx)
	return err == nil && st.KeyCount == want.n
}

// buildConverted streams the previous file src into a new file at tmp, reads
// every pair of the new file back against the stream's digest, and writes
// the digest to marker. A new file that fails is removed.
func buildConverted(ctx context.Context, src *s4dbv1.Reader, tmp, marker string, progress func(keys, bytes int64)) (digest, error) {
	// Build, digesting the stream.
	want, err := streamConverted(ctx, src, tmp, progress)
	if err != nil {
		return digest{}, err
	}

	// Read it back and record the digest.
	if err := verifyConverted(ctx, tmp, want); err != nil {
		return digest{}, errors.Join(err, os.Remove(tmp))
	}
	return want, writeConvertMarker(marker, want)
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

// writeConvertMarker records d in the marker file durably.
func writeConvertMarker(marker string, d digest) error {
	f, err := os.Create(marker) // #nosec G304 -- beside the Volume file.
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%d %s\n", d.n, hex.EncodeToString(d.x[:]))
	return errors.Join(err, f.Sync(), f.Close())
}

// readConvertMarker reads the digest a marker file records, reporting false
// for a missing or unreadable marker.
func readConvertMarker(marker string) (digest, bool) {
	// Read the two fields.
	b, err := os.ReadFile(marker) // #nosec G304 -- beside the Volume file.
	if err != nil {
		return digest{}, false
	}
	fields := strings.Fields(string(b))
	if len(fields) != 2 {
		return digest{}, false
	}

	// Parse the count and the hash.
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return digest{}, false
	}
	x, err := hex.DecodeString(fields[1])
	if err != nil || len(x) != 32 {
		return digest{}, false
	}
	return digest{n: n, x: [32]byte(x)}, true
}

// installConverted keeps the previous file at path as <path>.pre-rewrite
// and renames the verified new file over path. A link an earlier attempt
// made is kept, and a new file another process already installed is left.
func installConverted(path string) error {
	// Make the new file and the marker durable before any name refers to
	// them.
	dir := filepath.Dir(path)
	if err := syncDir(dir); err != nil {
		return err
	}

	// Keep the previous file under its pre-rewrite name.
	pre := path + preRewriteSuffix
	if err := os.Link(path, pre); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		same, err := sameFile(path, pre)
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("%s exists and is not the volume file; move it aside to convert", pre)
		}
	}

	// Put the new file in place and drop the marker.
	if err := os.Rename(path+convertingSuffix, path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.Remove(path + convertedMarkerSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDir(dir)
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
	// Open the source, refusing one a process has open.
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
