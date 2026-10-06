//go:build !js && !wasip1

package volume_s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	bdb_errors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
	"golang.org/x/sync/errgroup"
)

// boltMagic is the magic number in a bbolt meta page.
const boltMagic = 0xED0CDAED

// boltMagicOff is the file offset of the first meta page's magic number,
// after the page header.
const boltMagicOff = 16

// boltBucket is the bucket a bolt Volume keeps every key in.
var boltBucket = []byte("hydra")

// convertBatch is the number of value bytes copied per s4db commit.
const convertBatch = 64 << 20

// convertReaders is the number of key ranges read at once. bbolt reads
// through page faults, so parallel ranges keep several device reads in
// flight where one cursor waits on each in turn.
const convertReaders = 16

// convertSplits is the number of times the key space is halved, giving up
// to 64 ranges so the readers stay busy when range sizes differ.
const convertSplits = 6

// convertChunk is the number of value bytes a reader hands the writer at
// once.
const convertChunk = 4 << 20

// convertBolt replaces a bolt Volume file at path with an s4db file holding
// the same keys, so the Volume keeps its key, peer ID, and data. It does nothing
// when the file is absent or not a bolt file.
//
// The copy goes to a sibling file renamed over path once verified, so a
// crash leaves the bolt file in place for the next open to convert again.
func convertBolt(ctx context.Context, le *logrus.Entry, path string) error {
	// Read the magic number.
	isBolt, err := isBoltFile(path)
	if err != nil || !isBolt {
		return err
	}

	// Lock the bolt file against other converters. A converter that opens
	// the path after another replaced it, or waited for the lock while it
	// did, leaves the path alone.
	src, err := bdb.Open(path, 0o600, &bdb.Options{Exclusive: true})
	if errors.Is(err, bdb_errors.ErrInvalid) {
		return removeBoltLocks(path)
	}
	if err != nil {
		return err
	}
	defer src.Close()
	if same, err := isOpenFile(path, src); err != nil || !same {
		return err
	}

	// Start a fresh sibling file.
	le = le.WithField("path", path)
	le.Info("converting bolt volume to s4db")
	start := time.Now()
	tmp := path + ".converting"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dst, err := s4db.Open(tmp, s4db.Options{})
	if err != nil {
		return err
	}

	// Copy the keys into it and check them.
	bounds, err := splitBolt(src)
	if err == nil {
		var want digest
		want, err = copyBolt(ctx, src, dst, bounds)
		if err == nil {
			err = verifyCopy(ctx, dst, bounds, want)
		}
	}
	if err := errors.Join(err, dst.Close()); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}

	// Replace the bolt file and drop its lock files.
	if err := src.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return err
	}
	le.WithField("elapsed", time.Since(start)).Info("converted bolt volume to s4db")
	return removeBoltLocks(path)
}

// removeBoltLocks removes the lock files bbolt keeps beside path.
func removeBoltLocks(path string) error {
	for _, lock := range []string{path + "-lock", path + "-lock-coord"} {
		if err := os.Remove(lock); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// isBoltFile reports whether the file at path starts with a bbolt meta page.
func isBoltFile(path string) (bool, error) {
	// Open the file if it exists.
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Compare the magic number, treating a short file as another format.
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], boltMagicOff); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	return binary.LittleEndian.Uint32(magic[:]) == boltMagic, nil
}

// isOpenFile reports whether path still names the file db has open.
func isOpenFile(path string, db *bdb.DB) (bool, error) {
	err := db.ValidatePath()
	if errors.Is(err, bdb_errors.ErrLockFileChanged) {
		return false, nil
	}
	return err == nil, err
}

