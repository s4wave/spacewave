package block_gc

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"hash/fnv"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// Record kinds. Each kind byte begins a key under the Volume's liveness prefix.
// The child list keeps the kind and encoding of store format 6, so a format 6
// Volume carries its lists to format 7 in place. Format 10 adds the up and down
// logs and moves the pop mark, in lower case outside the old reference graph's
// kinds, which a Volume upgraded from a format before 7 deletes in the
// background. Format9Kinds lists the kinds it no longer writes. Format 11 keeps
// a leased root's expiry on its lease, in the free kinds 'e' and 'o', in place
// of format 10's index by expiry, 'E'.
const (
	// kindChildren keys the child list of a block by the block. Its value
	// packs the block keys of its children, sorted and distinct. The record
	// exists from the put that stored the block until the collector frees
	// it, so its presence is what "stored" means here. A list written from
	// the block's content, by Land, a refill or the settle walk, begins
	// with listMark. One an older store wrote does not, and may lack a
	// child, as Generation_GENERATION_OPEN describes. A block key never
	// begins with a zero byte, so the two cannot be confused.
	kindChildren byte = 'c'
	// kindCount keys the count record of a block by the block. A block with
	// no record has one holder, and a uvarint value n above zero gives it
	// n+1. A stored block at zero holders keeps a zero record instead: a zero
	// byte, then the big-endian ripe sequence, the up log's end when it last
	// reached zero, as Collector describes. A uvarint above zero never begins
	// with a zero byte, so the two cannot be confused, and only a block with a
	// child list has a zero record.
	kindCount byte = 'H'
	// kindRoot keys a named root by its bucket and name. Its value is a
	// RootEntry.
	kindRoot byte = 'R'
	// kindUpLog keys a page of the up log by its big-endian up sequence. Its
	// value packs the block keys of the blocks that gained a holder beyond
	// the one a stored block implies, once per holder, in no order. The
	// collector nets each page whatever its age, from the up cursor on.
	kindUpLog byte = 'u'
	// kindDownLog keys a page of the down log by its big-endian down
	// sequence. Its value is its big-endian commit time in Unix
	// milliseconds, then the packed block keys of the blocks that lost a
	// holder, once per holder. The collector nets the pages in sequence
	// order, each once it is Grace old and only in a slice that drained the
	// up log, so a down never nets before an up written before it, and the
	// first page left is the down log's cursor.
	kindDownLog byte = 'd'
	// kindQueue keys a block at zero holders by its big-endian ripe
	// sequence, then the block. It holds no value. The free walks it in
	// ripe order, stopping at the first entry the up cursor has not
	// reached, and a Land that references a queued block deletes its entry,
	// as Land describes.
	kindQueue byte = 'Q'
	// kindProof keys the completion proof of a block by the block: the graph
	// under it is stored. It holds no value and goes with the block.
	kindProof byte = 'P'
	// kindState is the one LivenessState record.
	kindState byte = 'S'
	// kindLease keys a lease by its ID. Its value is the lease's big-endian
	// expiry in Unix milliseconds, zero once the collector released it, as
	// leases.go describes.
	kindLease byte = 'e'
	// kindLeaseRoot keys a root a lease holds by the ID's length as a
	// uvarint, the ID, then the root's bucket and name as namedKey writes
	// them. It holds no value, and SetRoot and DeleteRoot keep it with the
	// root.
	kindLeaseRoot byte = 'o'
	// kindHole keys a hole the audit found under a live root, by the block:
	// a block the store lost together with its child list, whose parents'
	// lists still reference it. Its value is empty, or holeWrittenOff once
	// an operator wrote the hole off, as Records.writeOffHole describes.
	// Its children's counts may lack its holder: no recount reads a list
	// for it, and an audit cannot tell whether the last one ran before the
	// loss. A put of the block counts it as stored, as Land describes,
	// gives its children its holder, and deletes the record, written off
	// or not. When the counts held it already, they stay one high until a
	// recount.
	kindHole byte = 'O'
	// kindParked keys a Dead block the free held back while the generation
	// is Open, by its big-endian ripe sequence, then the block, as the free
	// queue keys it, when its list still holds its children, as a collector
	// with ParkKeepsHolds parks. It holds no value. A parked block is in the
	// run, as Records.park describes. Once the generation leaves Open the
	// collector moves each back onto the free queue, where the free drops it
	// as unmarked and the sweep deletes it, giving its children their downs.
	kindParked byte = 'X'
	// kindReleased keys a parked block as kindParked does, when its park
	// released the holds its list gave, as the free does: its children have
	// their downs, so its list holds nothing while the key stands. It holds
	// no value. It is the one record of that fact, and every path that gives
	// the block's list a down or an up reads it: an unpark or a put gives the
	// ups back, and the sweep or a rebirth deletes the key and gives no down.
	// The collector never requeues it, since the sweep deletes the block.
	kindReleased byte = 'x'
	// kindFront keys a block on the settle's front, by the block: a block
	// with no list from content, stored or absent, referenced by a young or
	// verified block, a root or a down page not netted yet, which the walk
	// still has to decode. It holds no value. Land and the walk push onto
	// it while the generation is Open, as Generation_GENERATION_OPEN
	// describes. A write that marks the block's list takes its entry off,
	// and so does the collector once its free proof holds for the block. An
	// entry whose block the store does not list stays while it has holders
	// or its bytes remain, for the walk to list it. With neither the proof
	// pops it. A written-off hole is never on it, as Records.writeOffHole
	// describes.
	kindFront byte = 'G'
	// kindUnmarked keys an entry of the unmarked run, by the block: a block
	// whose bytes or list the store holds without a list from content, or
	// one the free parked while Open, as Records.park describes. Its value
	// is empty, runReborn once a put stored the block again, or runSwept
	// once the sweep deleted it. The run is built in slices while the
	// generation is Open, and is complete once its runComplete entry
	// exists. The flip makes each empty entry unmarked. While Sweeping no
	// entry is deleted, so the run stays the run at the flip until it
	// settles, as Generation_GENERATION_SWEEPING describes.
	kindUnmarked byte = 'I'
	// kindUnmarkedCount keys the count record of an unmarked block by the
	// block, in kindCount's encoding, while the generation is Sweeping. The
	// flip freezes an unmarked block's kindCount record. Every holder the
	// block gains or loses after the flip lands here instead, as
	// Records.countKind routes it. A block without one has its implied
	// holder alone, as at its birth. When the block leaves the run, by the
	// sweep or a rebirth, Records.releaseOld moves the record over the
	// frozen one.
	kindUnmarkedCount byte = 'T'
	// kindAdoptBorn keys a block Land gave birth to while the Volume adopts,
	// by the block. It holds no value and lasts until the adoption ends.
	kindAdoptBorn byte = 'B'
	// kindAdoptRoot keys a root Land moved while the Volume adopts, by its
	// bucket and name. Its value is the block key the name held when the
	// adoption began, empty when the name was free, and it lasts until the
	// adoption ends.
	kindAdoptRoot byte = 'A'
	// kindReleasePage keys a page of the child list of the freed block whose
	// paged release is in progress, by its big-endian page number. Its value
	// packs up to releasePageChildren block keys, as a child list does. The
	// pages exist from the free that starts the release until the slice that
	// moves their last child.
	kindReleasePage byte = 'D'
	// kindRewritten is one record, with no key past the kind and no value,
	// that a list rebuild or a Land writes when a list gains a child on a
	// Volume that keeps logs and no adoption counts: it logs no holder for
	// the child, so the counts lack those holders until an adoption
	// recounts the lists. The settle's
	// flip returns the Volume to unadopted when it finds it, and the adoption
	// that follows deletes it with the counts.
	kindRewritten byte = 'Y'
	// kindPopMark is one record, with no key past the kind, whose value is
	// the big-endian latest ripe sequence of a block the settle's front
	// popped while Open. A block at zero that references a popped child
	// reached zero no later than the child, so the flip waits only for the
	// queued blocks at or below the mark, as Collector.flip describes. A
	// readoption after a pop gives such a block a new ripe sequence, so it
	// raises the mark to the largest uint64 while it recounts and caps it
	// at the up log's end once the recount is done, past every ripe
	// sequence the recount gave, as Adopt describes. The flip deletes it.
	kindPopMark byte = 'k'
	// kindAddRev is one record, with no key past the kind, whose value is
	// the big-endian add revision: every transaction that writes a front
	// or hole key raises it once, through Records.addFront,
	// Records.addHole or Records.writeOffHole, the only writers of either
	// kind. A reader that found no front key, and no hole key but a
	// written-off one, in a read snapshot at revision n knows a later
	// transaction that reads n finds the same, from that one point read,
	// without seeking the prefixes over their deleted keys, as flipProof
	// describes. An absent record is revision zero.
	kindAddRev byte = 'v'
	// kindHolesClear is one record, with no key past the kind and no
	// value, present only while no hole key exists. Each collector pass
	// checks it, even before adoption or with frees off. The collector
	// writes it when a snapshot found no hole and the add revision still
	// matches. Records.addHole deletes it, so
	// Land checks a new block for a hole record only while it is absent,
	// as Records.holesOpen describes. An absent record claims nothing.
	// Not 'h', which the old reference graph used and its sweep deletes.
	kindHolesClear byte = 'z'
)

