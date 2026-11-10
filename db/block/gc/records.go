package block_gc

import (
	"bytes"
	"cmp"
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
	// kindCount keys the count record of a block by the block. A block with
	// no record has one holder, and a uvarint value n above zero gives it
	// n+1. A stored block at zero holders keeps a zero record instead: a zero
	// byte, then a uvarint of its ripe sequence, the journal's end when it
	// last reached zero, as Collector describes. A uvarint above zero never
	// begins with a zero byte, so the two cannot be confused, and only a
	// block with a child list has a zero record.
	kindCount byte = 'H'
	// kindRoot keys a named root by its bucket and name. Its value is a
	// RootEntry.
	kindRoot byte = 'R'
	// kindJournal keys a journal record by its big-endian sequence number. Its
	// value is a JournalRecord.
	kindJournal byte = 'J'
	// kindQueue keys a Dead block, one at zero holders whose children no
	// longer count it, by its big-endian ripe sequence, then the block. It
	// holds no value, and the free walks it in ripe order, stopping at the
	// first entry the cursor has not passed.
	kindQueue byte = 'Q'
	// kindCascade keys a block whose cascade is due by the block: a Dying
	// block, at zero holders while its children still count it, whose value
	// is cascadeDown, or a Holding one, with holders while its children do
	// not count it, whose value is cascadeUp. Netting writes it only when a
	// slice cannot run the cascade itself, and the free waits while any
	// exists.
	kindCascade byte = 'C'
	// kindProof keys the completion proof of a block by the block: the graph
	// under it is stored. It holds no value and goes with the block.
	kindProof byte = 'P'
	// kindState is the one LivenessState record.
	kindState byte = 'S'
	// kindLease keys a leased root by its big-endian expiry in Unix
	// milliseconds, then its bucket and name, so the collector seeks to the
	// lapsed ones. It holds no value, and SetRoot and DeleteRoot keep it with
	// the root.
	kindLease byte = 'E'
	// kindHole keys a hole the audit found under a live root, by the block: a
	// block the store lost together with its child list, whose parents' lists
	// still name it. It holds no value. A put of the block counts it as
	// stored, as Land describes, and deletes the record.
	kindHole byte = 'O'
	// kindFillMark keys a fill mark store format 7 wrote on a block a cache
	// fill gave birth to, which gave the block's children no holder. Nothing
	// reads it: opening a format 7 Volume leaves it unadopted, and the
	// adoption's clear deletes every one before it counts the lists.
	kindFillMark byte = 'F'
	// kindUpIndex keys an entry of the up index store format 8 wrote, which
	// named each up pair of the journal past the netting cursor. Nothing
	// reads it: opening a format 8 Volume leaves it unadopted, and the
	// adoption's clear deletes every one before it counts the lists.
	kindUpIndex byte = 'U'
	// kindFillRecord keys the journal record of one cache fill by its
	// big-endian time in Unix milliseconds, then the block it gave birth to,
	// so a fill writes it blind, with no sequence number shared with the
	// journal, and the collector reads the aged records first. Its value is a
	// JournalRecord with one down pair, for the block's own implied holder.
	kindFillRecord byte = 'L'
	// kindFillUps keys the up pairs of a fill record the collector has not
	// applied yet by the record's key: its time, then the filled block. Its
	// value packs the block keys of the children the block's list names, as
	// a child list does. The collector applies it in the first slice that
	// finds it, whatever its age, and frees nothing while any exists, so a
	// fill holds its children before any free can run. Store format 8 keyed
	// a fill's holds under the same byte in another shape; the adoption a
	// format 8 Volume opens to clears them before anything reads one.
	kindFillUps byte = 'N'
	// kindFillBorn keys a fill record not netted yet by the block it gave
	// birth to, then the record's big-endian time. It holds no value and
	// lasts as long as its record. An adoption treats a block with one as
	// born, since the record supplies its children's holders and takes its
	// own implied holder back.
	kindFillBorn byte = 'M'
	// kindVerified keys a block whose child list a list rebuild verified
	// against the block's decoded refs, by the block. It holds no value and
	// lasts until the rebuild clears the Volume's unverified lists mark.
	kindVerified byte = 'V'
	// kindLanded keys a block Land gave birth to while the Volume's child
	// lists are unverified, by the block. It holds no value. Land wrote the
	// block's list from the put's refs, so the list is verified, and every
	// holder of the block came through Land and the journal, so its count is
	// exact although older lists may lack a child. The free takes only such
	// blocks while the mark stands, as Collector describes. A put of a block
	// stored already, a refilled hole and a cache fill write none, since
	// their blocks are older. It lasts until the free takes the block or the
	// list rebuild clears the mark.
	kindLanded byte = 'W'
	// kindParked keys a Dead block the free held back while the Volume's
	// child lists were unverified, because Land did not give birth to it, by
	// its big-endian ripe sequence, then the block, as the free queue keys it.
	// It holds no value. Once the mark clears, the collector moves each back
	// onto the free queue, where the free checks the block again.
	kindParked byte = 'X'
	// kindReached keys a block a streaming walk from the roots reached, by the
	// block. It holds no value; the walk deletes every one when it starts and
	// when it ends.
	kindReached byte = 'K'
	// kindAdoptBorn keys a block Land gave birth to while the Volume adopts,
	// by the block. It holds no value and lasts until the adoption ends.
	kindAdoptBorn byte = 'B'
	// kindAdoptRoot keys a root Land moved while the Volume adopts, by its
	// bucket and name. Its value is the block key the name held when the
	// adoption began, empty when the name was free, and it lasts until the
	// adoption ends.
	kindAdoptRoot byte = 'A'
	// kindReleasePage keys a page of the child list of the block whose paged
	// cascade is in progress, by its big-endian page number. Its value packs
	// up to releasePageChildren block keys, as a child list does. The pages
	// exist from the slice that starts the cascade until the slice that
	// moves their last child.
	kindReleasePage byte = 'D'
	// kindRewritten is one record, with no key past the kind and no value,
	// that a list rebuild writes when it rewrites a list on an adopted
	// Volume: it journals no holder for a child the list gains, so the counts
	// lack those holders until an adoption recounts the lists. Finish returns
	// the Volume to unadopted when it finds it, and the adoption that follows
	// deletes it with the counts.
	kindRewritten byte = 'Y'
)

