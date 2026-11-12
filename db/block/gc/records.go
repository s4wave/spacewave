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
	"google.golang.org/protobuf/encoding/protowire"
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
	// so its presence is what "stored" means here. A list written from the
	// block's content, by Land, a fill or the settle walk, begins with
	// listMark, or listPartial for a fill's; one an older store wrote does
	// not, and may lack a child, as Generation_GENERATION_OPEN describes. A
	// block key begins with its hash type, never 0 or 0xff, so a mark cannot
	// be confused with a key.
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
	// longer count it, by its place as queuePos encodes it: the big-endian
	// order number of its down cascade, its big-endian ripe sequence, then
	// the block. It holds no value, and the free walks it in cascade order,
	// stopping at the first entry the cursor has not passed, so a parent is
	// freed before the children its release brought to zero.
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
	// kindParked keys a Dead block the free held back while the generation
	// is Open, by its place, as the free queue keys it. It holds no value.
	// Once the flip leaves Open, the settle's parked slice deletes each in
	// that order and sweeps its block while it is unmarked.
	kindParked byte = 'X'
	// kindFront keys a block on the settle's front, by the block: a stored
	// block whose list may lack a child, named by a young or verified block,
	// a root or a recent down pair, which the walk still has to decode. It
	// holds no value. Land, LandFill and the walk push onto it while the
	// generation is Open, as Generation_GENERATION_OPEN describes; a write
	// that marks the block's list takes its entry off, and so does the
	// collector once its free proof holds for the block. An entry whose block
	// the store does not list stays, and keeps the generation Open.
	kindFront byte = 'G'
	// kindUnmarked keys an entry of the unmarked run, by the block: a block
	// whose bytes or list the store holds without a list from content. Its
	// value is empty, runReborn once a put stored the block again, or
	// runSwept once the sweep deleted it. The run is built in slices while
	// the generation is Open, and is complete once its runComplete entry
	// exists; the flip makes each empty entry unmarked. While Sweeping no
	// entry is deleted, so the run stays the run at the flip until it
	// settles, as Generation_GENERATION_SWEEPING describes.
	kindUnmarked byte = 'I'
	// kindUnmarkedCount keys the count record of an unmarked block by the
	// block, in kindCount's encoding, while the generation is Sweeping. The
	// flip freezes an unmarked block's kindCount record, which says whether
	// its old list still holds its children, as
	// Records.oldListHolds reads them; every holder the block gains or loses
	// after the flip lands here instead, as Records.countKind routes it. A
	// block without one has its implied holder alone, as at its birth. When
	// the block leaves the run, by the sweep or a rebirth, Records.releaseOld
	// moves the record over the frozen one.
	kindUnmarkedCount byte = 'T'
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
	// that a list rebuild or a Land writes when a list gains a child on a
	// Volume that keeps a journal: it journals no holder for the child, so
	// the counts lack those holders until an adoption recounts the lists. The
	// settle's flip returns the Volume to unadopted when it finds it, and the
	// adoption that follows deletes it with the counts.
	kindRewritten byte = 'Y'
	// kindVerifiedRetired, kindLandedRetired, kindReachedRetired and
	// kindSuspectRetired are the verified and landed marks, the walk's
	// reached marks and the suspect records an older store wrote. Nothing
	// reads them; an adoption's clear deletes them.
	kindVerifiedRetired byte = 'V'
	kindLandedRetired   byte = 'W'
	kindReachedRetired  byte = 'K'
	kindSuspectRetired  byte = 'Z'
)

// listMark begins a child list written from its block's content.
const listMark byte = 0

// listPartial begins a child list a fill wrote from its block's content. The
// list is partial: a fill stores a block without its subtree, so a child it
// names may be absent, and a read fetches it from the fill's source. Only
// LandFill writes it; a Land that writes the list writes listMark, which
// clears it.
//
// LandFill runs only for a put a read-through store marks as a cache fill of
// a block it read from another store, so no path writes a partial list on a
// Volume with no source. Were one there, a missing child under it would be a
// loss as under any other list: the fetch has nowhere to go.
const listPartial byte = 0xff

// The unmarked run's values, and the key of its completion entry.
var (
	// runReborn is the value of a run entry whose block a put stored again
	// after the flip, so the block is live, and the sweep of an unmarked
	// parent skips it, since the put took back what the old list held.
	runReborn = []byte{'r'}
	// runSwept is the value of a run entry whose block the sweep deleted.
	// The entry stays until the run settles, so a child the block's old list
	// named still reads as inside the run at the flip, and a put of the
	// block after the sweep is a birth, not a rebirth.
	runSwept = []byte{'s'}
	// runComplete is the key after the kind of the entry that says the run
	// names every block the store holds without a list from content. A block
	// key is never empty, so no block's entry has this key.
	runComplete = []byte{}
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
	// unadopted reports that a write in the transaction returned the Volume
	// to unadopted.
	unadopted bool
	// genKnown reports that gen holds the Volume's generation as the
	// transaction sees it, read from the state, written with it, or told by
	// AssumeSettled.
	genKnown bool
	// gen is the generation when genKnown is set.
	gen Generation
	// flip is the state's Flip when genKnown is set, nil outside Sweeping.
	flip *Flip
}