// LiveKinds are the kinds format 12 writes, one per constant above. No
// kind a format upgrade deletes may be among them, or the upgrade would
// delete live records.
var LiveKinds = []byte{
	kindChildren, kindCount, kindRoot, kindUpLog, kindDownLog, kindQueue,
	kindProof, kindState, kindLease, kindLeaseRoot, kindHole, kindParked, kindReleased,
	kindFront, kindUnmarked, kindUnmarkedCount, kindAdoptBorn, kindAdoptRoot,
	kindReleasePage, kindRewritten, kindPopMark, kindAddRev, kindHolesClear,
}

// listMark begins a child list written from its block's content.
const listMark byte = 0

// The unmarked run's values, and the key of its completion entry.
var (
	// runReborn is the value of a run entry whose block a put stored again
	// after the flip, so the block is live, and the sweep of an unmarked
	// parent skips it, since the put took back what the old list held.
	runReborn = []byte{'r'}
	// runSwept is the value of a run entry whose block the sweep deleted.
	// The entry stays until the run settles, so a child the block's old
	// list referenced still reads as inside the run at the flip, and a put
	// of the block after the sweep is a birth, not a rebirth.
	runSwept = []byte{'s'}
	// runComplete is the key after the kind of the entry that says the run
	// contains every block the store holds without a list from content. A
	// block key is never empty, so no block's entry has this key.
	runComplete = []byte{}
)

// Format9Kinds are the kinds store format 9 or an older one wrote that format
// 10 neither writes nor reads: the journal ('J'), cascade records ('C'), the
// fill records and marks ('F', 'L', 'N', 'M'), the up index ('U'), and the
// retired verified, landed, reached and suspect marks ('V', 'W', 'K', 'Z'). The
// format upgrade deletes them in slices. None is a block or a block's bytes.
var Format9Kinds = []byte{'J', 'C', 'F', 'L', 'N', 'M', 'U', 'V', 'W', 'K', 'Z'}

// format9PopMark is the key, after the prefix, of format 9's pop mark, which
// shared its kind with the old reference graph's 'p' records, so the sweep
// of that graph could delete it. Format 10 keeps it under kindPopMark.
const format9PopMark byte = 'p'

// format10Lease is the kind of format 10's lease index, which keyed each
// leased root by its expiry and then its bucket and name. Format 11 keeps the
// expiry on the root's lease instead, and CarryFormat10Leases moves the roots
// onto leases.
const format10Lease byte = 'E'

// CarryFormat9Counts rewrites up to limit of the zero records format 9 wrote
// among the count and frozen count records, after the position after (the kind
// byte and block key of the last record a call read, nil for the first), into
// format 10's encoding, and returns the last position it read and whether more
// may follow. Format 9 wrote the ripe sequence of a zero record as a uvarint,
// format 10 as eight big-endian bytes. A record already nine bytes long is
// format 10's, since a format 9 ripe sequence is far below the 2^49 a nine-byte
// uvarint encoding needs, so a rerun rewrites nothing twice. Every other
// record, holders beyond the implied one, keeps its encoding. It changes no
// count and no ripe sequence, so a parked block's parked key and a queued
// block's queue entry still match it.
func (r *Records) CarryFormat9Counts(ctx context.Context, after []byte, limit int) ([]byte, bool, error) {
	// Gather the format 9 zero records of both count kinds, to limit.
	type carry struct{ key, value []byte }
	var carries []carry
	n := 0
	for _, kind := range []byte{kindCount, kindUnmarkedCount} {
		// Skip a kind the position passed, and resume in the one the
		// position selects.
		var from []byte
		switch {
		case after != nil && after[0] > kind:
			continue
		case after != nil && after[0] == kind:
			from = append(bytes.Clone(after[1:]), 0)
		}

		// Gather the format 9 zero records of the kind.
		prefix := r.key(kind)
		err := r.scan(ctx, prefix, from, func(key []byte, value func() ([]byte, error)) (bool, error) {
			// Stop at the limit, and note the position read.
			if n == limit {
				return false, nil
			}
			n++
			after = append([]byte{kind}, key...)

			// Skip a nonzero record and one already in format 10's encoding.
			v, err := value()
			if err != nil || len(v) == 0 || v[0] != 0 || len(v) == 9 {
				return true, err
			}

			// Encode the ripe sequence as eight big-endian bytes.
			ripe, size := binary.Uvarint(v[1:])
			if size <= 0 || 1+size != len(v) {
				return false, errCountMalformed
			}
			carries = append(carries, carry{
				key:   append(bytes.Clone(prefix), key...),
				value: binary.BigEndian.AppendUint64([]byte{0}, ripe),
			})
			return true, nil
		})
		if err != nil {
			return after, false, err
		}
		if n == limit {
			break
		}
	}

	// Write them once the scan is closed.
	for _, c := range carries {
		if err := r.tx.Set(ctx, c.key, c.value); err != nil {
			return after, false, err
		}
	}
	return after, n == limit, nil
}

