// Package paylog is a volume engine shell over a storage Device: block
// payloads append to a log of segment files, and one ordered index holds the
// key-value store, the block locations, and the garbage collection journal.
//
// Every durable index commit also publishes the locations of the blocks
// written since the previous one. The commit's flush covers the earlier segment
// writes, so a published block is durable, and a key-value commit that follows
// a block write publishes that block with it. An ordered commit publishes no
// blocks, since a crash could keep its record and lose their payloads.
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

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	kvtx_txcache "github.com/s4wave/spacewave/db/kvtx/txcache"
	"github.com/s4wave/spacewave/db/volume/device"
)

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

// Index is the ordered key-value index a Store keeps on its device. Read
// transactions see a snapshot and may stay open across commits. A write
// transaction excludes other writers, and its Commit applies atomically and
// makes every earlier write on the device durable no later than the commit
// itself, so a crash never keeps a commit without them. A write transaction
// may also implement kvtx.OrderedCommitTx, whose commits a device flush makes
// durable. The index keeps its own files on the device, none named with
// segmentPrefix.
type Index interface {
	kvtx.Store

	// Close releases the index. Uncommitted writes are lost.
	Close() error
}

// Store is a volume engine on a Device with an ordered index.
type Store struct {
	// kvtx.Store serves the key-value store under kvPrefix.
	kvtx.Store

	// dev holds the index and the segments.
	dev device.Device
	// index holds the key-value store, the block locations, and the journal.
	index Index

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

// Open opens the store on dev with index, an index opened on the same device.
// The store owns the index and closes it on Close or on failure.
func Open(ctx context.Context, dev device.Device, index Index) (*Store, error) {
	s := &Store{
		dev:     dev,
		index:   index,
		pending: make(map[string]*pendingBlock),
	}
	s.Store = kvtx_prefixer.NewPrefixer(indexStore{s}, []byte(kvPrefix))

	// Find the last journal sequence.
	err := s.view(ctx, func(tx kvtx.Tx) error {
		it := tx.Iterate(ctx, []byte(journalPrefix), true, true)
		defer it.Close()
		if it.Next() {
			s.journal = binary.BigEndian.Uint64(it.Key()[len(journalPrefix):])
		}
		return it.Err()
	})
	if err != nil {
		_ = index.Close()
		return nil, errors.Wrap(err, "read journal")
	}

	// Start a new segment after every existing one, since an unpublished tail
	// of the last one may be torn.
	files, err := dev.List(ctx)
	if err != nil {
		_ = index.Close()
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
	return s.index.Close()
}

// view runs fn in an index read transaction.
func (s *Store) view(ctx context.Context, fn func(tx kvtx.Tx) error) error {
	tx, err := s.index.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(tx)
}

// update runs fn in an index write transaction and commits it, durably with
// the pending blocks unless ordered is set.
func (s *Store) update(ctx context.Context, ordered bool, fn func(tx kvtx.Tx) error) error {
	tx, err := s.index.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if fn != nil {
		if err := fn(tx); err != nil {
			return err
		}
	}
	return s.commit(ctx, tx, ordered)
}

// commit commits the index write transaction tx with write ordering if
// ordered is set, and otherwise publishes the pending blocks with it.
func (s *Store) commit(ctx context.Context, tx kvtx.Tx, ordered bool) error {
	if ordered {
		return kvtx.CommitOrdered(ctx, tx)
	}
	return s.publish(ctx, tx)
}

// publish writes the pending block locations into tx, commits it, and drops
// the published entries from pending. Entries replaced during the commit stay.
func (s *Store) publish(ctx context.Context, tx kvtx.Tx) error {
	// Snapshot pending after the payload writes it names were issued.
	s.mtx.Lock()
	published := maps.Clone(s.pending)
	s.mtx.Unlock()

	// Write the locations in key order and commit; the commit's flush covers
	// the payloads. A B+tree index splits nodes only at commit, so unordered
	// puts would shift a growing leaf on every insert.
	for _, key := range slices.Sorted(maps.Keys(published)) {
		loc := published[key].loc
		if loc == nil {
			if err := tx.Delete(ctx, []byte(key)); err != nil {
				return err
			}
			continue
		}
		if err := tx.Set(ctx, []byte(key), loc.marshal()); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
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
// changes and applies them in one index write transaction at commit, which
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
	w := &writeTx{s: i.s, ctx: ctx}
	w.Tx, err = kvtx_txcache.NewTxWithCbs(read, true, read.Discard, w.begin, false)
	if err != nil {
		read.Discard()
		return nil, err
	}
	return w, nil
}

// writeTx is a buffered index write transaction.
type writeTx struct {
	// Tx collects the changes.
	*kvtx_txcache.Tx
	// s is the store.
	s *Store
	// ctx is the context the transaction was opened with.
	ctx context.Context
	// itx is the index write transaction opened at commit.
	itx kvtx.Tx
}

// begin opens the index write transaction the collected changes apply to.
func (w *writeTx) begin() (kvtx.Tx, error) {
	itx, err := w.s.index.NewTransaction(w.ctx, true)
	if err != nil {
		return nil, err
	}
	w.itx = itx
	return itx, nil
}

// Commit applies the collected changes, publishes the pending blocks, and
// commits.
func (w *writeTx) Commit(ctx context.Context) error {
	return w.commit(ctx, false)
}

// CommitOrdered applies the collected changes and commits them with write
// ordering, leaving the pending blocks to the next Sync or durable commit.
func (w *writeTx) CommitOrdered(ctx context.Context) error {
	return w.commit(ctx, true)
}

// commit applies the collected changes and commits the index transaction.
func (w *writeTx) commit(ctx context.Context, ordered bool) error {
	err := w.Tx.Commit(ctx)
	if w.itx == nil {
		return err
	}
	defer w.itx.Discard()
	if err != nil {
		return err
	}
	return w.s.commit(ctx, w.itx, ordered)
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*writeTx)(nil)

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
