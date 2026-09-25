// Package paylog is a volume engine shell over a storage Device: block
// payloads append to a log of segment files, and one ordered index holds the
// key-value store, the block locations, and the garbage collection journal.
//
// Every index commit also publishes the locations of the blocks written since
// the previous one. The commit's flush covers the earlier segment writes, so a
// published block is durable, and a key-value commit that follows a block write
// publishes that block with it.
package paylog

import (
	"context"
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
	kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	"github.com/s4wave/spacewave/db/volume/device"
)

// indexName is the device file holding the index.
const indexName = "index"

// segmentPrefix starts the device file name of every payload segment.
const segmentPrefix = "seg-"

// segmentSize is the length at which a segment stops taking payloads.
const segmentSize = 64 << 20

// Index key prefixes.
const (
	// kvPrefix holds the key-value store.
	kvPrefix = "k/"
	// blockPrefix maps a block reference key to its payload location.
	blockPrefix = "b/"
	// journalPrefix holds garbage collection journal entries by sequence.
	journalPrefix = "j/"
)

// bucket is the bbolt bucket holding every index key.
var bucket = []byte("paylog")

// Store is a volume engine on a Device with a bbolt index.
type Store struct {
	// kvtx.Store serves the key-value store under kvPrefix.
	kvtx.Store

	// dev holds the index and the segments.
	dev device.Device
	// db is the index.
	db *bbolt.DB
	// index serves unprefixed index transactions whose commits publish
	// pending blocks.
	index *kvtx_bolt.Store

	// mtx guards the fields below. It is held across segment writes so a
	// pending location is never published before its payload write is issued.
	mtx sync.Mutex
	// segment is the number of the segment taking payloads.
	segment uint64
	// offset is the end of the payloads in the segment.
	offset int64
	// pending holds unpublished block writes and removes by block key.
	pending map[string]*pendingBlock
	// journal is the sequence of the last journal entry.
	journal uint64
}

// pendingBlock is one unpublished block write or remove.
type pendingBlock struct {
	// loc is the payload location; nil for a remove.
	loc *location
}

// Open opens the store on dev, creating it when dev is empty.
func Open(ctx context.Context, dev device.Device) (*Store, error) {
	// Open the index on its device file.
	h, err := device.OpenHandle(ctx, dev, indexName)
	if err != nil {
		return nil, err
	}
	db, err := bbolt.OpenStorage(h, &bbolt.Options{PageSize: 4096, NoGrowSync: true})
	if err != nil {
		return nil, errors.Wrap(err, "open index")
	}
	s := &Store{
		dev:     dev,
		db:      db,
		index:   kvtx_bolt.NewStore(db, bucket),
		pending: make(map[string]*pendingBlock),
	}
	s.Store = kvtx_prefixer.NewPrefixer(indexStore{s}, []byte(kvPrefix))

	// Find the last journal sequence, and create the bucket on a new index.
	var found bool
	err = db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucket)
		if found = b != nil; !found {
			return nil
		}
		c := b.Cursor()
		c.Seek([]byte(journalPrefix + "\xff"))
		if k, _ := c.Prev(); strings.HasPrefix(string(k), journalPrefix) {
			s.journal = binary.BigEndian.Uint64(k[len(journalPrefix):])
		}
		return nil
	})
	if err == nil && !found {
		err = db.Update(func(tx *bbolt.Tx) error {
			_, err := tx.CreateBucket(bucket)
			return err
		})
	}
	if err != nil {
		_ = db.Close()
		return nil, errors.Wrap(err, "init index")
	}

	// Start a new segment after every existing one, since an unpublished tail
	// of the last one may be torn.
	files, err := dev.List(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	for _, f := range files {
		if n, ok := parseSegment(f.Name); ok {
			s.segment = max(s.segment, n)
		}
	}
	s.segment++
	return s, nil
}

// Close closes the index. Unpublished block writes are lost.
func (s *Store) Close() error {
	return s.db.Close()
}

// update runs fn in an index write transaction that also publishes the
// pending blocks, and commits it.
func (s *Store) update(fn func(b *bbolt.Bucket) error) error {
	tx, err := s.db.Begin(true)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	b := tx.Bucket(bucket)
	if fn != nil {
		if err := fn(b); err != nil {
			return err
		}
	}
	return s.publish(tx, b)
}