// AssumeSettled tells the records that the Volume's generation is
// Settled, which the Volume knows from a transaction that read or wrote it,
// so the transaction reads no state to learn it. Settled is the last
// generation, so the knowledge never goes stale.
func (r *Records) AssumeSettled() {
	r.genKnown, r.gen, r.flip = true, Generation_GENERATION_SETTLED, nil
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
	state.Adoption = Adoption_ADOPTION_UNADOPTED
	state.AdoptionRun = ""
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

// HasChildren reports whether the child list of ref exists.
func (r *Records) HasChildren(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindChildren, BlockKey(ref)))
}

// PutChildren writes the child list of ref from its content, with listMark.
func (r *Records) PutChildren(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindChildren, BlockKey(ref)), markList(EncodeChildren(children)))
}

// PutPartialChildren writes the child list of ref from its content, with
// listPartial, for a fill.
func (r *Records) PutPartialChildren(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindChildren, BlockKey(ref)), append([]byte{listPartial}, EncodeChildren(children)...))
}

// markList returns the packed block keys as a list value written from
// content.
func markList(packed []byte) []byte {
	return append([]byte{listMark}, packed...)
}

// listMarked reports whether the list value was written from content, by a
// Land, the walk or a fill.
func listMarked(value []byte) bool {
	return len(value) != 0 && (value[0] == listMark || value[0] == listPartial)
}

