//go:build !tinygo

// Package block_store_s3_overlay lays a local record over an object store it
// only reads, so a pack sweep rehearses its full cycle on a production store's
// real packs without changing them.
//
// Each key may have one record in the overlay's directory, at the key's path:
// local bytes a put wrote, an alias naming the base key a move took the object
// from, or a marker hiding the key. A key without a record reads from the base.
// Each record is replaced whole by a rename, so every step leaves each key
// readable as one object or as none, and processes sharing the directory, such
// as a backend and the sweep's command, each restarted at any point, see one
// view. Nothing is ever written to the base.
package block_store_s3_overlay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// Record kinds, the first byte of a record file.
const (
	// kindData is local bytes, which follow.
	kindData = 'D'
	// kindAlias is a moved object, read from the base key that follows.
	kindAlias = 'A'
	// kindHidden hides the key.
	kindHidden = 'H'
)

// tempPrefix starts the names of records being written, which no key has.
const tempPrefix = ".overlay-tmp-"

// Store is an ObjectMover over one bucket of a base object store, holding its
// changes in a local directory.
type Store struct {
	// base is the store read for keys without a record.
	base block_store_s3.ObjectStore
	// bucket is the one bucket served.
	bucket string
	// dir holds the records.
	dir string
}

// New serves bucket of base with the records in dir, which it creates. Records
// already in dir apply, so a restarted process sees the view it left.
func New(base block_store_s3.ObjectStore, bucket, dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{base: base, bucket: bucket, dir: dir}, nil
}

// Conns returns the base's connections, when it reports them, so a pack store
// over the overlay reads with the base's width.
func (s *Store) Conns() int {
	if w, ok := s.base.(interface{ Conns() int }); ok {
		return w.Conns()
	}
	return 1
}

// view is how a key reads: local bytes, the base key it reads from, or hidden.
type view struct {
	// file is the open record of local bytes, positioned after its kind.
	file *os.File
	// target is the base key read when file is nil and the key is not hidden.
	target string
	// hidden is set when the key reads as missing.
	hidden bool
}

// path returns the record path of key, refusing another bucket and keys that
// are not clean relative paths.
func (s *Store) path(bucket, key string) (string, error) {
	if bucket != s.bucket {
		return "", errors.Errorf("the overlay serves bucket %q, not %q", s.bucket, bucket)
	}
	if !fs.ValidPath(key) || key == "." || strings.HasPrefix(path.Base(key), tempPrefix) {
		return "", errors.Errorf("the overlay cannot hold key %q", key)
	}
	return filepath.Join(s.dir, filepath.FromSlash(key)), nil
}

// lookup reads how key reads. The caller closes the view's file.
func (s *Store) lookup(name, key string) (view, error) {
	// Open the record; a key without one reads from the base.
	f, err := os.Open(name)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return view{target: key}, nil
	}
	if err != nil {
		return view{}, err
	}
	var kind [1]byte
	if _, err := io.ReadFull(f, kind[:]); err != nil {
		_ = f.Close()
		return view{}, errors.Wrap(err, "read the overlay record of "+key)
	}

	// Keep a data record open, and read an alias's target.
	switch kind[0] {
	case kindData:
		return view{file: f}, nil
	case kindAlias:
		target, err := io.ReadAll(f)
		_ = f.Close()
		return view{target: string(target)}, err
	case kindHidden:
		_ = f.Close()
		return view{hidden: true}, nil
	}
	_ = f.Close()
	return view{}, errors.Errorf("the overlay record of %s has kind %q", key, kind[0])
}

// notFound is the error of a missing key.
func notFound(key string) error {
	return errors.Wrap(block_store_s3.ErrNotFound, key)
}

// PutObject writes the object's bytes to its record.
func (s *Store) PutObject(_ context.Context, bucket, key string, data []byte, _ string) error {
	name, err := s.path(bucket, key)
	if err != nil {
		return err
	}
	return writeRecord(name, kindData, data)
}

// GetObject opens the object's body.
func (s *Store) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	// Read the key's view.
	name, err := s.path(bucket, key)
	if err != nil {
		return nil, err
	}
	v, err := s.lookup(name, key)
	if err != nil {
		return nil, err
	}

	// Read the local bytes, or the base key.
	switch {
	case v.file != nil:
		return v.file, nil
	case v.hidden:
		return nil, notFound(key)
	}
	return s.base.GetObject(ctx, bucket, v.target)
}