// CarryFormat9PopMark moves format 9's pop mark, when it exists, to the key
// format 10 keeps it under, reading one key and writing two.
func (r *Records) CarryFormat9PopMark(ctx context.Context) error {
	// Read the format 9 mark.
	old := r.key(format9PopMark)
	value, found, err := r.tx.Get(ctx, old)
	if err != nil || !found {
		return err
	}
	if err := r.tx.Set(ctx, r.key(kindPopMark), value); err != nil {
		return err
	}
	return r.tx.Delete(ctx, old)
}

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
	if ref.GetEmpty() {
		return nil
	}
	return AppendBlockKey(make([]byte, 0, blockKeyLen(ref)), ref)
}

// blockKeyLen returns the length of the BlockKey of a non-empty ref.
func blockKeyLen(ref *block.BlockRef) int {
	return 1 + len(ref.GetHash().GetHash())
}

// AppendBlockKey appends the BlockKey of a non-empty ref to dst, so a caller
// building many keys can share one allocation.
func AppendBlockKey(dst []byte, ref *block.BlockRef) []byte {
	h := ref.GetHash()
	dst = append(dst, byte(h.GetHashType())) //nolint:gosec // hash types fit in a byte.
	return append(dst, h.GetHash()...)
}

// ParseBlockKey returns the ref a BlockKey encodes and the bytes after it.
func ParseBlockKey(key []byte) (*block.BlockRef, []byte, error) {
	// Read a known hash type followed by a digest of its length.
	bkey, ok := nextBlockKey(key)
	if !ok {
		return nil, nil, errBadBlockKey
	}
	digest := bytes.Clone(bkey[1:])
	return block.NewBlockRef(&hash.Hash{HashType: hash.HashType(bkey[0]), Hash: digest}), key[len(bkey):], nil
}

// errBadBlockKey reports a record naming a block in an unknown encoding.
var errBadBlockKey = errors.New("liveness record names a block in an unknown encoding")

// Records reads and writes the liveness records of one Volume transaction:
// the child lists, count records and named roots that decide which blocks
// are live, and the up and down logs, free queue, proofs and state the
// collector keeps. Every key begins with prefix. Opening Records reads nothing.
type Records struct {
	// tx is the Volume transaction.
	tx kvtx.TxOps
	// prefix begins every liveness key.
	prefix []byte
	// unadopted reports that a write in the transaction returned the Volume
	// to unadopted.
	unadopted bool
	// genKnown reports that gen holds the Volume's generation as the
	// transaction sees it, read from the state, written with it, or told by
	// AssumeSettled.
	genKnown bool
	// gen is the generation when genKnown is set.
	gen Generation
	// moved reports that a write in the transaction moved the generation.
	moved bool
	// queued are the queue entries the transaction wrote through enqueue, in
	// the order written, which the collector reads once it commits, as
	// Collector.queueFrom describes.
	queued []queuedBlock
	// addRaised reports that the transaction raised the add revision, so a
	// later front or hole key it writes raises it no further.
	addRaised bool
}

// AssumeSettled tells the records that the Volume's generation is
// Settled, which the Volume knows from a transaction that read or wrote it,
// so the transaction reads no state to learn it. Only a transaction that
// writes the state moves the generation, and it reads the state first, so
// a Volume that clears its knowledge before such a write commits never
// assumes a stale generation, as GenerationMoved describes.
func (r *Records) AssumeSettled() {
	r.genKnown, r.gen = true, Generation_GENERATION_SETTLED
}

// GenerationMoved reports whether a write in the transaction moved the
// Volume's generation: the settle's flip and its last step, or a reopen, as
// Land describes. A Volume that caches the generation drops the cache
// before such a transaction commits.
func (r *Records) GenerationMoved() bool {
	return r.moved
}

// Settled reports whether the transaction has seen the Volume's generation
// at Settled, read from the state or written with it.
func (r *Records) Settled() bool {
	return r.genKnown && r.gen == Generation_GENERATION_SETTLED
}

// generation returns the Volume's generation, reading the state unless the
// transaction knows it.
func (r *Records) generation(ctx context.Context) (Generation, error) {
	if !r.genKnown {
		if _, err := r.GetState(ctx); err != nil {
			return 0, err
		}
	}
	return r.gen, nil
}

// NewRecords returns the liveness records under prefix in tx.
func NewRecords(tx kvtx.TxOps, prefix []byte) *Records {
	return &Records{tx: tx, prefix: prefix}
}

// unadopt returns the Volume in state to unadopted under no run, as a claim
// does, ending any release in progress, and notes it for Unadopted. The
// caller writes state.
func (r *Records) unadopt(state *LivenessState) {
	// End any run and release, dropping the run's cursor.
	state.Adoption = Adoption_ADOPTION_UNADOPTED
	state.AdoptionRun = ""
	state.AdoptCursor = nil
	state.Release = nil
	r.unadopted = true
}

// Unadopted reports whether a write in the transaction returned the Volume to
// unadopted, so once it commits the owner of the Volume's adoption starts the
// recount.
func (r *Records) Unadopted() bool {
	return r.unadopted
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

// GetChildList returns the children in the child list of ref and what the
// list proves: RefsAsserted for a marked list, RefsListed for an unverified
// one, which may lack a child, and RefsUnknown when the Volume does not store
// ref.
func (r *Records) GetChildList(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, block.RefsSource, error) {
	// Read the list, then say what its mark proves.
	value, found, err := r.tx.Get(ctx, r.key(kindChildren, BlockKey(ref)))
	if err != nil || !found {
		return nil, block.RefsUnknown, err
	}
	children, err := DecodeChildren(value)
	if listMarked(value) {
		return children, block.RefsAsserted, err
	}
	return children, block.RefsListed, err
}

// HasChildren reports whether the child list of ref exists.
func (r *Records) HasChildren(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.listed(ctx, BlockKey(ref))
}

// listed reports whether the child list of the block at key exists.
func (r *Records) listed(ctx context.Context, key []byte) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindChildren, key))
}