// listIsPartial reports whether the list value is a fill's partial list.
func listIsPartial(value []byte) bool {
	return len(value) != 0 && value[0] == listPartial
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
// place on the free queue. An unmarked block's holders are the ones since
// the flip, as kindUnmarkedCount describes.
func (r *Records) GetHolders(ctx context.Context, ref *block.BlockRef) (uint64, DeadPos, error) {
	// Read the record of the kind the block's state picks.
	key := BlockKey(ref)
	kind, err := r.countKind(ctx, key)
	if err != nil {
		return 0, DeadPos{}, err
	}
	value, found, err := r.tx.Get(ctx, r.key(kind, key))
	if err != nil || !found {
		return 1, DeadPos{}, err
	}
	return decodeHolders(value)
}

// decodeHolders decodes a count record value as GetHolders returns it.
func decodeHolders(value []byte) (uint64, DeadPos, error) {
	// A zero record holds the ripe sequence, then the order, after its zero
	// byte.
	if len(value) != 0 && value[0] == 0 {
		ripe, size := binary.Uvarint(value[1:])
		if size <= 0 {
			return 0, DeadPos{}, errCountMalformed
		}
		order, osize := binary.Uvarint(value[1+size:])
		if osize <= 0 || 1+size+osize != len(value) {
			return 0, DeadPos{}, errCountMalformed
		}
		return 0, DeadPos{order: order, ripe: ripe}, nil
	}

	// Any other holds the holders beyond the implied one.
	n, size := binary.Uvarint(value)
	if size <= 0 {
		return 0, DeadPos{}, errCountMalformed
	}
	return n + 1, DeadPos{}, nil
}

// errCountMalformed reports a count record that decodes to nothing.
var errCountMalformed = errors.New("liveness count record is malformed")

// SetHolders writes the count record of ref with holders, deleting it at one,
// the implied holder, and writing a zero record with pos at none. Only a
// block with a child list may be set to zero. An unmarked block's record is
// its kindUnmarkedCount record.
func (r *Records) SetHolders(ctx context.Context, ref *block.BlockRef, holders uint64, pos DeadPos) error {
	// Write the record of the kind the block's state picks.
	bkey := BlockKey(ref)
	kind, err := r.countKind(ctx, bkey)
	if err != nil {
		return err
	}
	key := r.key(kind, bkey)
	switch holders {
	case 0:
		return r.tx.Set(ctx, key, binary.AppendUvarint(binary.AppendUvarint([]byte{0}, pos.ripe), pos.order))
	case 1:
		return r.tx.Delete(ctx, key)
	}
	return r.tx.Set(ctx, key, binary.AppendUvarint(nil, holders-1))
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

// Hidden reports whether ref, a block with a child list, reads absent until
// a put stores it again: while Sweeping, an unmarked block, whose put gives
// birth to it again, as Records.rebirth describes; while Open, a parked one,
// whose cascade released its children while the settle's front may have
// popped them, so a hold onto it would revive a list whose children the
// flip can leave in the run. Settled, it reads only the generation; while
// Sweeping, one run entry; while Open, the count record and, for a zero
// one, its parked key.
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
// zero record whose place names a parked key. Call it only while
// Open, when its count is the kindCount record and every parked key waits
// for the flip.
func (r *Records) parked(ctx context.Context, key []byte) (bool, error) {
	// Read the count, and for a zero one its parked key.
	value, found, err := r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil || !found {
		return false, err
	}
	holders, pos, err := decodeHolders(value)
	if err != nil || holders != 0 {
		return false, err
	}
	return r.tx.Exists(ctx, r.parkedKey(pos, key))
}

// repark takes the parked block at key back from the free, while Open, so
// a put can give birth to it again: it deletes its zero count, which leaves
// it the implied holder of a born block, its parked key, its run entry and
// its front entry. Its cascade released its children already, so the
// caller writes its list from content and journals a holder for each child
// as for a born block.
func (r *Records) repark(ctx context.Context, key []byte) error {
	// Read the place that keys the parked entry.
	value, _, err := r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil {
		return err
	}
	_, pos, err := decodeHolders(value)
	if err != nil {
		return err
	}

	// Delete the count, the parked key, the run entry and the front entry.
	for _, del := range [][]byte{r.key(kindCount, key), r.parkedKey(pos, key), r.key(kindUnmarked, key), r.key(kindFront, key)} {
		if err := r.tx.Delete(ctx, del); err != nil {
			return err
		}
	}
	return nil
}

// Held reports whether ref is present with holders: its count record is not
// a zero record. A root renewed onto a block that is not held would hold a
// block the collector may free, since the collector released its children
// when it reached zero. A hidden block is not held: a Land refuses a hold
// onto it.
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

// queueKey returns the queue key of the Dead block at key at pos.
func (r *Records) queueKey(pos DeadPos, key []byte) []byte {
	return r.key(kindQueue, queuePos(pos, key))
}

// parkedKey returns the parked key of the Dead block at key at pos.
func (r *Records) parkedKey(pos DeadPos, key []byte) []byte {
	return r.key(kindParked, queuePos(pos, key))
}

// scanDead calls fn with each Dead block of kind, the free queue or the
// parked keys, in cascade order, from the entry at or after the position
// from, as queuePos returns, or from the start when from is nil, until fn
// returns false or an error.
func (r *Records) scanDead(ctx context.Context, kind byte, from []byte, fn func(pos DeadPos, ref *block.BlockRef) (bool, error)) error {
	return r.scan(ctx, r.key(kind), from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		// Parse the block key past the order and the ripe sequence.
		if len(key) < 16 {
			return false, errBadBlockKey
		}
		ref, rest, err := ParseBlockKey(key[16:])
		if err != nil || len(rest) != 0 {
			return false, errBadBlockKey
		}

		// Decode the place the key sorts by and pass the block on.
		pos := DeadPos{order: binary.BigEndian.Uint64(key), ripe: binary.BigEndian.Uint64(key[8:])}
		return fn(pos, ref)
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
	// Read the record, then note its generation.
	state := &LivenessState{}
	value, found, err := r.tx.Get(ctx, r.key(kindState))
	if err == nil && found {
		err = state.UnmarshalVT(value)
	}
	if err != nil {
		return state, err
	}
	retireSuspectLists(state)
	r.know(state)
	return state, nil
}

// stateSuspectLists is the field number of suspect_lists, which a
// development build of store format 7 wrote in LivenessState and the proto
// now reserves.
const stateSuspectLists protowire.Number = 14

// retireSuspectLists reads a state whose retired suspect_lists field is
// above zero as Open, and drops the field so the next write of the state
// stores Open. Those suspect lists may lack a child, as any unverified list
// may, so the settle must walk them, as Generation_GENERATION_OPEN
// describes. A state with no unknown field reads nothing more.
func retireSuspectLists(state *LivenessState) {
	// Split the retired field from the other unknown fields.
	var keep []byte
	suspect := false
	for b := state.unknownFields; len(b) != 0; {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		m := protowire.ConsumeFieldValue(num, typ, b[n:])
		if m < 0 {
			return
		}
		if num == stateSuspectLists && typ == protowire.VarintType {
			v, _ := protowire.ConsumeVarint(b[n:])
			suspect = suspect || v != 0
		} else {
			keep = append(keep, b[:n+m]...)
		}
		b = b[n+m:]
	}

	// Read suspect lists as Open, and drop the field.
	if !suspect {
		return
	}
	state.unknownFields = keep
	if state.Generation == Generation_GENERATION_SETTLED {
		state.Generation = Generation_GENERATION_OPEN
	}
}

// SetState writes the state record.
func (r *Records) SetState(ctx context.Context, state *LivenessState) error {
	value, err := state.MarshalVT()
	if err != nil {
		return err
	}
	r.know(state)
	return r.tx.Set(ctx, r.key(kindState), value)
}

// know notes the generation and flip of state, read or written by the
// transaction.
func (r *Records) know(state *LivenessState) {
	r.genKnown, r.gen, r.flip = true, state.GetGeneration(), state.GetFlip()
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