// splitBolt returns the bounds dividing the keys of the bolt Volume bucket
// into ranges of similar size. The first and last bounds are nil, meaning
// open.
func splitBolt(src *bdb.DB) ([][]byte, error) {
	bounds := [][]byte{nil, nil}
	err := src.View(func(btx *bdb.Tx) error {
		// An empty bolt file has no bucket and one empty range.
		bkt := btx.Bucket(boltBucket)
		if bkt == nil {
			return nil
		}

		// Halve every range that holds keys to split between.
		c := bkt.Cursor()
		for range convertSplits {
			next := [][]byte{nil}
			for i := 1; i < len(bounds); i++ {
				if mid := splitRange(c, bounds[i-1], bounds[i]); mid != nil {
					next = append(next, mid)
				}
				next = append(next, bounds[i])
			}
			bounds = next
		}
		return nil
	})
	return bounds, err
}

// splitRange returns a bound strictly after the first key from lo up to hi
// and at most its last key, interpolating the eight bytes after their shared
// prefix. Interpolating between the range's own keys skips prefixes every
// key shares. It returns nil when no such bound exists.
func splitRange(c *bdb.Cursor, lo, hi []byte) []byte {
	// Find the first and last keys of the range.
	first, _ := c.Seek(lo)
	if first == nil || !inRange(first, hi) {
		return nil
	}
	first = bytes.Clone(first)
	last, _ := c.Last()
	if hi != nil {
		if k, _ := c.Seek(hi); k != nil {
			last, _ = c.Prev()
		}
	}

	// Interpolate after the shared prefix.
	p := 0
	for p < len(first) && p < len(last) && first[p] == last[p] {
		p++
	}
	a, b := keyNumber(first[p:]), keyNumber(last[p:])
	mid := make([]byte, p+8)
	copy(mid, first[:p])
	binary.BigEndian.PutUint64(mid[p:], a+(b-a)/2)
	if bytes.Compare(mid, first) <= 0 || bytes.Compare(mid, last) > 0 {
		return nil
	}
	return mid
}

// keyNumber reads the first eight bytes of k as a big-endian number,
// padding a shorter k with zeros.
func keyNumber(k []byte) uint64 {
	var b [8]byte
	copy(b[:], k)
	return binary.BigEndian.Uint64(b[:])
}

// inRange reports whether k sorts before the open-ended bound hi.
func inRange(k, hi []byte) bool {
	return hi == nil || bytes.Compare(k, hi) < 0
}

// pair is one key and value copied out of the bolt file.
type pair struct {
	// key is the key.
	key []byte
	// val is the value.
	val []byte
}

// copyBolt copies every key of the bolt Volume bucket into dst, reading the
// ranges between bounds in parallel and committing once per convertBatch
// bytes. It returns the digest of the keys copied.
func copyBolt(ctx context.Context, src *bdb.DB, dst *s4db.DB, bounds [][]byte) (digest, error) {
	// Read each range in its own bolt transaction, convertReaders at once.
	eg, ectx := errgroup.WithContext(ctx)
	chunks := make(chan []pair, convertReaders)
	digests := make([]digest, len(bounds)-1)
	eg.Go(func() error {
		// Close chunks once every reader returns.
		defer close(chunks)
		var readers errgroup.Group
		readers.SetLimit(convertReaders)
		for i := range digests {
			readers.Go(func() error {
				return readBolt(ectx, src, bounds[i], bounds[i+1], &digests[i], chunks)
			})
		}
		return readers.Wait()
	})

	// Write the chunks as they arrive.
	eg.Go(func() error {
		return writeChunks(ectx, dst, chunks)
	})
	if err := eg.Wait(); err != nil {
		return digest{}, err
	}
	var sum digest
	for _, d := range digests {
		sum.merge(d)
	}
	return sum, nil
}

// readBolt sends the pairs of the bolt Volume bucket from lo up to hi to
// chunks, adding each to d.
func readBolt(ctx context.Context, src *bdb.DB, lo, hi []byte, d *digest, chunks chan<- []pair) error {
	return src.View(func(btx *bdb.Tx) error {
		// An empty bolt file has no bucket and nothing to copy.
		bkt := btx.Bucket(boltBucket)
		if bkt == nil {
			return nil
		}

		// Copy the pairs out of the mapped file, sending each full chunk.
		h := blake3.New()
		var chunk []pair
		var size int
		c := bkt.Cursor()
		for k, v := c.Seek(lo); k != nil && inRange(k, hi); k, v = c.Next() {
			buf := slices.Concat(k, v)
			p := pair{key: buf[:len(k)], val: buf[len(k):]}
			d.add(h, p.key, p.val)
			chunk = append(chunk, p)
			size += len(buf)
			if size < convertChunk {
				continue
			}
			if err := sendChunk(ctx, chunks, chunk); err != nil {
				return err
			}
			chunk, size = nil, 0
		}
		return sendChunk(ctx, chunks, chunk)
	})
}