// The values of a cascade record: the direction its block's children move.
const (
	// cascadeDown marks a Dying block: each child loses its holder.
	cascadeDown byte = 'd'
	// cascadeUp marks a Holding block: each child gains its holder back.
	cascadeUp byte = 'u'
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

// ScanChildren calls fn with each child list in key order. The audit's
// release cursor depends on that order, so this walks a sorted iterator rather
// than ScanPrefix, whose order a store such as Redis leaves undefined.
func (r *Records) ScanChildren(ctx context.Context, fn func(ref *block.BlockRef, children []*block.BlockRef) error) error {
	return r.scan(ctx, r.key(kindChildren), nil, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Decode the block and its list.
		ref, rest, err := ParseBlockKey(key)
		if err != nil || len(rest) != 0 {
			return false, errBadBlockKey
		}
		children, err := decodeValue(value)
		if err != nil {
			return false, err
		}
		return true, fn(ref, children)
	})
}

// scan calls fn with each record under prefix in key order, from the first
// key at or after prefix then from, until fn returns false or an error. fn
// gets the key after prefix and a reader of the value, so a scan that needs
// only keys reads no value. Both are valid only during the call.
func (r *Records) scan(ctx context.Context, prefix, from []byte, fn func(key []byte, value func() ([]byte, error)) (bool, error)) error {
	// Seek from, then walk the records under prefix.
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(append(bytes.Clone(prefix), from...)); err != nil {
		return err
	}
	for ; it.Valid(); it.Next() {
		more, err := fn(it.Key()[len(prefix):], it.Value)
		if err != nil || !more {
			return err
		}
	}
	return it.Err()
}