// GetObjectRange reads length bytes from offset off.
func (s *Store) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Read the key's view.
	name, err := s.path(bucket, key)
	if err != nil {
		return nil, err
	}
	v, err := s.lookup(name, key)
	if err != nil {
		return nil, err
	}

	// Read the base key, or say the key is missing.
	if v.file == nil {
		if v.hidden {
			return nil, notFound(key)
		}
		return s.base.GetObjectRange(ctx, bucket, v.target, off, length)
	}

	// Read the range from the local bytes, past the record's kind.
	defer v.file.Close()
	buf := make([]byte, length)
	n, err := v.file.ReadAt(buf, off+1)
	if err == io.EOF {
		err = nil
	}
	if n == 0 {
		return nil, err
	}
	return buf[:n], err
}

// DeleteObject hides the object. A missing object counts as deleted.
func (s *Store) DeleteObject(_ context.Context, bucket, key string) error {
	name, err := s.path(bucket, key)
	if err != nil {
		return err
	}
	return writeRecord(name, kindHidden, nil)
}

// MoveObject moves the object at from to to: local bytes by a link to the new
// record, an object of the base by an alias to it. The record at to is written
// before the one hiding from, so a killed move leaves the object at both keys
// and a repeat finishes it. A move whose source is gone counts as done when
// the destination reads; with neither, it returns ErrNotFound.
func (s *Store) MoveObject(ctx context.Context, bucket, from, to string) error {
	// Read the source's view.
	src, err := s.path(bucket, from)
	if err != nil {
		return err
	}
	dst, err := s.path(bucket, to)
	if err != nil {
		return err
	}
	v, err := s.lookup(src, from)
	if err != nil {
		return err
	}

	// Link local bytes to the destination's record.
	if v.file != nil {
		_ = v.file.Close()
		if err := linkRecord(src, dst); err != nil {
			return errors.Wrap(err, "move "+from)
		}
		return writeRecord(src, kindHidden, nil)
	}

	// A source gone counts as moved when the destination reads.
	found := !v.hidden
	if found {
		if found, err = s.baseHas(ctx, v.target); err != nil {
			return err
		}
	}
	if !found {
		return s.moved(ctx, bucket, from, to, dst)
	}

	// Point the destination at the base key, then hide the source.
	if v.target == to {
		err = os.Remove(dst)
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	} else {
		err = writeRecord(dst, kindAlias, []byte(v.target))
	}
	if err != nil {
		return errors.Wrap(err, "move "+from)
	}
	return writeRecord(src, kindHidden, nil)
}

// moved answers a move whose source is gone: done when the destination reads,
// else not found.
func (s *Store) moved(ctx context.Context, bucket, from, to, dst string) error {
	// Read the destination's view.
	v, err := s.lookup(dst, to)
	if err != nil {
		return err
	}
	found := v.file != nil
	if found {
		_ = v.file.Close()
	}

	// Check the base for an alias or a key without a record.
	if !found && !v.hidden {
		if found, err = s.baseHas(ctx, v.target); err != nil {
			return err
		}
	}
	if !found {
		return errors.Wrap(block_store_s3.ErrNotFound, "move "+from)
	}
	return nil
}

// baseHas reports whether the base holds key.
func (s *Store) baseHas(ctx context.Context, key string) (bool, error) {
	_, err := s.base.GetObjectRange(ctx, s.bucket, key, 0, 1)
	if errors.Is(err, block_store_s3.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// listed is a record a listing found.
type listed struct {
	// kind is the record's kind.
	kind byte
	// size is the size of local bytes.
	size int64
	// target is an alias's base key.
	target string
}

// ListObjects calls fn with the key and size of every object whose key starts
// with prefix: the base's keys without a record, in the base's order, then the
// local bytes and the aliases, in key order. An alias's size is the size of its
// base key, read from the base's listing of that key's directory.
func (s *Store) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	// Read the records under the prefix, and the base keys the aliases read.
	if bucket != s.bucket {
		return errors.Errorf("the overlay serves bucket %q, not %q", s.bucket, bucket)
	}
	records, err := s.readRecords(prefix)
	if err != nil {
		return err
	}
	sizes := make(map[string]int64)
	for _, r := range records {
		if r.kind == kindAlias {
			sizes[r.target] = -1
		}
	}

	// List the base, passing each key without a record and noting the sizes of
	// the aliases' targets.
	err = s.base.ListObjects(ctx, bucket, prefix, func(key string, size int64) error {
		if _, ok := sizes[key]; ok {
			sizes[key] = size
		}
		if _, ok := records[key]; ok {
			return nil
		}
		return fn(key, size)
	})
	if err != nil {
		return err
	}

	// Pass the local bytes and the aliases whose targets the base holds.
	if err := s.listTargets(ctx, sizes); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(records)) {
		if err := passRecord(key, records[key], sizes, fn); err != nil {
			return err
		}
	}
	return nil
}

