package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// Record kinds. Each kind byte begins a key under the Volume's liveness
// prefix. The child list keeps the kind and encoding of store format 6, so a
// format 6 Volume carries its lists to format 7 in place; every other kind is
// new in format 7 and upper case, so no record of an older graph is read as
// one of them.
const (
	// kindChildren keys the child list of a block by the block. Its value
	// packs the block keys of its children, sorted and distinct. The record
	// exists from the put that stored the block until the collector frees it,
	// so its presence is what "stored" means here.
	kindChildren byte = 'c'
	// kindCount keys the count record of a block by the block. Its value is a
	// uvarint, the block's holders minus one, kept only above zero.
	kindCount byte = 'H'
	// kindRoot keys a named root by its bucket and name. Its value is a
	// RootEntry.
	kindRoot byte = 'R'
	// kindJournal keys a journal record by its big-endian sequence number. Its
	// value is a JournalRecord.
	kindJournal byte = 'J'
	// kindQueue keys a free queue entry by the block. It holds no value.
	kindQueue byte = 'Q'
	// kindProof keys the completion proof of a block by the block: the graph
	// under it is stored. It holds no value and goes with the block.
	kindProof byte = 'P'
	// kindState is the one LivenessState record.
	kindState byte = 'S'
)

// The kinds a copy of a Volume's liveness records reads.
const (
	// ChildrenRecordKind begins the key of a child list.
	ChildrenRecordKind = kindChildren
	// RootRecordKind begins the key of a named root.
	RootRecordKind = kindRoot
	// ProofRecordKind begins the key of a completion proof.
	ProofRecordKind = kindProof
)

// BlockKey returns the key encoding of ref: its hash type byte, never zero,
// and its digest, whose length the type fixes. An empty ref has none.
func BlockKey(ref *block.BlockRef) []byte {
	// An empty ref has no key.
	if ref.GetEmpty() {
		return nil
	}

	// Append the hash type byte and the digest.
	h := ref.GetHash()
	key := make([]byte, 0, 1+len(h.GetHash()))
	key = append(key, byte(h.GetHashType())) //nolint:gosec // hash types fit in a byte.
	return append(key, h.GetHash()...)
}

// ParseBlockKey returns the ref a BlockKey encodes and the bytes after it.
func ParseBlockKey(key []byte) (*block.BlockRef, []byte, error) {
	// Read a known hash type followed by a digest of its length.
	if len(key) == 0 {
		return nil, nil, errBadBlockKey
	}
	t := hash.HashType(key[0])
	size := t.GetHashLen()
	if size <= 0 || len(key)-1 < size {
		return nil, nil, errBadBlockKey
	}
	digest := bytes.Clone(key[1 : 1+size])
	return block.NewBlockRef(&hash.Hash{HashType: t, Hash: digest}), key[1+size:], nil
}

// errBadBlockKey reports a record naming a block in an unknown encoding.
var errBadBlockKey = errors.New("liveness record names a block in an unknown encoding")

// Records reads and writes the liveness records of one Volume transaction:
// the child lists, count records and named roots that decide which blocks
// are live, and the journal, free queue, proofs and state the collector
// keeps. Every key begins with prefix. Opening Records reads nothing.
type Records struct {
	// tx is the Volume transaction.
	tx kvtx.TxOps
	// prefix begins every liveness key.
	prefix []byte
}

// NewRecords returns the liveness records under prefix in tx.
func NewRecords(tx kvtx.TxOps, prefix []byte) *Records {
	return &Records{tx: tx, prefix: prefix}
}

// key returns prefix, kind and the parts concatenated.
func (r *Records) key(kind byte, parts ...[]byte) []byte {
	// Size the key.
	size := len(r.prefix) + 1
	for _, p := range parts {
		size += len(p)
	}

	// Append the prefix, the kind and the parts.
	key := make([]byte, 0, size)
	key = append(key, r.prefix...)
	key = append(key, kind)
	for _, p := range parts {
		key = append(key, p...)
	}
	return key
}

// GetChildren returns the children in the child list of ref and whether the
// list exists, which is whether the Volume stores ref.
func (r *Records) GetChildren(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, bool, error) {
	value, found, err := r.tx.Get(ctx, r.key(kindChildren, BlockKey(ref)))
	if err != nil || !found {
		return nil, found, err
	}
	children, err := DecodeChildren(value)
	return children, true, err
}

// HasChildren reports whether the child list of ref exists.
func (r *Records) HasChildren(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindChildren, BlockKey(ref)))
}

// PutChildren writes the child list of ref.
func (r *Records) PutChildren(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindChildren, BlockKey(ref)), EncodeChildren(children))
}