// sendChunk sends a non-empty chunk unless ctx ends first.
func sendChunk(ctx context.Context, chunks chan<- []pair, chunk []pair) error {
	if len(chunk) == 0 {
		return nil
	}
	select {
	case chunks <- chunk:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// writeChunks sets the pairs of each chunk in dst, committing once per
// convertBatch bytes.
func writeChunks(ctx context.Context, dst *s4db.DB, chunks <-chan []pair) error {
	// Hold one write transaction per batch.
	var tx kvtx.Tx
	var size int
	for chunk := range chunks {
		// Start a transaction for the next batch.
		var err error
		if tx == nil {
			if tx, err = dst.NewTransaction(ctx, true); err != nil {
				return err
			}
		}

		// Set the pairs, committing a full batch.
		for _, p := range chunk {
			if err := tx.Set(ctx, p.key, p.val); err != nil {
				tx.Discard()
				return err
			}
			size += len(p.val)
		}
		if size < convertBatch {
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		tx, size = nil, 0
	}
	if tx == nil {
		return nil
	}
	return tx.Commit(ctx)
}

// verifyCopy checks that dst holds exactly the pairs digest want sums,
// reading the ranges between bounds in parallel.
func verifyCopy(ctx context.Context, dst *s4db.DB, bounds [][]byte, want digest) error {
	// Sum each range from one snapshot.
	digests := make([]digest, len(bounds)-1)
	var eg errgroup.Group
	eg.SetLimit(convertReaders)
	for i := range digests {
		eg.Go(func() error {
			return sumRange(ctx, dst, bounds[i], bounds[i+1], &digests[i])
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// Compare the total.
	var got digest
	for _, d := range digests {
		got.merge(d)
	}
	if got != want {
		return fmt.Errorf("converted volume holds %d keys that do not match the %d bolt keys", got.n, want.n)
	}
	return nil
}

// sumRange adds the pairs of dst from lo up to hi to d.
func sumRange(ctx context.Context, dst *s4db.DB, lo, hi []byte, d *digest) error {
	// Open a snapshot positioned at lo.
	tx, err := dst.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()
	if err := it.Seek(lo); err != nil {
		return err
	}

	// Add each pair before hi.
	h := blake3.New()
	for ; it.Valid() && inRange(it.Key(), hi); it.Next() {
		v, err := it.Value()
		if err != nil {
			return err
		}
		d.add(h, it.Key(), v)
	}
	return it.Err()
}

// digest is a checksum of a set of distinct keys and their values that
// does not depend on their order: the count and the XOR of each pair's hash.
type digest struct {
	// n is the number of pairs.
	n int64
	// x is the XOR of the pair hashes.
	x [32]byte
}

// add adds the pair k, v, hashing it with h.
func (d *digest) add(h *blake3.Hasher, k, v []byte) {
	// Hash the key length, key, and value.
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(k))) // #nosec G115 -- lengths are not negative.
	h.Reset()
	_, _ = h.Write(n[:])
	_, _ = h.Write(k)
	_, _ = h.Write(v)
	var sum [32]byte
	d.merge(digest{n: 1, x: [32]byte(h.Sum(sum[:0]))})
}

// merge adds the pairs of o.
func (d *digest) merge(o digest) {
	d.n += o.n
	for i := range d.x {
		d.x[i] ^= o.x[i]
	}
}

// syncDir makes a rename in dir durable. Windows cannot sync a directory and
// orders the rename itself.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