// PutChildren writes the child list of ref from its content, with listMark.
func (r *Records) PutChildren(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindChildren, BlockKey(ref)), packChildren([]byte{listMark}, normalizeChildren(children)))
}

// markList returns the packed block keys as a list value written from
// content.
func markList(packed []byte) []byte {
	return append([]byte{listMark}, packed...)
}

// listMarked reports whether the list value was written from content.
func listMarked(value []byte) bool {
	return len(value) != 0 && value[0] == listMark
}

// listKeys returns the packed block keys of a list value, without its mark.
func listKeys(value []byte) []byte {
	if listMarked(value) {
		return value[1:]
	}
	return value
}

// DeleteChildren deletes the child list of ref, which frees it in the liveness
// sense.
func (r *Records) DeleteChildren(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Delete(ctx, r.key(kindChildren, BlockKey(ref)))
}

// ScanChildren calls fn with each child list in key order. The audit's release
// cursor depends on that order, so ScanChildren walks a sorted iterator rather
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
	return packChildren(nil, normalizeChildren(children))
}

// packChildren returns head followed by the keys of children, which
// normalizeChildren returned, in one allocation.
func packChildren(head []byte, children []*block.BlockRef) []byte {
	// Size the value, then append each key after head.
	size := len(head)
	for _, child := range children {
		size += blockKeyLen(child)
	}
	value := append(make([]byte, 0, size), head...)
	for _, child := range children {
		value = AppendBlockKey(value, child)
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

// DecodeChildren decodes a child list value, marked or not, or packed block
// keys.
func DecodeChildren(value []byte) ([]*block.BlockRef, error) {
	var children []*block.BlockRef
	value = listKeys(value)
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
// ripe sequence: the cursor must pass it before the collector frees ref. An
// unmarked block's holders are the ones since the flip, as
// kindUnmarkedCount describes.
func (r *Records) GetHolders(ctx context.Context, ref *block.BlockRef) (holders, ripe uint64, err error) {
	return r.holders(ctx, BlockKey(ref))
}

// holders returns GetHolders of the block at key.
func (r *Records) holders(ctx context.Context, key []byte) (holders, ripe uint64, err error) {
	// Read the record of the kind the block's state picks.
	kind, err := r.countKind(ctx, key)
	if err != nil {
		return 0, 0, err
	}
	value, found, err := r.tx.Get(ctx, r.key(kind, key))
	if err != nil || !found {
		return 1, 0, err
	}
	return decodeHolders(value)
}

// decodeHolders decodes a count record value as GetHolders returns it.
func decodeHolders(value []byte) (holders, ripe uint64, err error) {
	// A zero record holds the ripe sequence after its zero byte.
	if len(value) != 0 && value[0] == 0 {
		if len(value) != 9 {
			return 0, 0, errCountMalformed
		}
		return 0, binary.BigEndian.Uint64(value[1:]), nil
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

// parseCount decodes the holders a count record of the plain uvarint
// encoding holds.
func parseCount(value []byte) (uint64, error) {
	n, size := binary.Uvarint(value)
	if size <= 0 {
		return 0, errCountMalformed
	}
	return n, nil
}

// SetHolders writes the count record of ref with holders, deleting it at one,
// the implied holder, and writing a zero record with ripe at none. Only a
// block with a child list may be set to zero. An unmarked block's record is
// its kindUnmarkedCount record.
func (r *Records) SetHolders(ctx context.Context, ref *block.BlockRef, holders, ripe uint64) error {
	// Write the record of the kind the block's state picks.
	bkey := BlockKey(ref)
	kind, err := r.countKind(ctx, bkey)
	if err != nil {
		return err
	}
	key := r.key(kind, bkey)
	switch holders {
	case 0:
		return r.tx.Set(ctx, key, zeroRecord(ripe))
	case 1:
		return r.tx.Delete(ctx, key)
	}
	return r.tx.Set(ctx, key, binary.AppendUvarint(nil, holders-1))
}

// zeroRecord returns the count record value of a block at zero holders,
// ripe at ripe.
func zeroRecord(ripe uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte{0}, ripe)
}

// Present reports whether the Volume stores ref in the liveness sense: it
// has a child list and does not read absent, as Hidden describes. A reader
// treats a block that is not present as absent, and a Land refuses a hold
// onto one, so the writer puts it again.
func (r *Records) Present(ctx context.Context, ref *block.BlockRef) (bool, error) {
	listed, err := r.HasChildren(ctx, ref)
	if err != nil || !listed {
		return false, err
	}
	hidden, err := r.Hidden(ctx, ref)
	return !hidden, err
}

// Hidden reports whether ref, a block with a child list, reads absent until a
// put stores it again: while Sweeping, an unmarked block, whose put gives birth
// to it again, as Records.rebirth describes, including every block parked at
// the flip, since the park put the block in the run. While Open, a parked
// block, which the walk skips, so its list may lack a child the flip leaves in
// the run, and a hold onto it would revive that list over a child the sweep
// deletes. While Settled, Hidden reads only the generation. While Sweeping,
// Hidden reads one run entry. While Open, Hidden reads the count record and,
// for a zero record, its parked key.
func (r *Records) Hidden(ctx context.Context, ref *block.BlockRef) (bool, error) {
	gen, err := r.generation(ctx)
	if err != nil {
		return false, err
	}
	switch gen {
	case Generation_GENERATION_OPEN:
		return r.parked(ctx, BlockKey(ref))
	case Generation_GENERATION_SWEEPING:
		return r.Unmarked(ctx, BlockKey(ref))
	}
	return false, nil
}

// parked reports whether the free parked the block at key: its count is a
// zero record whose ripe sequence names a parked or released key. Call it
// only while Open, when its count is the kindCount record and every parked
// key waits for the flip.
func (r *Records) parked(ctx context.Context, key []byte) (bool, error) {
	_, parked, released, err := r.parkState(ctx, key)
	return parked || released, err
}

// parkState reports whether the block at key, with a zero count record
// ripe at ripe, has a parked key and whether it has a released key. The
// flip freezes an unmarked block's kindCount record, so while Sweeping it
// finds the key the park wrote. It reads the count, and for a zero one up to
// two keys.
func (r *Records) parkState(ctx context.Context, key []byte) (ripe uint64, parked, released bool, err error) {
	// Read the count, then for a zero one its released and parked keys.
	value, found, err := r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil || !found {
		return 0, false, false, err
	}
	holders, ripe, err := decodeHolders(value)
	if err != nil || holders != 0 {
		return 0, false, false, err
	}
	if released, err = r.tx.Exists(ctx, r.releasedKey(ripe, key)); err != nil || released {
		return ripe, false, released, err
	}
	parked, err = r.tx.Exists(ctx, r.parkedKey(ripe, key))
	return ripe, parked, false, err
}

// releasedLists returns the block keys of the lists a park while Open
// released: those of the blocks with a released key, whose children the park
// gave their downs, so the lists name children they no longer hold. It scans
// the released keys, one per such park, and keeps them in memory, so it
// suits an audit.
func (r *Records) releasedLists(ctx context.Context) (map[string]struct{}, error) {
	released := make(map[string]struct{})
	err := r.scan(ctx, r.key(kindReleased), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		if len(key) < 8 {
			return false, errBadBlockKey
		}
		released[string(key[8:])] = struct{}{}
		return true, nil
	})
	return released, err
}

// parkedAt reports whether the block at key, with a zero count ripe at
// ripe, has a parked or a released key. It reads up to two keys.
func (r *Records) parkedAt(ctx context.Context, key []byte, ripe uint64) (bool, error) {
	if released, err := r.tx.Exists(ctx, r.releasedKey(ripe, key)); err != nil || released {
		return released, err
	}
	return r.tx.Exists(ctx, r.parkedKey(ripe, key))
}

// AnyParked reports whether the free parked any block that waits for the
// flip. It seeks the parked and released prefixes, so it steps over every
// deleted key at their start; its one caller reads it once as the Volume
// opens, in a read snapshot that holds no writer.
func (r *Records) AnyParked(ctx context.Context) (bool, error) {
	found, err := r.anyKey(ctx, r.key(kindParked))
	if err != nil || found {
		return found, err
	}
	return r.anyKey(ctx, r.key(kindReleased))
}

// park parks the Dead block at key, queued at the queue key queued, while
// Open: its queue entry turns into its parked key, parked, a kindParked key
// or, when the caller gives the children their downs in the same
// transaction, a kindReleased one, and it joins the run, so the flip unmarks
// it with the old blocks. The walk skips a parked
// block, so its list may lack a child the flip leaves in the run; a block
// that names such a child and stays present after the flip could be held
// again over a child the sweep deletes. An old block's run entry is
// already empty, or the run's scan writes it, so the write changes nothing
// there. It reads nothing.
func (r *Records) park(ctx context.Context, queued, parked, key []byte) error {
	if err := r.tx.Delete(ctx, queued); err != nil {
		return err
	}
	if err := r.tx.Set(ctx, parked, nil); err != nil {
		return err
	}
	return r.tx.Set(ctx, r.key(kindUnmarked, key), nil)
}

// unpark ends the park of the block at key, parked at ripe, when it gains a
// holder while Open, as an up gives it: its parked key goes, and so does its
// run entry when its list is marked, which only the park wrote. An old block's
// list stays unverified, so it stays in the run. A released park's list holds
// again once its key goes, and its children lack those holders, so it records
// under state that the counts owe a recount, which the flip runs before
// anything is deleted, and until which nothing is freed or parked: a bounded
// write in place of a cascade of ups. Land refuses a hold onto a parked block
// except in a partial batch, as a cache fill puts, so such a fill's up or an
// underflow reaches it: the free parks only up to the up cursor, so no up
// logged before the park nets after it. It reads the released key and the list
// record.
func (r *Records) unpark(ctx context.Context, state *LivenessState, key []byte, ripe uint64) error {
	// Delete the park's key, noting a recount for a released one.
	released, err := r.tx.Exists(ctx, r.releasedKey(ripe, key))
	if err != nil {
		return err
	}
	if released {
		if err := r.tx.Delete(ctx, r.releasedKey(ripe, key)); err != nil {
			return err
		}
		if err := r.noteRewrite(ctx, state); err != nil {
			return err
		}
	}
	if err := r.tx.Delete(ctx, r.parkedKey(ripe, key)); err != nil {
		return err
	}

	// Leave the run when the list is marked.
	_, marked, err := r.listState(ctx, key)
	if err != nil || !marked {
		return err
	}
	return r.tx.Delete(ctx, r.key(kindUnmarked, key))
}

// unrunParked takes each parked block of kind, kindParked or kindReleased, that
// page names and its transaction still reads, out of the run, as unpark does.
// The caller calls it only while the generation is Open. A readoption calls it
// before it deletes the parked records, and its recount then counts every
// stored list, a released one too. A released list holds its children again
// once counted, and the walk may have popped a child while the park left it at
// zero, so each child the walk owes a decode returns to the front, absent ones
// too: the count returns only stored children of marked lists. It reads one
// list record per parked block, and one per child of a released one.
func (r *Records) unrunParked(ctx context.Context, kind byte, page *keyPage) error {
	// Take each marked one out of the run; an unverified one stays. Return
	// a released one's owed children to the front.
	return page.each(func(parked []byte) error {
		// Skip a key taken out since the snapshot.
		if len(parked) < 8 {
			return errBadBlockKey
		}
		if found, err := r.tx.Exists(ctx, r.key(kind, parked)); err != nil || !found {
			return err
		}
		key := parked[8:]
		list, listed, err := r.tx.Get(ctx, r.key(kindChildren, key))
		if err != nil {
			return err
		}
		if listed && listMarked(list) {
			if err := r.tx.Delete(ctx, r.key(kindUnmarked, key)); err != nil {
				return err
			}
		}
		if listed && kind == kindReleased {
			return r.pushChildren(ctx, listKeys(list), nil)
		}
		return nil
	})
}

// repark takes the parked block at key back from the free, while Open, so
// a put can give birth to it again: it deletes its zero count, which leaves
// it the implied holder of a born block, its parked key, its run entry and
// its front entry. A put of a block without a list left with a zero count
// reparks it in any generation; while Sweeping its run entry stays, since
// the run stays the run at the flip, and it is not unmarked. The caller
// writes its list from content and logs a holder for each child as for a
// born block. A park that kept its holds left the stored list holding its
// children, so it returns the block keys of that list, which lose this
// holder as a rebirth's released children do, and reports holds. A released
// park already gave them their downs, so it returns that list with holds
// false: the put's list gives the ups, and a copied put that keeps the stored
// list gives that list's.
func (r *Records) repark(ctx context.Context, state *LivenessState, key []byte) (children [][]byte, holds bool, err error) {
	// Read the ripe sequence that keys the parked entry, and whether the
	// park released the list's holds.
	value, _, err := r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil {
		return nil, false, err
	}
	_, ripe, err := decodeHolders(value)
	if err != nil {
		return nil, false, err
	}
	released, err := r.tx.Exists(ctx, r.releasedKey(ripe, key))
	if err != nil {
		return nil, false, err
	}

	// Read the children of the stored list.
	list, _, err := r.tx.Get(ctx, r.key(kindChildren, key))
	if err != nil {
		return nil, false, err
	}
	for packed := listKeys(list); len(packed) != 0; {
		child, ok := nextBlockKey(packed)
		if !ok {
			return nil, false, errBadBlockKey
		}
		children = append(children, bytes.Clone(child))
		packed = packed[len(child):]
	}

	// Delete the count, the parked keys, the queue entry, the front entry
	// and, unless Sweeping, the run entry.
	for _, del := range [][]byte{
		r.key(kindCount, key), r.parkedKey(ripe, key), r.releasedKey(ripe, key),
		r.queueKey(ripe, key), r.key(kindFront, key),
	} {
		if err := r.tx.Delete(ctx, del); err != nil {
			return nil, false, err
		}
	}
	if state.GetGeneration() == Generation_GENERATION_SWEEPING {
		return children, !released, nil
	}
	return children, !released, r.tx.Delete(ctx, r.key(kindUnmarked, key))
}

// Held reports whether ref is present with holders: its count record is not
// a zero record. A root renewed onto a block that is not held would hold a
// block the collector may free. A hidden block is not held: a Land refuses a
// hold onto it.
func (r *Records) Held(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Answer absent for a block that is not present.
	present, err := r.Present(ctx, ref)
	if err != nil || !present {
		return false, err
	}

	// Read whether its count holds it.
	holders, _, err := r.GetHolders(ctx, ref)
	return holders != 0, err
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

// ParseNamedKey splits the part of a namedKey after its kind into the bucket
// and the name.
func ParseNamedKey(rest []byte) (string, string, error) {
	// Bound the bucket slice before converting either component.
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
// lease index record to the entry's lease.
func (r *Records) SetRoot(ctx context.Context, bucketID, name string, entry *RootEntry) error {
	// Move the lease index record.
	if err := r.moveLeaseRoot(ctx, bucketID, name, entry.GetLease()); err != nil {
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
	if err := r.moveLeaseRoot(ctx, bucketID, name, ""); err != nil {
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
		bucketID, name, err := ParseNamedKey(key[len(prefix):])
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

// runIncomplete reports whether the unmarked run lacks its completion
// entry. It reads one key.
func (r *Records) runIncomplete(ctx context.Context) (bool, error) {
	done, err := r.tx.Exists(ctx, r.key(kindUnmarked, runComplete))
	return !done, err
}

// addFront pushes the block at key onto the front, raising the add
// revision, as kindAddRev describes. It is the one writer of a front key.
func (r *Records) addFront(ctx context.Context, key []byte) error {
	if err := r.raiseAddRev(ctx); err != nil {
		return err
	}
	return r.tx.Set(ctx, r.key(kindFront, key), nil)
}

// addHole records the block at key as a hole, raising the add revision
// and deleting the holes-clear mark, as kindAddRev and kindHolesClear
// describe. It is the one writer that adds a hole key: writeOffHole
// rewrites the value of one that exists.
func (r *Records) addHole(ctx context.Context, key []byte) error {
	if err := r.raiseAddRev(ctx); err != nil {
		return err
	}
	if err := r.tx.Delete(ctx, r.key(kindHolesClear)); err != nil {
		return err
	}
	return r.tx.Set(ctx, r.key(kindHole, key), nil)
}

// raiseAddRev raises the add revision once per transaction. Cost: one point
// read and one write, in a transaction that writes a front or hole key.
func (r *Records) raiseAddRev(ctx context.Context) error {
	// Raise it once, however many keys the transaction adds.
	if r.addRaised {
		return nil
	}
	rev, err := r.addRev(ctx)
	if err != nil {
		return err
	}
	r.addRaised = true
	return r.tx.Set(ctx, r.key(kindAddRev), binary.BigEndian.AppendUint64(nil, rev+1))
}

// addRev reads the add revision. It reads one key.
func (r *Records) addRev(ctx context.Context) (uint64, error) {
	value, found, err := r.tx.Get(ctx, r.key(kindAddRev))
	if err != nil || !found {
		return 0, err
	}
	if len(value) != 8 {
		return 0, errors.New("liveness add revision is malformed")
	}
	return binary.BigEndian.Uint64(value), nil
}

// holeWrittenOff is the value of a hole key an operator wrote off.
var holeWrittenOff = []byte{'w'}

// writeOffHole writes off the hole at key and takes it off the front,
// and reports whether a hole record exists there. A hole written off
// already keeps its value.
//
// Invariant: a written-off hole has no front key. This deletes the key,
// and pushOwed, the one push that reaches a block without a list, skips a
// written-off hole. So the front holds the flip for it no longer, and no
// scan of an idle front has to find it. The delete raises no pop mark:
// the mark makes the flip wait for the Dead parents of a popped child to
// park, so none revives over a child the sweep deleted, as kindPopMark
// describes, and the sweep has nothing of a hole to delete. The record
// stays a hole key, so the holes-clear mark stays absent, Land still reads
// the key for a put of the block and refills it, a copied put stays
// refused, an audit finds it recorded, and the adoption forgets it once no
// holder references it. What the write-off gives up is what the Volume
// stores only below the hole: no list names it, so the walk never reaches
// it and the sweep deletes it with the run.
//
// Cost: three point reads and three writes, in an operator's transaction.
func (r *Records) writeOffHole(ctx context.Context, key []byte) (bool, error) {
	// Require the hole record.
	hole, err := r.tx.Exists(ctx, r.key(kindHole, key))
	if err != nil || !hole {
		return false, err
	}

	// Write it off, raising the add revision as every hole write does.
	if err := r.raiseAddRev(ctx); err != nil {
		return true, err
	}
	if err := r.tx.Set(ctx, r.key(kindHole, key), holeWrittenOff); err != nil {
		return true, err
	}
	return true, r.tx.Delete(ctx, r.key(kindFront, key))
}

// writtenOff reports whether the block at key is a written-off hole. It
// reads the holes-clear mark, and the hole key only while the mark is
// absent, so it costs one point read while no hole is recorded.
func (r *Records) writtenOff(ctx context.Context, key []byte) (bool, error) {
	open, err := r.holesOpen(ctx)
	if err != nil || !open {
		return false, err
	}
	value, found, err := r.tx.Get(ctx, r.key(kindHole, key))
	return found && bytes.Equal(value, holeWrittenOff), err
}

// holesOpen reports whether a hole key may exist: the holes-clear mark is
// absent, as kindHolesClear describes. It reads one key.
func (r *Records) holesOpen(ctx context.Context) (bool, error) {
	clear, err := r.tx.Exists(ctx, r.key(kindHolesClear))
	return !clear, err
}

// holesHold reports whether a recorded hole may hold the flip: the
// holes-clear mark is absent and a hole key is not written off. It reads
// the mark, and without it every hole key and value, which a Volume
// carries only while an operator leaves a loss recorded.
func (r *Records) holesHold(ctx context.Context) (bool, error) {
	// Take the mark's word.
	open, err := r.holesOpen(ctx)
	if err != nil || !open {
		return false, err
	}

	// Stop at the first hole nobody wrote off.
	var holds bool
	err = r.scan(ctx, r.key(kindHole), nil, func(_ []byte, value func() ([]byte, error)) (bool, error) {
		v, err := value()
		holds = err == nil && !bytes.Equal(v, holeWrittenOff)
		return err == nil && !holds, err
	})
	return holds, err
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

// upKey returns the key of the up page seq.
func (r *Records) upKey(seq uint64) []byte {
	return r.key(kindUpLog, binary.BigEndian.AppendUint64(nil, seq))
}

// downKey returns the key of the down page seq.
func (r *Records) downKey(seq uint64) []byte {
	return r.key(kindDownLog, binary.BigEndian.AppendUint64(nil, seq))
}

// downValue encodes a down page's value: its commit time, then its keys.
func downValue(at int64, keys ...[]byte) []byte {
	head := binary.BigEndian.AppendUint64(nil, uint64(at)) //nolint:gosec // decoded back to the same int64.
	return slices.Concat(append([][]byte{head}, keys...)...)
}

// downPage is one page of the down log.
type downPage struct {
	// at is its commit time in Unix milliseconds, and seq its down sequence.
	at  int64
	seq uint64
	// keys packs the block keys that lose one holder each.
	keys []byte
}

// appendLogs appends ups and downs, block keys gaining or losing one holder
// each, to the up and down logs at now, in pages of logPageKeys, advancing
// state's sequences. The caller writes state. A down page nets only in a slice
// that drained the up log, as Collector.netLogs describes, so it nets after
// these ups.
func (r *Records) appendLogs(ctx context.Context, state *LivenessState, now time.Time, ups, downs [][]byte) error {
	for len(ups) != 0 {
		n := min(len(ups), logPageKeys)
		if err := r.tx.Set(ctx, r.upKey(state.GetUpSeq()), slices.Concat(ups[:n]...)); err != nil {
			return err
		}
		state.UpSeq++
		ups = ups[n:]
	}
	for len(downs) != 0 {
		n := min(len(downs), logPageKeys)
		value := downValue(now.UnixMilli(), downs[:n]...)
		if err := r.tx.Set(ctx, r.downKey(state.GetDownSeq()), value); err != nil {
			return err
		}
		state.DownSeq++
		downs = downs[n:]
	}
	return nil
}

// scanUps calls fn with each up page from seq from on, in order, until fn
// returns false or an error. keys is valid only during the call.
func (r *Records) scanUps(ctx context.Context, from uint64, fn func(seq uint64, keys []byte) (bool, error)) error {
	seek := binary.BigEndian.AppendUint64(nil, from)
	return r.scan(ctx, r.key(kindUpLog), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		if len(key) != 8 {
			return false, errors.New("liveness up page key is malformed")
		}
		v, err := value()
		if err != nil {
			return false, err
		}
		return fn(binary.BigEndian.Uint64(key), v)
	})
}

// scanDowns calls fn with each down page from seq from on, in order, until
// fn returns false or an error. The page's keys are valid only during the
// call.
func (r *Records) scanDowns(ctx context.Context, from uint64, fn func(p downPage) (bool, error)) error {
	seek := binary.BigEndian.AppendUint64(nil, from)
	return r.scan(ctx, r.key(kindDownLog), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		v, err := value()
		if err != nil {
			return false, err
		}
		if len(key) != 8 || len(v) < 8 {
			return false, errors.New("liveness down page is malformed")
		}
		return fn(downPage{
			at:   int64(binary.BigEndian.Uint64(v)), //nolint:gosec // written from an int64.
			seq:  binary.BigEndian.Uint64(key),
			keys: v[8:],
		})
	})
}

// downCursor returns the sequence of the first down page left, which every
// down before it netted, or the down log's end when none is left, reading
// one key from the state's down cursor.
func (r *Records) downCursor(ctx context.Context, state *LivenessState) (uint64, error) {
	cursor := state.GetDownSeq()
	err := r.scanDowns(ctx, state.GetDownCursor(), func(p downPage) (bool, error) {
		cursor = p.seq
		return false, nil
	})
	return cursor, err
}

// deleteDown deletes the down page p.
func (r *Records) deleteDown(ctx context.Context, p downPage) error {
	return r.tx.Delete(ctx, r.downKey(p.seq))
}

// restampDown stamps the down page p at the time at, keeping its keys.
func (r *Records) restampDown(ctx context.Context, p downPage, at int64) error {
	return r.tx.Set(ctx, r.downKey(p.seq), downValue(at, p.keys))
}

// eachKey calls fn with each block key packed in keys, as a child list or a
// log page packs them.
func eachKey(keys []byte, fn func(key []byte) error) error {
	for len(keys) != 0 {
		key, ok := nextBlockKey(keys)
		if !ok {
			return errBadBlockKey
		}
		if err := fn(key); err != nil {
			return err
		}
		keys = keys[len(key):]
	}
	return nil
}

// queueKey returns the queue key of the Dead block at key with ripe.
func (r *Records) queueKey(ripe uint64, key []byte) []byte {
	return r.key(kindQueue, binary.BigEndian.AppendUint64(nil, ripe), key)
}

// parkedKey returns the parked key of the Dead block at key with ripe.
func (r *Records) parkedKey(ripe uint64, key []byte) []byte {
	return r.key(kindParked, binary.BigEndian.AppendUint64(nil, ripe), key)
}

// releasedKey returns the released key of the Dead block at key with ripe.
func (r *Records) releasedKey(ripe uint64, key []byte) []byte {
	return r.key(kindReleased, binary.BigEndian.AppendUint64(nil, ripe), key)
}

// queuePos returns the position on the free queue of the block at key,
// ripe at ripe: its queue key without the prefix and kind.
func queuePos(ripe uint64, key []byte) []byte {
	return append(binary.BigEndian.AppendUint64(nil, ripe), key...)
}

// queuedBlock is one entry of the free queue: the block at key, ripe at
// ripe.
type queuedBlock struct {
	ripe uint64
	key  []byte
}

// pos returns the entry's queue position, as queuePos returns it.
func (q queuedBlock) pos() []byte {
	return queuePos(q.ripe, q.key)
}

// enqueue writes the queue entry of the block at key, ripe at ripe, and
// notes it in queued. Every writer of a queue entry writes through it, so
// the collector learns each position its committed transactions wrote.
func (r *Records) enqueue(ctx context.Context, ripe uint64, key []byte) error {
	r.queued = append(r.queued, queuedBlock{ripe: ripe, key: bytes.Clone(key)})
	return r.tx.Set(ctx, r.queueKey(ripe, key), nil)
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

// ripeQueued reports whether the free queue holds a block at a ripe
// sequence no later than upTo, which the free has yet to take, reading at
// most one entry at or after the queue position from, and returns that
// entry's position, nil when it read none. Each entry deleted before it
// costs the read a step, so a caller that reads in the writer's
// transaction passes from a position a read snapshot found, as the flip
// does.
func (r *Records) ripeQueued(ctx context.Context, from []byte, upTo uint64) (bool, []byte, error) {
	var ripe bool
	var at []byte
	err := r.scanQueue(ctx, from, func(seq uint64, ref *block.BlockRef) (bool, error) {
		ripe, at = seq <= upTo, queuePos(seq, BlockKey(ref))
		return false, nil
	})
	return ripe, at, err
}

// popMark returns the pop mark and whether it exists, as kindPopMark
// describes, reading one key.
func (r *Records) popMark(ctx context.Context) (uint64, bool, error) {
	value, found, err := r.tx.Get(ctx, r.key(kindPopMark))
	if err != nil || !found {
		return 0, false, err
	}
	if len(value) != 8 {
		return 0, false, errors.New("liveness pop mark is malformed")
	}
	return binary.BigEndian.Uint64(value), true, nil
}

// raisePopMark raises the pop mark to at least seq, reading and writing one
// key.
func (r *Records) raisePopMark(ctx context.Context, seq uint64) error {
	mark, found, err := r.popMark(ctx)
	if err != nil || found && mark >= seq {
		return err
	}
	return r.tx.Set(ctx, r.key(kindPopMark), binary.BigEndian.AppendUint64(nil, seq))
}

// capPopMark lowers a pop mark above seq to seq, reading one key and
// writing it only when it lowers the mark. Only Adopt calls it, to end the
// recount's largest mark, as kindPopMark describes.
func (r *Records) capPopMark(ctx context.Context, seq uint64) error {
	mark, found, err := r.popMark(ctx)
	if err != nil || !found || mark <= seq {
		return err
	}
	return r.tx.Set(ctx, r.key(kindPopMark), binary.BigEndian.AppendUint64(nil, seq))
}

// HasProof reports whether ref has a completion proof, which says the store
// holds every block below it. It hides the proof of a hidden block, as
// Hidden describes, which answers absent, so a writer puts it again rather
// than naming it.
func (r *Records) HasProof(ctx context.Context, ref *block.BlockRef) (bool, error) {
	found, err := r.tx.Exists(ctx, r.key(kindProof, BlockKey(ref)))
	if err != nil || !found {
		return false, err
	}
	hidden, err := r.Hidden(ctx, ref)
	return !hidden, err
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
	// Read the record.
	state := &LivenessState{}
	value, found, err := r.tx.Get(ctx, r.key(kindState))
	if err == nil && found {
		err = state.UnmarshalVT(value)
	}
	if err != nil {
		return state, err
	}

	// Read a stale frees mark as frees off, then note the generation.
	if state.GetFrees() {
		mark, err := freesMark(state)
		if err != nil {
			return state, err
		}
		state.Frees = mark == state.GetFreesMark()
	}
	r.genKnown, r.gen = true, state.GetGeneration()
	return state, nil
}

// SetState writes the state record with its frees mark.
func (r *Records) SetState(ctx context.Context, state *LivenessState) error {
	// Mark the record, write it, and note its generation.
	mark, err := freesMark(state)
	if err != nil {
		return err
	}
	state.FreesMark = mark
	value, err := state.MarshalVT()
	if err != nil {
		return err
	}
	r.moved = r.moved || r.genKnown && r.gen != state.GetGeneration()
	r.genKnown, r.gen = true, state.GetGeneration()
	return r.tx.Set(ctx, r.key(kindState), value)
}

// freesEpoch salts every frees mark. Raising it makes each mark an earlier
// build wrote read as stale, so every Volume whose frees that build turned on
// reads as off and counts again at the next switch-on. Raise it when a build
// fixes a defect that left the records of a Volume adopted with frees on
// wrong in a way only a recount corrects.
//
// Epoch 1 follows the adoption's settle of holes: an earlier build left a
// hole punched behind the settle cursor uncounted and unmarked, and its
// refill of a hole an audit recorded after the recount that missed it gave
// the children no holder.
const freesEpoch = 1

// freesMark returns the frees mark of state: the FNV-1a hash of freesEpoch
// and the record without its mark and its unknown fields, as
// LivenessState.frees_mark describes. It leaves state as it found it.
func freesMark(state *LivenessState) (uint64, error) {
	// Hash the known fields alone, then put the mark and the rest back.
	mark, unknown := state.FreesMark, state.unknownFields
	state.FreesMark, state.unknownFields = 0, nil
	value, err := state.MarshalVT()
	state.FreesMark, state.unknownFields = mark, unknown
	h := fnv.New64a()
	_, _ = h.Write([]byte{freesEpoch})
	_, _ = h.Write(value)
	return h.Sum64(), err
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
// key order, from the release page holding its Next. Such a child still
// counts the freed block as a holder, as continueRelease describes.
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

// releaseCursor walks the children of a paged release not moved yet in block
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

// has reports whether the release references the block at key, moving the
// cursor past the children below it. Calls pass keys in ascending order, as
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
// the damaged release. No child moves until a repair.
func errReleasePageMissing(rel *Release, page uint64) error {
	name := "a block of unknown key"
	if ref, _, err := ParseBlockKey(rel.GetBlock()); err == nil {
		name = ref.MarshalString()
	}
	return errors.Errorf("release of %s: page %d of its %d children is missing", name, page, rel.GetChildren())
}
