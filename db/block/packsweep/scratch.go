package packsweep

import (
	"context"
	"encoding/binary"
	"maps"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// TailBytes is the bytes of a block key the sweep keys a block by.
const TailBytes = 16

// Key prefixes of the scratch store.
const (
	catalogPackPrefix     = "c/p/"
	catalogMemberPrefix   = "c/m/"
	catalogRefsPrefix     = "c/r/"
	headKey               = "y/head"
	snapshotPrefix        = "y/s/"
	visitedPrefix         = "y/w/v/"
	frontierPrefix        = "y/w/f/"
	holePrefix            = "y/w/x/"
	judgmentPrefix        = "y/j/"
	closureVisitedPrefix  = "y/c/v/"
	closureFrontierPrefix = "y/c/f/"
	logPullKey            = "y/l/pull"
	cyclePrefix           = "y/"
	packRecordPrefix      = "q/"
	// The compaction's rows stay across cycles with the pack records: each
	// new pack's row, and the pack each compacted key was placed in.
	compactPackPrefix = "k/p/"
	placedPrefix      = "k/t/"
)

// Tail returns the tail of a block key.
func Tail(key []byte) []byte {
	return key[max(len(key)-TailBytes, 0):]
}

// RefTail returns the tail of a block ref's key.
func RefTail(ref *block.BlockRef) []byte {
	return Tail(packfile.BlockKey(ref.GetHash()))
}

// prefixed returns prefix followed by each part.
func prefixed(prefix string, parts ...[]byte) []byte {
	// Size the key.
	n := len(prefix)
	for _, p := range parts {
		n += len(p)
	}

	// Join the prefix and the parts.
	out := make([]byte, 0, n)
	out = append(out, prefix...)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Refs states, best last, so a better copy of a key replaces a worse one.
const (
	// refsCorrupt is a key whose every catalogued copy failed its hash.
	refsCorrupt byte = iota
	// refsUnknown is a key whose copies hold its bytes without its refs.
	refsUnknown
	// refsKnown is a key whose refs are known; none is a leaf.
	refsKnown
)

// Member flags.
const (
	// memberCorrupt marks a copy whose bytes failed their hash.
	memberCorrupt byte = 1 << iota
	// memberRefsUnknown marks a copy stored without its refs.
	memberRefsUnknown
)

// catalogPack is a catalogued pack.
type catalogPack struct {
	// blocks is the keys the pack holds.
	blocks uint64
	// valueBytes is the bytes of the pack's values.
	valueBytes uint64
	// size is the pack's file size.
	size uint64
	// created is the pack's creation time in Unix seconds, zero when unknown.
	created int64
}

// marshal encodes the row.
func (p catalogPack) marshal() []byte {
	b := binary.AppendUvarint(nil, p.blocks)
	b = binary.AppendUvarint(b, p.valueBytes)
	b = binary.AppendUvarint(b, p.size)
	return binary.AppendVarint(b, p.created)
}

// unmarshalCatalogPack decodes a row.
func unmarshalCatalogPack(b []byte) (catalogPack, error) {
	r := &rowReader{b: b, ok: true}
	p := catalogPack{blocks: r.uvarint(), valueBytes: r.uvarint(), size: r.uvarint(), created: r.varint()}
	return p, r.finish("catalog pack row")
}

// snapshotPack is a pack registered at S.
type snapshotPack struct {
	// size is the pack's file size.
	size uint64
	// created is the pack's creation time in Unix seconds, zero when unknown.
	created int64
}

// marshal encodes the row.
func (p snapshotPack) marshal() []byte {
	return binary.AppendVarint(binary.AppendUvarint(nil, p.size), p.created)
}

// unmarshalSnapshotPack decodes a row.
func unmarshalSnapshotPack(b []byte) (snapshotPack, error) {
	r := &rowReader{b: b, ok: true}
	p := snapshotPack{size: r.uvarint(), created: r.varint()}
	return p, r.finish("snapshot row")
}

// member is a key a catalogued pack holds.
type member struct {
	// valueSize is the bytes of the key's value in the pack.
	valueSize uint64
	// flags are the member flags.
	flags byte
}

// marshal encodes the row.
func (m member) marshal() []byte {
	return append(binary.AppendUvarint(nil, m.valueSize), m.flags)
}

// unmarshalMember decodes a row.
func unmarshalMember(b []byte) (member, error) {
	r := &rowReader{b: b, ok: true}
	m := member{valueSize: r.uvarint(), flags: r.byte()}
	return m, r.finish("catalog member row")
}

// marshalRefs encodes a refs row.
func marshalRefs(state byte, refs []*block.BlockRef) ([]byte, error) {
	out := []byte{state}
	for _, ref := range refs {
		b, err := ref.MarshalVT()
		if err != nil {
			return nil, err
		}
		out = binary.AppendUvarint(out, uint64(len(b)))
		out = append(out, b...)
	}
	return out, nil
}

// unmarshalRefs decodes a refs row.
func unmarshalRefs(b []byte) (byte, []*block.BlockRef, error) {
	// Read the state, then each length-prefixed ref to the end.
	r := &rowReader{b: b, ok: true}
	state := r.byte()
	var refs []*block.BlockRef
	for r.ok && len(r.b) != 0 {
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(r.bytes()); err != nil {
			return 0, nil, errors.Wrap(err, "refs row")
		}
		refs = append(refs, ref)
	}

	// Refuse a short row.
	if err := r.finish("refs row"); err != nil {
		return 0, nil, err
	}
	return state, refs, nil
}

// Head is the header of a cycle.
type Head struct {
	// Cycle numbers the cycle, from one.
	Cycle int
	// SnapshotAt is the time S the pack list was taken; zero until it is.
	SnapshotAt time.Time
	// SnapshotPacks is the packs registered at S.
	SnapshotPacks int
	// Catalogued is set once every snapshot pack is in the catalog.
	Catalogued bool
	// Roots are the walk's roots, as b58 refs.
	Roots []string
	// LogPosition is the write log's position R at the roots, empty for none.
	LogPosition string
	// Seeded is set once the roots are on the frontier.
	Seeded bool
	// Walked is set once the frontier is empty.
	Walked bool
	// Live is the keys in L.
	Live uint64
	// Holes counts the holes by reason.
	Holes map[string]uint64
	// JudgedAt is the time of the judgment; zero until it ran.
	JudgedAt time.Time
	// Closure is the keys the closure added to W under the log's keys, which
	// the log pull counts.
	Closure uint64
}

// ReadHead reads the cycle's header, or a zero header when none exists.
func ReadHead(ctx context.Context, store kvtx.Store) (*Head, error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	return readHead(ctx, tx)
}

// readHead reads the header in tx.
func readHead(ctx context.Context, tx kvtx.TxOps) (*Head, error) {
	// Read the row.
	data, found, err := tx.Get(ctx, []byte(headKey))
	if err != nil {
		return nil, err
	}

	// Decode it; no row is a cycle not yet started.
	head := &Head{}
	if !found {
		return head, nil
	}
	if err := head.unmarshal(data); err != nil {
		return nil, errors.Wrap(err, "cycle header")
	}
	return head, nil
}

// writeHead writes the header in tx.
func writeHead(ctx context.Context, tx kvtx.TxOps, head *Head) error {
	return tx.Set(ctx, []byte(headKey), head.marshal())
}

// headVersion is the encoding of the header row.
const headVersion = 1

// Header flags.
const (
	headCatalogued byte = 1 << iota
	headSeeded
	headWalked
)

// marshal encodes the header row.
func (h *Head) marshal() []byte {
	// Write the version, the cycle and the snapshot.
	b := []byte{headVersion}
	b = binary.AppendUvarint(b, uint64(h.Cycle)) //nolint:gosec // a count is never negative.
	b = appendTime(b, h.SnapshotAt)
	b = binary.AppendUvarint(b, uint64(h.SnapshotPacks)) //nolint:gosec // a count is never negative.
	b = append(b, headFlag(h.Catalogued, headCatalogued)|headFlag(h.Seeded, headSeeded)|headFlag(h.Walked, headWalked))

	// Write the walk's roots, position and live count.
	b = binary.AppendUvarint(b, uint64(len(h.Roots)))
	for _, root := range h.Roots {
		b = appendString(b, root)
	}
	b = appendString(b, h.LogPosition)
	b = binary.AppendUvarint(b, h.Live)

	// Write the holes in reason order, then the judgment's time.
	reasons := slices.Sorted(maps.Keys(h.Holes))
	b = binary.AppendUvarint(b, uint64(len(reasons)))
	for _, reason := range reasons {
		b = appendString(b, reason)
		b = binary.AppendUvarint(b, h.Holes[reason])
	}
	b = appendTime(b, h.JudgedAt)
	return binary.AppendUvarint(b, h.Closure)
}

// headFlag returns flag when set, else zero.
func headFlag(set bool, flag byte) byte {
	if set {
		return flag
	}
	return 0
}

// unmarshal decodes a header row.
func (h *Head) unmarshal(b []byte) error {
	// Read the version, the cycle and the snapshot.
	if len(b) == 0 || b[0] != headVersion {
		return errors.New("unknown cycle header version")
	}
	r := &rowReader{b: b[1:], ok: true}
	h.Cycle = int(r.uvarint()) //nolint:gosec // written from an int.
	h.SnapshotAt = r.time()
	h.SnapshotPacks = int(r.uvarint()) //nolint:gosec // written from an int.
	flags := r.byte()
	h.Catalogued, h.Seeded, h.Walked = flags&headCatalogued != 0, flags&headSeeded != 0, flags&headWalked != 0

	// Read the walk's roots, position and live count.
	if n := r.uvarint(); n != 0 && r.ok {
		h.Roots = make([]string, 0, min(n, uint64(len(r.b))))
		for i := uint64(0); i < n && r.ok; i++ {
			h.Roots = append(h.Roots, r.string())
		}
	}
	h.LogPosition = r.string()
	h.Live = r.uvarint()

	// Read the holes, then the judgment's time.
	if n := r.uvarint(); n != 0 && r.ok {
		h.Holes = make(map[string]uint64)
		for i := uint64(0); i < n && r.ok; i++ {
			reason := r.string()
			h.Holes[reason] = r.uvarint()
		}
	}
	h.JudgedAt = r.time()
	h.Closure = r.uvarint()
	return r.finish("cycle header")
}

// appendString appends a length-prefixed string.
func appendString(b []byte, s string) []byte {
	return append(binary.AppendUvarint(b, uint64(len(s))), s...)
}

// appendTime appends a time as Unix nanoseconds, zero for the zero time.
func appendTime(b []byte, t time.Time) []byte {
	if t.IsZero() {
		return binary.AppendVarint(b, 0)
	}
	return binary.AppendVarint(b, t.UnixNano())
}

// rowReader decodes a row's fields, clearing ok at the first short field.
type rowReader struct {
	// b is the bytes left.
	b []byte
	// ok is cleared once a field ran short.
	ok bool
}

// uvarint reads a uvarint.
func (r *rowReader) uvarint() uint64 {
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.fail()
		return 0
	}
	r.b = r.b[n:]
	return v
}

// varint reads a varint.
func (r *rowReader) varint() int64 {
	v, n := binary.Varint(r.b)
	if n <= 0 {
		r.fail()
		return 0
	}
	r.b = r.b[n:]
	return v
}

// byte reads one byte.
func (r *rowReader) byte() byte {
	if len(r.b) == 0 {
		r.fail()
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

// bytes reads a length-prefixed field, sharing the row's bytes.
func (r *rowReader) bytes() []byte {
	// Read the length, failing past the row's end.
	n := r.uvarint()
	if n > uint64(len(r.b)) {
		r.fail()
		return nil
	}

	// Take the field.
	var v []byte
	v, r.b = r.b[:n:n], r.b[n:]
	return v
}

// string reads a length-prefixed string.
func (r *rowReader) string() string {
	return string(r.bytes())
}

// time reads a time written by appendTime.
func (r *rowReader) time() time.Time {
	v := r.varint()
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(0, v).UTC()
}

// fail marks the row short and drops what is left.
func (r *rowReader) fail() {
	r.ok, r.b = false, nil
}

// finish returns an error naming the row when a field ran short or bytes
// remain past the last field.
func (r *rowReader) finish(row string) error {
	if !r.ok || len(r.b) != 0 {
		return errors.New("malformed " + row)
	}
	return nil
}

// updateHead applies fn to the header in its own transaction.
func updateHead(ctx context.Context, store kvtx.Store, fn func(*Head) error) (*Head, error) {
	// Open a write.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read the header, apply fn, and write it back.
	head, err := readHead(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := fn(head); err != nil {
		return nil, err
	}
	if err := writeHead(ctx, tx, head); err != nil {
		return nil, err
	}
	return head, tx.Commit(ctx)
}

// deletePrefix deletes every key under prefix, a batch per transaction.
func deletePrefix(ctx context.Context, store kvtx.Store, prefix []byte) error {
	for {
		// Collect a batch of keys.
		var keys [][]byte
		tx, err := store.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		errFull := errors.New("full")
		err = tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			keys = append(keys, append([]byte(nil), key...))
			if len(keys) == deleteBatch {
				return errFull
			}
			return nil
		})
		tx.Discard()
		if err != nil && err != errFull {
			return err
		}
		if len(keys) == 0 {
			return nil
		}

		// Delete them.
		wtx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := wtx.Delete(ctx, key); err != nil {
				wtx.Discard()
				return err
			}
		}
		if err := wtx.Commit(ctx); err != nil {
			wtx.Discard()
			return err
		}
		wtx.Discard()
	}
}

// deleteBatch bounds the keys one delete transaction removes.
const deleteBatch = 8192

// NewCycle starts a new cycle, dropping the last one's state and keeping the
// catalog.
func NewCycle(ctx context.Context, store kvtx.Store) (*Head, error) {
	prev, err := ReadHead(ctx, store)
	if err != nil {
		return nil, err
	}
	if err := deletePrefix(ctx, store, []byte(cyclePrefix)); err != nil {
		return nil, err
	}
	return updateHead(ctx, store, func(h *Head) error {
		h.Cycle = prev.Cycle + 1
		return nil
	})
}

// appendUvarints appends each value as a uvarint.
func appendUvarints(b []byte, vs ...uint64) []byte {
	for _, v := range vs {
		b = binary.AppendUvarint(b, v)
	}
	return b
}