// DeleteChildren deletes the child list of ref, which frees it in the liveness
// sense.
func (r *Records) DeleteChildren(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Delete(ctx, r.key(kindChildren, BlockKey(ref)))
}

// ScanChildren calls fn with each child list in key order.
func (r *Records) ScanChildren(ctx context.Context, fn func(ref *block.BlockRef, children []*block.BlockRef) error) error {
	prefix := r.key(kindChildren)
	return r.tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Decode the block and its list.
		ref, rest, err := ParseBlockKey(key[len(prefix):])
		if err != nil || len(rest) != 0 {
			return errBadBlockKey
		}
		children, err := DecodeChildren(value)
		if err != nil {
			return err
		}
		return fn(ref, children)
	})
}

// EncodeChildren packs the block keys of children, sorted and distinct, as a
// child list value.
func EncodeChildren(children []*block.BlockRef) []byte {
	// Sort the children's keys.
	keys := make([][]byte, 0, len(children))
	for _, child := range children {
		if k := BlockKey(child); k != nil {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, bytes.Compare)

	// Pack each distinct key.
	value := make([]byte, 0, len(keys)*33)
	for i, k := range keys {
		if i != 0 && bytes.Equal(keys[i-1], k) {
			continue
		}
		value = append(value, k...)
	}
	return value
}

// DecodeChildren decodes a child list value.
func DecodeChildren(value []byte) ([]*block.BlockRef, error) {
	var children []*block.BlockRef
	for len(value) != 0 {
		child, rest, err := ParseBlockKey(value)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
		value = rest
	}
	return children, nil
}

// GetCount returns the count record of ref: its holders minus one, zero when
// it has no record.
func (r *Records) GetCount(ctx context.Context, ref *block.BlockRef) (uint64, error) {
	// Read the record.
	value, found, err := r.tx.Get(ctx, r.key(kindCount, BlockKey(ref)))
	if err != nil || !found {
		return 0, err
	}

	// Decode the count.
	n, size := binary.Uvarint(value)
	if size <= 0 {
		return 0, errors.New("liveness count record is malformed")
	}
	return n, nil
}

// SetCount writes the count record of ref, deleting it at zero.
func (r *Records) SetCount(ctx context.Context, ref *block.BlockRef, n uint64) error {
	key := r.key(kindCount, BlockKey(ref))
	if n == 0 {
		return r.tx.Delete(ctx, key)
	}
	return r.tx.Set(ctx, key, binary.AppendUvarint(nil, n))
}

// rootKey returns the key of the root named name in bucketID.
func (r *Records) rootKey(bucketID, name string) []byte {
	return r.key(kindRoot, binary.AppendUvarint(nil, uint64(len(bucketID))), []byte(bucketID), []byte(name))
}

// GetRoot returns the entry of the root named name in bucketID, or nil when
// the name is free.
func (r *Records) GetRoot(ctx context.Context, bucketID, name string) (*RootEntry, error) {
	value, found, err := r.tx.Get(ctx, r.rootKey(bucketID, name))
	if err != nil || !found {
		return nil, err
	}
	entry := &RootEntry{}
	return entry, entry.UnmarshalVT(value)
}

// SetRoot writes the entry of the root named name in bucketID.
func (r *Records) SetRoot(ctx context.Context, bucketID, name string, entry *RootEntry) error {
	value, err := entry.MarshalVT()
	if err != nil {
		return err
	}
	return r.tx.Set(ctx, r.rootKey(bucketID, name), value)
}

// DeleteRoot frees the root named name in bucketID.
func (r *Records) DeleteRoot(ctx context.Context, bucketID, name string) error {
	return r.tx.Delete(ctx, r.rootKey(bucketID, name))
}

// ListRoots calls fn with each root of bucketID whose name starts with
// prefix, in name order.
func (r *Records) ListRoots(ctx context.Context, bucketID, prefix string, fn func(name string, entry *RootEntry) error) error {
	// Iterate the bucket's roots under the name prefix in key order.
	bucket := r.key(kindRoot, binary.AppendUvarint(nil, uint64(len(bucketID))), []byte(bucketID))
	it := r.tx.Iterate(ctx, append(bytes.Clone(bucket), prefix...), true, false)
	defer it.Close()

	// Decode each entry and call fn.
	for it.Next(); it.Valid(); it.Next() {
		value, err := it.Value()
		if err != nil {
			return err
		}
		entry := &RootEntry{}
		if err := entry.UnmarshalVT(value); err != nil {
			return err
		}
		if err := fn(string(it.Key()[len(bucket):]), entry); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanRoots calls fn with every named root of every bucket.
func (r *Records) ScanRoots(ctx context.Context, fn func(bucketID, name string, entry *RootEntry) error) error {
	prefix := r.key(kindRoot)
	return r.tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Split the bucket from the name.
		rest := key[len(prefix):]
		n, size := binary.Uvarint(rest)
		if size <= 0 || uint64(len(rest)-size) < n { //nolint:gosec // size > 0 and at most len(rest).
			return errors.New("liveness root record is malformed")
		}
		bucketID, name := string(rest[size:size+int(n)]), string(rest[size+int(n):]) //nolint:gosec // n fits the key.

		// Decode the entry.
		entry := &RootEntry{}
		if err := entry.UnmarshalVT(value); err != nil {
			return err
		}
		return fn(bucketID, name, entry)
	})
}

// journalKey returns the key of the journal record seq.
func (r *Records) journalKey(seq uint64) []byte {
	return r.key(kindJournal, binary.BigEndian.AppendUint64(nil, seq))
}

// PutJournal writes the journal record seq.
func (r *Records) PutJournal(ctx context.Context, seq uint64, rec *JournalRecord) error {
	value, err := rec.MarshalVT()
	if err != nil {
		return err
	}
	return r.tx.Set(ctx, r.journalKey(seq), value)
}

// DeleteJournal deletes the journal record seq.
func (r *Records) DeleteJournal(ctx context.Context, seq uint64) error {
	return r.tx.Delete(ctx, r.journalKey(seq))
}

// ScanJournal calls fn with each journal record from seq from on, in order,
// until fn returns false or an error.
func (r *Records) ScanJournal(ctx context.Context, from uint64, fn func(seq uint64, rec *JournalRecord) (bool, error)) error {
	// Position the sorted iterator at from.
	prefix := r.key(kindJournal)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(r.journalKey(from)); err != nil {
		return err
	}

	// Decode records until fn stops.
	for ; it.Valid(); it.Next() {
		key := it.Key()
		if len(key) != len(prefix)+8 {
			return errors.New("liveness journal key is malformed")
		}
		value, err := it.Value()
		if err != nil {
			return err
		}
		rec := &JournalRecord{}
		if err := rec.UnmarshalVT(value); err != nil {
			return err
		}
		more, err := fn(binary.BigEndian.Uint64(key[len(prefix):]), rec)
		if err != nil || !more {
			return err
		}
	}
	return it.Err()
}

// Queued reports whether ref is on the free queue.
func (r *Records) Queued(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindQueue, BlockKey(ref)))
}

