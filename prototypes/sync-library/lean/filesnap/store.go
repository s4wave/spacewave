// Package filesnap provides a prototype file-backed KVTX store: the full
// key/value state persists to one JSON snapshot, rewritten atomically on
// every commit. It exists to prove durable world persistence in compiled
// JavaScript; production targets should use a real storage backend.
package volume_filesnap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	kvtx "github.com/s4wave/spacewave/db/kvtx"
)

// Store is a file-backed KVTX store persisting a JSON snapshot per commit.
// Values are stored base64-encoded because block bytes are binary.
type Store struct {
	mtx  sync.Mutex
	path string
	data map[string]string // key -> base64(value)
}

var _ kvtx.Store = (*Store)(nil)

// NewStore opens the snapshot file if present and returns the store.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]string{}}
	// Treat any read failure as a fresh store: the js fs bridge wraps
	// ENOENT in its own error type, so os.IsNotExist is unreliable there.
	data, err := os.ReadFile(path)
	if err != nil {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return nil, err
	}
	return s, nil
}

// Execute executes the given store.
func (s *Store) Execute(ctx context.Context) error { return nil }

// persist writes the snapshot atomically.
func (s *Store) persistLocked() error {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data, err := json.MarshalIndent(s.data, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// fileTx is one transaction over the snapshot store.
type fileTx struct {
	s     *Store
	write bool
	dirty map[string]*string
	done  bool
}

var _ kvtx.Tx = (*fileTx)(nil)

// Commit flushes buffered mutations into the parent map and persists.
func (t *fileTx) Commit(ctx context.Context) error {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	if t.done {
		return nil
	}
	t.done = true
	for key, val := range t.dirty {
		if val == nil {
			delete(t.s.data, key)
		} else {
			t.s.data[key] = base64.StdEncoding.EncodeToString([]byte(*val))
		}
	}
	return t.s.persistLocked()
}

// Discard drops buffered mutations.
func (t *fileTx) Discard() { t.done = true }

// Size returns the number of keys.
func (t *fileTx) Size(ctx context.Context) (uint64, error) { return uint64(len(t.s.data)), nil }

// Get returns the value at key.
func (t *fileTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	if v, ok := t.dirty[string(key)]; ok {
		if v == nil {
			return nil, false, nil
		}
		return []byte(*v), true, nil
	}
	enc, ok := t.s.data[string(key)]
	if !ok {
		return nil, false, nil
	}
	v, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// Exists checks if a key exists.
func (t *fileTx) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.Get(ctx, key)
	return found, err
}

// Set buffers a write.
func (t *fileTx) Set(ctx context.Context, key, value []byte) error {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	v := string(value)
	t.dirty[string(key)] = &v
	return nil
}

// Delete buffers a delete.
func (t *fileTx) Delete(ctx context.Context, key []byte) error {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	t.dirty[string(key)] = nil
	return nil
}

// ScanPrefix iterates keys with the prefix.
func (t *fileTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	for key, value := range t.dirty {
		if len(key) >= len(prefix) && key[:len(prefix)] == string(prefix) {
			if value == nil {
				continue
			}
			if err := cb([]byte(key), []byte(*value)); err != nil {
				return err
			}
		}
	}
	for key, value := range t.s.data {
		if _, dirty := t.dirty[key]; dirty {
			continue
		}
		if len(key) >= len(prefix) && key[:len(prefix)] == string(prefix) {
			if err := cb([]byte(key), []byte(value)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ScanPrefixKeys iterates keys only with the prefix.
func (t *fileTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, _ []byte) error { return cb(key) })
}

// NewTransaction returns a new transaction against the store.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return &fileTx{s: s, write: write, dirty: map[string]*string{}}, nil
}

// Iterate returns an iterator over keys with the prefix.
func (t *fileTx) Iterate(ctx context.Context, prefix []byte, sortKeys, reverse bool) kvtx.Iterator {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	seen := map[string]string{}
	for key, enc := range t.s.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == string(prefix) {
			seen[key] = enc
		}
	}
	for key, val := range t.dirty {
		if len(key) >= len(prefix) && key[:len(prefix)] != string(prefix) {
			continue
		}
		if val == nil {
			delete(seen, key)
		} else {
			seen[key] = base64.StdEncoding.EncodeToString([]byte(*val))
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if reverse {
		for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
			keys[i], keys[j] = keys[j], keys[i]
		}
	}
	return &sliceIterator{ctx: ctx, keys: keys, values: seen}
}

// sliceIterator iterates a fixed key sequence.
type sliceIterator struct {
	ctx    context.Context
	keys   []string
	values map[string]string
	pos    int
	err    error
}

func (i *sliceIterator) Err() error { return i.err }
func (i *sliceIterator) Valid() bool {
	return i.err == nil && i.pos < len(i.keys) && i.ctx.Err() == nil
}

func (i *sliceIterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return []byte(i.keys[i.pos])
}

func (i *sliceIterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(i.values[i.keys[i.pos]])
}

func (i *sliceIterator) ValueCopy(dst []byte) ([]byte, error) {
	v, err := i.Value()
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(v) {
		return append(dst[:0], v...), nil
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (i *sliceIterator) Next() bool {
	if i.pos < len(i.keys) {
		i.pos++
	}
	return i.Valid()
}

func (i *sliceIterator) Seek(k []byte) error {
	target := string(k)
	for pos, key := range i.keys {
		if key >= target {
			i.pos = pos
			return nil
		}
	}
	i.pos = len(i.keys)
	return nil
}
func (i *sliceIterator) Close() { i.err = context.Canceled }