// publish writes the pending block locations into tx, commits it, and drops
// the published entries from pending. Entries replaced during the commit stay.
func (s *Store) publish(tx *bbolt.Tx, b *bbolt.Bucket) error {
	// Snapshot pending after the payload writes it names were issued.
	s.mtx.Lock()
	published := maps.Clone(s.pending)
	s.mtx.Unlock()

	// Write the locations in key order and commit; the commit's flush covers
	// the payloads. bbolt splits a node only at commit, so unordered puts
	// would shift a growing leaf on every insert.
	for _, key := range slices.Sorted(maps.Keys(published)) {
		loc := published[key].loc
		if loc == nil {
			if err := b.Delete([]byte(key)); err != nil {
				return err
			}
			continue
		}
		if err := b.Put([]byte(key), loc.marshal()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Drop the entries the commit published.
	s.mtx.Lock()
	for key, p := range published {
		if s.pending[key] == p {
			delete(s.pending, key)
		}
	}
	s.mtx.Unlock()
	return nil
}

// indexStore serves index transactions. A write transaction collects its
// changes and applies them in one bbolt write transaction at commit, which
// also publishes the pending blocks, so any number of write transactions can
// be open at once.
type indexStore struct {
	s *Store
}

// NewTransaction opens an index transaction.
func (i indexStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	read, err := i.s.index.NewTransaction(ctx, false)
	if err != nil || !write {
		return read, err
	}
	w := &writeTx{s: i.s}
	w.Tx, err = kvtx_txcache.NewTxWithCbs(read, true, read.Discard, w.begin, false)
	if err != nil {
		read.Discard()
		return nil, err
	}
	return w, nil
}

// writeTx is an index write transaction.
type writeTx struct {
	// Tx collects the changes.
	*kvtx_txcache.Tx
	// s is the store.
	s *Store
	// btx is the bbolt write transaction opened at commit.
	btx *bbolt.Tx
}

// begin opens the bbolt write transaction the collected changes apply to.
func (w *writeTx) begin() (kvtx.Tx, error) {
	btx, err := w.s.db.Begin(true)
	if err != nil {
		return nil, err
	}
	w.btx = btx
	return kvtx_bolt.NewTx(btx, bucket), nil
}

// Commit applies the collected changes, publishes the pending blocks, and
// commits.
func (w *writeTx) Commit(ctx context.Context) error {
	err := w.Tx.Commit(ctx)
	if w.btx == nil {
		return err
	}
	defer func() { _ = w.btx.Rollback() }()
	if err != nil {
		return err
	}
	return w.s.publish(w.btx, w.btx.Bucket(bucket))
}

// location is where a block payload lives.
type location struct {
	// segment is the segment number.
	segment uint64
	// offset is the payload's byte offset in the segment.
	offset int64
	// length is the payload length.
	length int64
}

// marshal encodes the location as three unsigned varints. Offsets and
// lengths are never negative.
func (l *location) marshal() []byte {
	b := binary.AppendUvarint(nil, l.segment)
	b = binary.AppendUvarint(b, uint64(l.offset))    //nolint:gosec
	return binary.AppendUvarint(b, uint64(l.length)) //nolint:gosec
}

// unmarshalLocation decodes a location.
func unmarshalLocation(b []byte) (*location, error) {
	var v [3]uint64
	for i := range v {
		n, size := binary.Uvarint(b)
		if size <= 0 {
			return nil, errors.New("invalid block location")
		}
		v[i], b = n, b[size:]
	}
	if v[1] > math.MaxInt64-v[2] {
		return nil, errors.New("invalid block location")
	}
	return &location{segment: v[0], offset: int64(v[1]), length: int64(v[2])}, nil //nolint:gosec
}

// segmentName returns the device file name of segment n.
func segmentName(n uint64) string {
	return segmentPrefix + strconv.FormatUint(n, 10)
}

// parseSegment returns the number of a segment file name.
func parseSegment(name string) (uint64, bool) {
	rest, ok := strings.CutPrefix(name, segmentPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(rest, 10, 64)
	return n, err == nil
}