// Enqueue puts ref on the free queue.
func (r *Records) Enqueue(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindQueue, BlockKey(ref)), nil)
}

// Dequeue takes ref off the free queue.
func (r *Records) Dequeue(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Delete(ctx, r.key(kindQueue, BlockKey(ref)))
}

// ScanQueue calls fn with each queued block in key order, after the block
// key after, until fn returns false or an error.
func (r *Records) ScanQueue(ctx context.Context, after []byte, fn func(ref *block.BlockRef) (bool, error)) error {
	// Position the sorted iterator past after.
	prefix := r.key(kindQueue)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(append(r.key(kindQueue, after), 0)); err != nil {
		return err
	}

	// Decode entries until fn stops.
	for ; it.Valid(); it.Next() {
		ref, rest, err := ParseBlockKey(it.Key()[len(prefix):])
		if err != nil || len(rest) != 0 {
			return errBadBlockKey
		}
		more, err := fn(ref)
		if err != nil || !more {
			return err
		}
	}
	return it.Err()
}

// HasProof reports whether ref has a completion proof.
func (r *Records) HasProof(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindProof, BlockKey(ref)))
}

// SetProof records the completion proof of ref.
func (r *Records) SetProof(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindProof, BlockKey(ref)), nil)
}

// DeleteProof drops the completion proof of ref.
func (r *Records) DeleteProof(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Delete(ctx, r.key(kindProof, BlockKey(ref)))
}

// GetState returns the state record, or the zero state when it is missing.
func (r *Records) GetState(ctx context.Context) (*LivenessState, error) {
	state := &LivenessState{}
	value, found, err := r.tx.Get(ctx, r.key(kindState))
	if err != nil || !found {
		return state, err
	}
	return state, state.UnmarshalVT(value)
}

// SetState writes the state record.
func (r *Records) SetState(ctx context.Context, state *LivenessState) error {
	value, err := state.MarshalVT()
	if err != nil {
		return err
	}
	return r.tx.Set(ctx, r.key(kindState), value)
}