// passRecord passes a listed record's object to fn: local bytes at their size,
// an alias at its target's size when the base holds it, and nothing for a key
// hidden.
func passRecord(key string, r listed, sizes map[string]int64, fn func(key string, size int64) error) error {
	switch {
	case r.kind == kindData:
		return fn(key, r.size)
	case r.kind == kindAlias && sizes[r.target] >= 0:
		return fn(key, sizes[r.target])
	}
	return nil
}

// listTargets fills the sizes of the aliases' targets the prefix's listing did
// not reach, with one base listing of each of their directories. A target the
// base lacks keeps size -1.
func (s *Store) listTargets(ctx context.Context, sizes map[string]int64) error {
	// Find the directories of the targets still unsized.
	var dirs []string
	for key, size := range sizes {
		if dir, _ := path.Split(key); size < 0 && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}

	// List each, noting the targets' sizes.
	for _, dir := range dirs {
		err := s.base.ListObjects(ctx, s.bucket, dir, func(key string, size int64) error {
			if old, ok := sizes[key]; ok && old < 0 {
				sizes[key] = size
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// readRecords reads the records whose keys start with prefix, walking the
// directories that may hold some.
func (s *Store) readRecords(prefix string) (map[string]listed, error) {
	// Walk from the prefix's directory.
	records := make(map[string]listed)
	dir, _ := path.Split(prefix)
	root := filepath.Join(s.dir, filepath.FromSlash(dir))
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		// Skip a missing root, and the directories no key under the prefix
		// passes through.
		if err != nil {
			if name == root && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(s.dir, name)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if d.IsDir() {
			return skipDir(name == root, key+"/", prefix)
		}

		// Read each record under the prefix.
		if !strings.HasPrefix(key, prefix) || strings.HasPrefix(d.Name(), tempPrefix) {
			return nil
		}
		r, err := readListed(name)
		if err == nil {
			records[key] = r
		}
		return err
	})
	return records, err
}

// skipDir returns fs.SkipDir for a directory, other than the walk's root,
// that no key starting with prefix passes through.
func skipDir(root bool, sub, prefix string) error {
	if root || strings.HasPrefix(sub, prefix) || strings.HasPrefix(prefix, sub) {
		return nil
	}
	return fs.SkipDir
}

// readListed reads the kind of the record at name, its size if local bytes and
// its target if an alias.
func readListed(name string) (listed, error) {
	// Read the kind.
	f, err := os.Open(name)
	if err != nil {
		return listed{}, err
	}
	defer f.Close()
	var kind [1]byte
	if _, err := io.ReadFull(f, kind[:]); err != nil {
		return listed{}, errors.Wrap(err, "read the overlay record "+name)
	}

	// Read the size of local bytes, or an alias's target.
	r := listed{kind: kind[0]}
	switch r.kind {
	case kindData:
		info, err := f.Stat()
		if err != nil {
			return listed{}, err
		}
		r.size = info.Size() - 1
	case kindAlias:
		target, err := io.ReadAll(f)
		if err != nil {
			return listed{}, err
		}
		r.target = string(target)
	}
	return r, nil
}

// writeRecord writes the record at name, its kind then payload, under a
// temporary name it syncs and renames into place.
func writeRecord(name string, kind byte, payload []byte) error {
	// Create the temporary file beside the record.
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return err
	}
	tmp := f.Name()

	// Write and sync it, then rename it over any record there.
	_, err = f.Write([]byte{kind})
	if err == nil {
		_, err = f.Write(payload)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, name)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// linkRecord links the record at src to dst, over any record there, through a
// temporary name beside dst.
func linkRecord(src, dst string) error {
	// Link to a temporary name.
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tag := make([]byte, 8)
	_, _ = rand.Read(tag)
	tmp := filepath.Join(dir, tempPrefix+hex.EncodeToString(tag))
	if err := os.Link(src, tmp); err != nil {
		return err
	}

	// Rename it into place.
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// _ is a type assertion
var _ block_store_s3.ObjectMover = (*Store)(nil)