// decodeValue reads a value packing block keys, as a child list does, and
// decodes it.
func decodeValue(value func() ([]byte, error)) ([]*block.BlockRef, error) {
	v, err := value()
	if err != nil {
		return nil, err
	}
	return DecodeChildren(v)
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

// normalizeChildren returns the distinct, non-empty children in block key
// order, the list EncodeChildren stores, so a count of the list's holders
// matches the list.
func normalizeChildren(children []*block.BlockRef) []*block.BlockRef {
	// Drop the empty refs and sort by block key.
	out := make([]*block.BlockRef, 0, len(children))
	for _, child := range children {
		if !child.GetEmpty() {
			out = append(out, child)
		}
	}
	slices.SortFunc(out, compareRefs)

	// Drop the duplicates.
	return slices.CompactFunc(out, func(a, b *block.BlockRef) bool {
		return compareRefs(a, b) == 0
	})
}

// compareRefs orders a and b as their block keys, from their hash type and
// digest in place, without building either key.
func compareRefs(a, b *block.BlockRef) int {
	ha, hb := a.GetHash(), b.GetHash()
	if c := cmp.Compare(byte(ha.GetHashType()), byte(hb.GetHashType())); c != 0 { //nolint:gosec // hash types fit in a byte.
		return c
	}
	return bytes.Compare(ha.GetHash(), hb.GetHash())
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

// GetHolders returns the netted holders of ref and, at zero holders, its
// ripe sequence: the cursor must pass it before the collector frees ref.
func (r *Records) GetHolders(ctx context.Context, ref *block.BlockRef) (holders, ripe uint64, err error) {
	value, found, err := r.tx.Get(ctx, r.key(kindCount, BlockKey(ref)))
	if err != nil || !found {
		return 1, 0, err
	}
	return decodeHolders(value)
}

// decodeHolders decodes a count record value as GetHolders returns it.
func decodeHolders(value []byte) (holders, ripe uint64, err error) {
	// A zero record holds the ripe sequence after its zero byte.
	if len(value) != 0 && value[0] == 0 {
		ripe, size := binary.Uvarint(value[1:])
		if size <= 0 {
			return 0, 0, errCountMalformed
		}
		return 0, ripe, nil
	}

	// Any other holds the holders beyond the implied one.
	n, size := binary.Uvarint(value)
	if size <= 0 {
		return 0, 0, errCountMalformed
	}
	return n + 1, 0, nil
}

// errCountMalformed reports a count record that decodes to nothing.
var errCountMalformed = errors.New("liveness count record is malformed")

// SetHolders writes the count record of ref with holders, deleting it at one,
// the implied holder, and writing a zero record with ripe at none. Only a
// block with a child list may be set to zero.
func (r *Records) SetHolders(ctx context.Context, ref *block.BlockRef, holders, ripe uint64) error {
	key := r.key(kindCount, BlockKey(ref))
	switch holders {
	case 0:
		return r.tx.Set(ctx, key, binary.AppendUvarint([]byte{0}, ripe))
	case 1:
		return r.tx.Delete(ctx, key)
	}
	return r.tx.Set(ctx, key, binary.AppendUvarint(nil, holders-1))
}

// Held reports whether ref is stored with holders: it has a child list and
// its count record is not a zero record. A root renewed onto a block that
// is not held would hold a block the collector may free, since the
// collector released its children when it reached zero.
func (r *Records) Held(ctx context.Context, ref *block.BlockRef) (bool, error) {
	listed, err := r.HasChildren(ctx, ref)
	if err != nil || !listed {
		return false, err
	}
	holders, _, err := r.GetHolders(ctx, ref)
	return holders != 0, err
}

// tally returns the holders an adoption counted for ref so far, from its
// count record as a plain uvarint, zero when it has none.
func (r *Records) tally(ctx context.Context, ref *block.BlockRef) (uint64, error) {
	// Read the count record as a plain uvarint.
	value, found, err := r.tx.Get(ctx, r.key(kindCount, BlockKey(ref)))
	if err != nil || !found {
		return 0, err
	}
	n, size := binary.Uvarint(value)
	if size <= 0 {
		return 0, errCountMalformed
	}
	return n, nil
}

// setTally writes the holders an adoption counted for ref so far, as a plain
// uvarint, above zero. The adoption's settle step rewrites each into the
// count record GetHolders reads.
func (r *Records) setTally(ctx context.Context, ref *block.BlockRef, n uint64) error {
	return r.tx.Set(ctx, r.key(kindCount, BlockKey(ref)), binary.AppendUvarint(nil, n))
}

// rootKey returns the key of the root named name in bucketID.
func (r *Records) rootKey(bucketID, name string) []byte {
	return r.namedKey(kindRoot, bucketID, name)
}

// namedKey returns the key of kind for the name in bucketID: the bucket's
// length as a uvarint, the bucket and the name.
func (r *Records) namedKey(kind byte, bucketID, name string) []byte {
	return r.key(kind, binary.AppendUvarint(nil, uint64(len(bucketID))), []byte(bucketID), []byte(name))
}

// parseNamedKey splits the part of a namedKey after its kind into the bucket
// and the name.
func parseNamedKey(rest []byte) (string, string, error) {
	n, size := binary.Uvarint(rest)
	if size <= 0 || uint64(len(rest)-size) < n { //nolint:gosec // size > 0 and at most len(rest).
		return "", "", errors.New("liveness root record is malformed")
	}
	return string(rest[size : size+int(n)]), string(rest[size+int(n):]), nil //nolint:gosec // n fits the key.
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

// SetRoot writes the entry of the root named name in bucketID, and moves its
// lease index record to the entry's expiry.
func (r *Records) SetRoot(ctx context.Context, bucketID, name string, entry *RootEntry) error {
	// Move the lease index record.
	if err := r.moveLease(ctx, bucketID, name, entry.GetExpiresUnixMilli()); err != nil {
		return err
	}

	// Write the entry.
	value, err := entry.MarshalVT()
	if err != nil {
		return err
	}
	return r.tx.Set(ctx, r.rootKey(bucketID, name), value)
}

// DeleteRoot frees the root named name in bucketID, with its lease index
// record.
func (r *Records) DeleteRoot(ctx context.Context, bucketID, name string) error {
	if err := r.moveLease(ctx, bucketID, name, 0); err != nil {
		return err
	}
	return r.tx.Delete(ctx, r.rootKey(bucketID, name))
}

// ListRoots calls fn with each root of bucketID whose name starts with
// prefix, in name order.
func (r *Records) ListRoots(ctx context.Context, bucketID, prefix string, fn func(name string, entry *RootEntry) error) error {
	under := append(r.namedKey(kindRoot, bucketID, ""), prefix...)
	return r.scan(ctx, under, nil, func(key []byte, value func() ([]byte, error)) (bool, error) {
		entry, err := decodeMsg[RootEntry](value)
		if err != nil {
			return false, err
		}
		return true, fn(prefix+string(key), entry)
	})
}

// decodeMsg reads a value and decodes it as a message of type T.
func decodeMsg[T any, P interface {
	*T
	UnmarshalVT([]byte) error
}](value func() ([]byte, error)) (P, error) {
	v, err := value()
	if err != nil {
		return nil, err
	}
	msg := P(new(T))
	return msg, msg.UnmarshalVT(v)
}

// ScanRoots calls fn with every named root of every bucket.
func (r *Records) ScanRoots(ctx context.Context, fn func(bucketID, name string, entry *RootEntry) error) error {
	prefix := r.key(kindRoot)
	return r.tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Split the bucket from the name.
		bucketID, name, err := parseNamedKey(key[len(prefix):])
		if err != nil {
			return err
		}

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

// anyKey reports whether a key begins with prefix.
func (r *Records) anyKey(ctx context.Context, prefix []byte) (bool, error) {
	var found bool
	err := r.scan(ctx, prefix, nil, func([]byte, func() ([]byte, error)) (bool, error) {
		found = true
		return false, nil
	})
	return found, err
}

// ScanJournal calls fn with each journal record from seq from on, in order,
// until fn returns false or an error.
func (r *Records) ScanJournal(ctx context.Context, from uint64, fn func(seq uint64, rec *JournalRecord) (bool, error)) error {
	seek := binary.BigEndian.AppendUint64(nil, from)
	return r.scan(ctx, r.key(kindJournal), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Decode the record.
		if len(key) != 8 {
			return false, errors.New("liveness journal key is malformed")
		}
		rec, err := decodeMsg[JournalRecord](value)
		if err != nil {
			return false, err
		}
		return fn(binary.BigEndian.Uint64(key), rec)
	})
}

// queueKey returns the queue key of the Dead block at key with ripe.
func (r *Records) queueKey(ripe uint64, key []byte) []byte {
	return r.key(kindQueue, binary.BigEndian.AppendUint64(nil, ripe), key)
}

// parkedKey returns the parked key of the Dead block at key with ripe.
func (r *Records) parkedKey(ripe uint64, key []byte) []byte {
	return r.key(kindParked, binary.BigEndian.AppendUint64(nil, ripe), key)
}

// listVerified reports whether the child list of the block at key is known
// to name every child: a list rebuild verified it, or Land wrote it while
// the Volume's lists were unverified.
func (r *Records) listVerified(ctx context.Context, key []byte) (bool, error) {
	verified, err := r.tx.Exists(ctx, r.key(kindVerified, key))
	if err != nil || verified {
		return verified, err
	}
	return r.tx.Exists(ctx, r.key(kindLanded, key))
}

// queuePos returns the position on the free queue of the block at key,
// ripe at ripe: its queue key without the prefix and kind.
func queuePos(ripe uint64, key []byte) []byte {
	return append(binary.BigEndian.AppendUint64(nil, ripe), key...)
}

// scanQueue calls fn with each Dead block on the free queue in ripe order,
// from the entry at or after the queue position from, a ripe sequence and
// block key as queuePos returns, or from the start when from is nil, until
// fn returns false or an error.
func (r *Records) scanQueue(ctx context.Context, from []byte, fn func(ripe uint64, ref *block.BlockRef) (bool, error)) error {
	return r.scan(ctx, r.key(kindQueue), from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		if len(key) < 8 {
			return false, errBadBlockKey
		}
		ref, rest, err := ParseBlockKey(key[8:])
		if err != nil || len(rest) != 0 {
			return false, errBadBlockKey
		}
		return fn(binary.BigEndian.Uint64(key), ref)
	})
}

// cascadeKey returns the cascade key of the block at key.
func (r *Records) cascadeKey(key []byte) []byte {
	return r.key(kindCascade, key)
}

// getCascade returns the direction of the cascade due on the block at key,
// and whether one is.
func (r *Records) getCascade(ctx context.Context, key []byte) (byte, bool, error) {
	value, found, err := r.tx.Get(ctx, r.cascadeKey(key))
	if err != nil || !found {
		return 0, false, err
	}
	if len(value) != 1 || (value[0] != cascadeDown && value[0] != cascadeUp) {
		return 0, false, errors.New("liveness cascade record is malformed")
	}
	return value[0], true, nil
}

// addCascade writes the cascade record of the block at key, which has none,
// with dir, and counts it in state, which the caller writes.
func (r *Records) addCascade(ctx context.Context, state *LivenessState, key []byte, dir byte) error {
	state.Cascades++
	return r.tx.Set(ctx, r.cascadeKey(key), []byte{dir})
}

// deleteCascade deletes the cascade record of the block at key, which has
// one, and uncounts it in state, which the caller writes. A count already
// at zero while it is kept is an error: the records and the count disagree,
// and a free trusting the count could run while a cascade is due.
func (r *Records) deleteCascade(ctx context.Context, state *LivenessState, key []byte) error {
	switch {
	case state.GetCascades() != 0:
		state.Cascades--
	case cascadesCounted(state):
		return errors.New("liveness cascade count is below its records")
	}
	return r.tx.Delete(ctx, r.cascadeKey(key))
}

// cascadesCounted reports whether state's cascade count is exact: from the
// transaction that marks an adoption run adopting, after its clear, on.
func cascadesCounted(state *LivenessState) bool {
	return state.GetAdoption() != Adoption_ADOPTION_UNADOPTED
}

// scanCascades calls fn with the block key and direction of each due
// cascade in key order from the block key from, or from the first when from
// is nil, until fn returns false or an error.
func (r *Records) scanCascades(ctx context.Context, from []byte, fn func(key []byte, dir byte) (bool, error)) error {
	return r.scan(ctx, r.key(kindCascade), from, func(key []byte, value func() ([]byte, error)) (bool, error) {
		v, err := value()
		if err != nil {
			return false, err
		}
		if len(v) != 1 {
			return false, errors.New("liveness cascade record is malformed")
		}
		return fn(bytes.Clone(key), v[0])
	})
}

// settled reports whether no cascade is due or in progress: no cascade
// record exists and the state names no paged cascade. Then every block below
// a block with holders has holders too. It reads the state's count of the
// records where it is exact, so a Land pays no seek past the records a long
// cascade deleted, and seeks the records otherwise.
func (r *Records) settled(ctx context.Context, state *LivenessState) (bool, error) {
	switch {
	case state.GetRelease() != nil:
		return false, nil
	case cascadesCounted(state):
		return state.GetCascades() == 0, nil
	}
	due, err := r.anyKey(ctx, r.key(kindCascade))
	return !due, err
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

// releasePageKey returns the key of release page page.
func (r *Records) releasePageKey(page uint64) []byte {
	return r.key(kindReleasePage, binary.BigEndian.AppendUint64(nil, page))
}

// putReleasePages writes children, sorted and distinct as a child list
// holds them, to release pages of releasePageChildren each.
func (r *Records) putReleasePages(ctx context.Context, children []*block.BlockRef) error {
	var page uint64
	for start := 0; start < len(children); start += releasePageChildren {
		end := min(start+releasePageChildren, len(children))
		if err := r.tx.Set(ctx, r.releasePageKey(page), EncodeChildren(children[start:end])); err != nil {
			return err
		}
		page++
	}
	return nil
}

// getReleasePage returns the children release page page holds, none when it
// is missing.
func (r *Records) getReleasePage(ctx context.Context, page uint64) ([]*block.BlockRef, error) {
	value, found, err := r.tx.Get(ctx, r.releasePageKey(page))
	if err != nil || !found {
		return nil, err
	}
	return DecodeChildren(value)
}

// deleteReleasePage deletes release page page.
func (r *Records) deleteReleasePage(ctx context.Context, page uint64) error {
	return r.tx.Delete(ctx, r.releasePageKey(page))
}

// scanReleasePending calls fn with each child of rel not moved yet, in block
// key order, from the release page holding its Next. A child of a release
// still counts the block as a holder, and a child of a re-hold does not yet,
// as continueRelease describes.
func (r *Records) scanReleasePending(ctx context.Context, rel *Release, fn func(child *block.BlockRef) error) error {
	cur := newReleaseCursor(r, rel)
	for {
		child, err := cur.child(ctx)
		if err != nil || child == nil {
			return err
		}
		if err := fn(child); err != nil {
			return err
		}
		cur.next++
	}
}

// releaseCursor walks the children of a paged cascade not moved yet in block
// key order, the order of its block's child list, holding one release page
// at a time.
type releaseCursor struct {
	// rec are the liveness records it reads the pages from.
	rec *Records
	// rel is the release, nil when none is in progress.
	rel *Release
	// next is the position of the child at the cursor.
	next uint64
	// page holds the children of release page pageNo once read.
	page []*block.BlockRef
	// pageNo is the page page holds.
	pageNo uint64
}

// newReleaseCursor returns a cursor at the first child of rel not moved
// yet.
func newReleaseCursor(rec *Records, rel *Release) *releaseCursor {
	return &releaseCursor{rec: rec, rel: rel, next: rel.GetNext()}
}

// child returns the child at the cursor, or nil past the last one.
func (c *releaseCursor) child(ctx context.Context) (*block.BlockRef, error) {
	// Past the last child there is none.
	if c.next >= c.rel.GetChildren() {
		return nil, nil
	}

	// Read the page holding it, unless the cursor holds it already.
	no := c.next / releasePageChildren
	if c.page == nil || c.pageNo != no {
		page, err := c.rec.getReleasePage(ctx, no)
		if err != nil {
			return nil, err
		}
		c.page, c.pageNo = page, no
	}
	within := c.next % releasePageChildren
	if within >= uint64(len(c.page)) {
		return nil, errReleasePageMissing(c.rel, no)
	}
	return c.page[within], nil
}

// has reports whether the release names the block at key, moving the cursor
// past the children below it. Calls pass keys in ascending order, as
// ScanChildren yields them.
func (c *releaseCursor) has(ctx context.Context, key []byte) (bool, error) {
	for {
		child, err := c.child(ctx)
		if err != nil || child == nil {
			return false, err
		}
		switch cmp := bytes.Compare(BlockKey(child), key); {
		case cmp == 0:
			return true, nil
		case cmp > 0:
			return false, nil
		}
		c.next++
	}
}

// errReleasePageMissing reports that release page page of rel is missing or
// ends before the release's Next, naming its block, so an operator can find
// the damaged cascade. No child moves until a repair.
func errReleasePageMissing(rel *Release, page uint64) error {
	name := "a block of unknown key"
	if ref, _, err := ParseBlockKey(rel.GetBlock()); err == nil {
		name = ref.MarshalString()
	}
	return errors.Errorf("release of %s: page %d of its %d children is missing", name, page, rel.GetChildren())
}
