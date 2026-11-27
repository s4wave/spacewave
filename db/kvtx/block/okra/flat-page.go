package kvtx_block_okra

import (
	"bytes"
	"encoding/binary"
	"math"
	"slices"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// ErrMalformedPage is returned when stored page bytes do not decode.
var ErrMalformedPage = errors.New("okra page: malformed encoding")

// flatSpan locates a byte range of a flatPage's buffer. A zero span is empty.
type flatSpan struct {
	// off is the first byte of the range.
	off uint32
	// n is the length of the range.
	n uint32
}

// flatRef locates the hash of a stored block reference. It holds no pointers.
type flatRef struct {
	// hashType is the hash algorithm of the digest.
	hashType int32
	// digest locates the hash bytes.
	digest flatSpan
}

// empty reports whether the reference points to no block, as BlockRef.GetEmpty.
func (r flatRef) empty() bool { return r.digest.n == 0 || r.hashType == 0 }

// flatAnchor is the first of the flatEntry flags.
const (
	flatAnchor uint8 = 1 << iota
	flatValueIsBlob
	flatInlineBlob
	flatInlineRaw
)

// flatEntry is one page entry as offsets into the page buffer. It holds no
// pointers, so a slice of entries is allocated without scan bits and the
// garbage collector never walks it.
type flatEntry struct {
	// key and hash locate the entry key and its hash.
	key, hash flatSpan
	// size is the entry's key count.
	size uint64
	// child and value are the child page and external value references.
	child, value flatRef
	// raw locates the data of an inline raw blob value.
	raw flatSpan
	// childPage is one more than the index in the page's children of the
	// child page embedded in this entry, or zero when the entry embeds none.
	childPage uint32
	// flags holds the flat* bits.
	flags uint8
}

// flatPage is the read-only, flat form of an Okra page. It keeps the page's
// encoded bytes as its only byte storage and indexes every field as an offset
// into them, so a resident page is one noscan byte array, one noscan entry
// array and a header with two pointers, instead of an entry, three references
// and several slices for each key. A page is immutable once decoded: nothing
// writes buf or entries after UnmarshalDecodedBlock returns, and views returned
// from it stay valid for as long as the caller holds them, because the garbage
// collector frees buf only when no view and no cache entry references buf.
type flatPage struct {
	// buf is the encoded page, owned by this page.
	buf []byte
	// entries index buf, one per page entry.
	entries []flatEntry
	// level is the page height. Zero is a leaf.
	level uint32
	// startsAtAnchor is set when the first entry is the anchor.
	startsAtAnchor bool
	// size is the number of keys under the page.
	size uint64
	// lower, upper and pageHash locate the bounds and the page hash.
	lower, upper, pageHash flatSpan
	// children are the child pages embedded in this page's entries. Each
	// shares buf, and is decoded with the page, so a page in the decoded-block
	// cache holds every page its block stores.
	children []flatPage
	// route is the entry index of each embedded page from the block's root
	// page down to this page. It is empty for the block's root page.
	route []uint32
}

// flatPageHeapSize is the allocation size of a flatPage header.
const flatPageHeapSize = 160

// flatEntrySize is the size of one flatEntry.
const flatEntrySize = int(unsafe.Sizeof(flatEntry{}))

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (p *flatPage) DecodedBlockCacheTypeKey() string {
	return "db/kvtx/block/okra.flatPage"
}

// SharesDecodedBlock marks flat pages as shareable between transactions: a
// decoded page is never written.
func (p *flatPage) SharesDecodedBlock() {}

// DecodedHeapSize returns the heap bytes the page retains: its header, its
// buffer and its entry index, each rounded to the allocator size class.
func (p *flatPage) DecodedHeapSize() int {
	if p == nil {
		return 0
	}
	return flatPageHeapSize + heapAlloc(cap(p.buf)) + p.indexHeapSize()
}

// indexHeapSize returns the heap bytes of the page's entry index, route and
// embedded pages, which share the buffer of the block's root page.
func (p *flatPage) indexHeapSize() int {
	n := heapAlloc(cap(p.entries)*flatEntrySize) + heapAlloc(cap(p.route)*4)
	if cap(p.children) != 0 {
		n += heapAlloc(cap(p.children) * flatPageHeapSize)
	}
	for i := range p.children {
		n += p.children[i].indexHeapSize()
	}
	return n
}

// MarshalVT returns a copy of the encoded page.
func (p *flatPage) MarshalVT() ([]byte, error) {
	return bytes.Clone(p.buf), nil
}

// UnmarshalVT decodes the encoded page in data.
func (p *flatPage) UnmarshalVT(data []byte) error {
	return p.UnmarshalDecodedBlock(data)
}

// UnmarshalDecodedBlock indexes the encoded page in data. It copies data into
// the page, so the caller keeps ownership of its slice. Unknown fields are
// skipped and stay in the buffer, so MarshalVT returns them.
func (p *flatPage) UnmarshalDecodedBlock(data []byte) error {
	// Reject pages whose offsets could not fit a span.
	if uint64(len(data)) > math.MaxUint32 {
		return errors.Wrap(ErrMalformedPage, "page larger than 4 GiB")
	}

	// Index the page and its embedded pages over a copy of data.
	buf := bytes.Clone(data)
	next, err := decodeFlatPage(buf, flatSpan{n: uint32(len(buf))}, nil) //nolint:gosec // bounded by the 4 GiB check.
	if err != nil {
		return err
	}
	*p = next
	return nil
}

// decodeFlatPage indexes the page message at s in buf, reached from the
// block's root page through route.
func decodeFlatPage(buf []byte, s flatSpan, route []uint32) (flatPage, error) {
	// Count entries and embedded pages so each index is allocated once at its
	// exact size.
	msg := buf[s.off : s.off+s.n]
	var count, children int
	if err := forEachField(msg, s.off, func(num uint64, wire int, _ uint64, e flatSpan) error {
		if num != 7 || wire != 2 {
			return nil
		}
		count++
		return forEachField(buf[e.off:e.off+e.n], e.off, func(num uint64, wire int, _ uint64, _ flatSpan) error {
			if num == 9 && wire == 2 {
				children++
			}
			return nil
		})
	}); err != nil {
		return flatPage{}, err
	}

	// Index every page field into a fresh page.
	next := flatPage{buf: buf, entries: make([]flatEntry, 0, count), route: route}
	if children != 0 {
		next.children = make([]flatPage, 0, children)
	}
	err := forEachField(msg, s.off, func(num uint64, wire int, v uint64, f flatSpan) error {
		return next.decodeField(num, wire, v, f)
	})
	return next, err
}

// decodeField records one top-level page field.
func (p *flatPage) decodeField(num uint64, wire int, v uint64, s flatSpan) error {
	switch {
	case num == 1 && wire == 0:
		p.level = uint32(v) //nolint:gosec // protobuf uint32 fields truncate.
	case num == 2 && wire == 2:
		p.lower = s
	case num == 3 && wire == 2:
		p.upper = s
	case num == 4 && wire == 0:
		p.startsAtAnchor = v != 0
	case num == 5 && wire == 0:
		p.size = v
	case num == 6 && wire == 2:
		p.pageHash = s
	case num == 7 && wire == 2:
		entry, err := p.decodeEntry(s)
		if err != nil {
			return err
		}
		p.entries = append(p.entries, entry)
	case num >= 1 && num <= 7:
		return errors.Wrapf(ErrMalformedPage, "wire type %d for page field %d", wire, num)
	}
	return nil
}

// decodeEntry indexes the entry message at s.
func (p *flatPage) decodeEntry(s flatSpan) (flatEntry, error) {
	var e flatEntry
	err := forEachField(p.buf[s.off:s.off+s.n], s.off, func(num uint64, wire int, v uint64, f flatSpan) error {
		switch {
		case num == 1 && wire == 0:
			e.flags = setFlag(e.flags, flatAnchor, v != 0)
		case num == 2 && wire == 2:
			e.key = f
		case num == 3 && wire == 2:
			e.hash = f
		case num == 4 && wire == 0:
			e.size = v
		case num == 5 && wire == 2:
			ref, err := p.decodeRef(f)
			e.child = ref
			return err
		case num == 6 && wire == 2:
			ref, err := p.decodeRef(f)
			e.value = ref
			return err
		case num == 7 && wire == 0:
			e.flags = setFlag(e.flags, flatValueIsBlob, v != 0)
		case num == 8 && wire == 2:
			return p.decodeBlob(&e, f)
		case num == 9 && wire == 2:
			return p.decodeChildPage(&e, f)
		case num >= 1 && num <= 9:
			return errors.Wrapf(ErrMalformedPage, "wire type %d for entry field %d", wire, num)
		}
		return nil
	})
	return e, err
}

// decodeRef indexes the block reference message at s.
func (p *flatPage) decodeRef(s flatSpan) (flatRef, error) {
	var ref flatRef
	err := forEachField(p.buf[s.off:s.off+s.n], s.off, func(num uint64, wire int, _ uint64, h flatSpan) error {
		// Only field 1, the hash message, carries the reference.
		if num != 1 || wire != 2 {
			return nil
		}
		var inner flatRef
		err := forEachField(p.buf[h.off:h.off+h.n], h.off, func(num uint64, wire int, v uint64, d flatSpan) error {
			switch {
			case num == 1 && wire == 0:
				inner.hashType = int32(v) //nolint:gosec // protobuf enums truncate to int32.
			case num == 2 && wire == 2:
				inner.digest = d
			}
			return nil
		})
		ref = inner
		return err
	})
	return ref, err
}

// decodeBlob records the inline blob message at s on e.
func (p *flatPage) decodeBlob(e *flatEntry, s flatSpan) error {
	// Mark the entry inline and read the blob's type and raw payload.
	e.flags |= flatInlineBlob
	var blobType uint64
	var raw flatSpan
	err := forEachField(p.buf[s.off:s.off+s.n], s.off, func(num uint64, wire int, v uint64, d flatSpan) error {
		switch {
		case num == 1 && wire == 0:
			blobType = v
		case num == 3 && wire == 2:
			raw = d
		}
		return nil
	})
	if blobType == uint64(blob.BlobType_BlobType_RAW) {
		e.flags |= flatInlineRaw
		e.raw = raw
	}
	return err
}

// decodeChildPage indexes the child page embedded at s as the child of the
// entry p is decoding, which is the next entry of p.
func (p *flatPage) decodeChildPage(e *flatEntry, s flatSpan) error {
	// Accept one child page per entry, decoded on the entry's route.
	if e.childPage != 0 {
		return errors.Wrap(ErrMalformedPage, "repeated child page")
	}
	route := append(slices.Clip(p.route), uint32(len(p.entries))) //nolint:gosec // page entry indexes are bounded by the 4 GiB check.
	child, err := decodeFlatPage(p.buf, s, route)
	if err != nil {
		return err
	}
	p.children = append(p.children, child)
	e.childPage = uint32(len(p.children)) //nolint:gosec // bounded by the entry count.
	return nil
}

// setFlag sets or clears flag in flags.
func setFlag(flags, flag uint8, on bool) uint8 {
	if on {
		return flags | flag
	}
	return flags &^ flag
}

// forEachField calls fn for each field of the message in msg, which starts at
// base in the page buffer. A varint field reports its value in v. A
// length-delimited field reports its payload span in s, relative to the page
// buffer. Fixed-width fields are skipped.
func forEachField(msg []byte, base uint32, fn func(num uint64, wire int, v uint64, s flatSpan) error) error {
	for i := 0; i < len(msg); {
		// Read the field key.
		key, n := binary.Uvarint(msg[i:])
		if n <= 0 {
			return errors.Wrap(ErrMalformedPage, "field key")
		}
		i += n
		num, wire := key>>3, int(key&7)

		// Read the payload of the field's wire type.
		var v uint64
		var s flatSpan
		switch wire {
		case 0:
			v, n = binary.Uvarint(msg[i:])
			if n <= 0 {
				return errors.Wrap(ErrMalformedPage, "varint")
			}
			i += n
		case 1:
			i += 8
		case 2:
			l, n := binary.Uvarint(msg[i:])
			if n <= 0 || l > uint64(len(msg)-i-n) { //nolint:gosec // n > 0 and i+n <= len(msg), so the difference is not negative.
				return errors.Wrap(ErrMalformedPage, "length")
			}
			i += n
			s = flatSpan{off: base + uint32(i), n: uint32(l)} //nolint:gosec // bounded by the 4 GiB check.
			i += int(l)                                       //nolint:gosec // bounded by len(msg).
		case 5:
			i += 4
		default:
			return errors.Wrapf(ErrMalformedPage, "wire type %d", wire)
		}
		if i > len(msg) {
			return errors.Wrap(ErrMalformedPage, "truncated field")
		}

		// Hand the field to the caller.
		if err := fn(num, wire, v, s); err != nil {
			return err
		}
	}
	return nil
}

// view returns the bytes s locates, borrowed from the page buffer.
func (p *flatPage) view(s flatSpan) []byte {
	return p.buf[s.off : s.off+s.n : s.off+s.n]
}

// refOf builds an owned block reference from r, or nil when r is empty.
func (p *flatPage) refOf(r flatRef) *block.BlockRef {
	if r.empty() {
		return nil
	}
	return &block.BlockRef{Hash: &hash.Hash{HashType: hash.HashType(r.hashType), Hash: slices.Clone(p.view(r.digest))}}
}

// Len returns the number of entries.
func (p *flatPage) Len() int { return len(p.entries) }

// GetLevel returns the page height. Zero is a leaf.
func (p *flatPage) GetLevel() uint32 { return p.level }

// isAnchor reports whether entry i is the anchor.
func (p *flatPage) isAnchor(i int) bool { return p.entries[i].flags&flatAnchor != 0 }

// entryKey returns the key of entry i, borrowed from the page buffer.
func (p *flatPage) entryKey(i int) []byte { return p.view(p.entries[i].key) }

// embeddedChild returns entry i's child page when the entry embeds it in this
// page's block, or nil when the child roots its own block.
func (p *flatPage) embeddedChild(i int) *flatPage {
	if n := p.entries[i].childPage; n != 0 {
		return &p.children[n-1]
	}
	return nil
}

// childRef returns an owned copy of entry i's child page reference.
func (p *flatPage) childRef(i int) *block.BlockRef { return p.refOf(p.entries[i].child) }

// valueRef returns an owned copy of entry i's external value reference.
func (p *flatPage) valueRef(i int) *block.BlockRef { return p.refOf(p.entries[i].value) }

// hasInlineBlob reports whether entry i holds its value as an inline Blob.
func (p *flatPage) hasInlineBlob(i int) bool { return p.entries[i].flags&flatInlineBlob != 0 }

// valueIsBlob reports whether entry i's value is a Blob.
func (p *flatPage) valueIsBlob(i int) bool { return p.entries[i].flags&flatValueIsBlob != 0 }

// rawInlineValue returns entry i's raw inline value, borrowed from the page.
func (p *flatPage) rawInlineValue(i int) ([]byte, bool) {
	e := &p.entries[i]
	if e.flags&flatInlineRaw == 0 {
		return nil, false
	}
	return p.view(e.raw), true
}

// containsKey reports whether key falls within the page bounds.
func (p *flatPage) containsKey(key []byte) bool {
	if !p.startsAtAnchor && bytes.Compare(key, p.view(p.lower)) < 0 {
		return false
	}
	if upper := p.view(p.upper); len(upper) != 0 && bytes.Compare(key, upper) >= 0 {
		return false
	}
	return true
}

// searchEntry returns the index of the last entry at or before key, or -1 when
// key sorts before every entry. The anchor sorts before every key.
func (p *flatPage) searchEntry(key []byte) int {
	idx, found := slices.BinarySearchFunc(p.entries, key, func(e flatEntry, key []byte) int {
		if e.flags&flatAnchor != 0 {
			return -1
		}
		return bytes.Compare(p.view(e.key), key)
	})
	if found {
		return idx
	}
	return idx - 1
}

// ValidateDecodedBlock validates the flat form as Page.Validate does.
func (p *flatPage) ValidateDecodedBlock() error {
	// Require entries and a page hash of the configured size.
	if len(p.entries) == 0 || p.pageHash.n != HashSize {
		return errors.Wrapf(
			ErrUnexpectedPageMetadata,
			"level %d entries %d page_hash_len %d",
			p.level, len(p.entries), p.pageHash.n,
		)
	}

	// Validate entry ordering and references while deriving the page size.
	var size uint64
	var prevKey, firstKey []byte
	for idx := range p.entries {
		e := &p.entries[idx]
		if e.hash.n != HashSize {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d hash_len %d", idx, e.hash.n)
		}
		if p.isAnchor(idx) {
			if idx != 0 || e.key.n != 0 {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d invalid anchor", idx)
			}
		} else {
			key := p.entryKey(idx)
			if len(key) == 0 {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d empty key", idx)
			}
			if prevKey != nil && bytes.Compare(prevKey, key) >= 0 {
				return errors.Wrapf(ErrUnsortedEntries, "entry %d", idx)
			}
			if firstKey == nil {
				firstKey = key
			}
			prevKey = key
		}
		if err := p.validateEntryRefs(idx); err != nil {
			return err
		}
		if child := p.embeddedChild(idx); child != nil {
			if err := child.validateChildOf(p, idx); err != nil {
				return errors.Wrapf(err, "entry %d child page", idx)
			}
		}
		size += e.size
	}

	// Check the stored bounds, anchor marker and size against the entries.
	return p.validateBounds(firstKey, size)
}

// validateEntryRefs checks the references permitted for entry idx.
func (p *flatPage) validateEntryRefs(idx int) error {
	// A leaf entry has no child. An internal entry has no value.
	e := &p.entries[idx]
	inline := p.hasInlineBlob(idx)
	if p.level == 0 {
		if !e.child.empty() || e.childPage != 0 {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d leaf child ref", idx)
		}
		if p.isAnchor(idx) && (!e.value.empty() || p.valueIsBlob(idx) || inline) {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d anchor value", idx)
		}
	} else {
		if !e.value.empty() || p.valueIsBlob(idx) || inline {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d internal value", idx)
		}
		if !e.child.empty() && e.childPage != 0 {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d child ref and child page", idx)
		}
		if !e.child.empty() {
			if err := p.refOf(e.child).Validate(false); err != nil {
				return errors.Wrap(err, "child_ref")
			}
		}
	}
	if inline && (!p.valueIsBlob(idx) || !e.value.empty()) {
		return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d ambiguous inline value", idx)
	}
	return nil
}

// validateChildOf validates p as the page embedded in entry idx of parent,
// one level below it.
func (p *flatPage) validateChildOf(parent *flatPage, idx int) error {
	if p.level+1 != parent.level {
		return errors.Wrapf(ErrUnexpectedPageMetadata, "child level %d under level %d", p.level, parent.level)
	}
	return p.ValidateDecodedBlock()
}

// validateBounds checks the page bounds and size against its entries.
func (p *flatPage) validateBounds(firstKey []byte, size uint64) error {
	// The anchor flag, both bounds and the size must match the entries.
	if p.startsAtAnchor != p.isAnchor(0) {
		return errors.Wrap(ErrUnexpectedPageMetadata, "starts_at_anchor mismatch")
	}
	lower := p.view(p.lower)
	if p.startsAtAnchor {
		if len(lower) != 0 && !bytes.Equal(lower, firstKey) {
			return errors.Wrap(ErrUnexpectedPageMetadata, "anchor page lower bound")
		}
	} else if !bytes.Equal(lower, firstKey) {
		return errors.Wrap(ErrUnexpectedPageMetadata, "lower bound mismatch")
	}
	if upper := p.view(p.upper); len(upper) != 0 && firstKey != nil && bytes.Compare(firstKey, upper) >= 0 {
		return errors.Wrap(ErrUnexpectedPageMetadata, "upper bound before first key")
	}
	if p.size != size {
		return errors.Wrapf(ErrUnexpectedPageMetadata, "size %d != %d", p.size, size)
	}
	return nil
}

// ApplyBlockRef rejects the change: a flat page is never written.
func (p *flatPage) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	return errors.New("okra page: flat page is read-only")
}

// GetBlockRefs returns the child and value references by entry ref ID.
func (p *flatPage) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	// A nil page holds no references.
	if p == nil {
		return nil, nil
	}

	// Size one arena for every reference, so the page costs three allocations
	// for its references instead of three per reference.
	var n, digestBytes int
	for i := range p.entries {
		for _, r := range [...]flatRef{p.entries[i].child, p.entries[i].value} {
			if !r.empty() {
				n, digestBytes = n+1, digestBytes+int(r.digest.n)
			}
		}
	}
	refs := make(map[uint32]*block.BlockRef, n)
	arena := block.NewRefArena(n, digestBytes)
	for i := range p.entries {
		e := &p.entries[i]
		if !e.child.empty() {
			refs[entryChildRefID(i)] = arena.Ref(hash.HashType(e.child.hashType), p.view(e.child.digest))
		}
		if !e.value.empty() {
			refs[entryValueRefID(i)] = arena.Ref(hash.HashType(e.value.hashType), p.view(e.value.digest))
		}
	}
	return refs, nil
}

// GetBlockRefCtor returns the constructor for the page at a child ref ID.
func (p *flatPage) GetBlockRefCtor(id uint32) block.Ctor {
	if _, ok := entryIndexFromChildRefID(id); ok {
		return newFlatPageBlock
	}
	return nil
}

// RangeSubBlocks calls fn with each embedded child page, so a walk of the
// block's refs reaches the refs the embedded pages hold.
func (p *flatPage) RangeSubBlocks(fn func(id uint32, subBlock block.SubBlock)) {
	if p == nil {
		return
	}
	for i := range p.entries {
		if child := p.embeddedChild(i); child != nil {
			fn(entryChildPageID(i), child)
		}
	}
}

// GetSubBlocks returns the embedded child pages by child page ID.
func (p *flatPage) GetSubBlocks() map[uint32]block.SubBlock {
	return block.CollectSubBlocks(p)
}

// GetSubBlockCtor returns the embedded child page at id, or nil when the
// entry embeds none. It never creates one: a flat page is read-only.
func (p *flatPage) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	// Return the existing embedded page and never construct one.
	idx, ok := entryIndexFromChildPageID(id)
	if !ok || idx >= len(p.entries) {
		return nil
	}
	child := p.embeddedChild(idx)
	if child == nil {
		return nil
	}
	return func(bool) block.SubBlock { return child }
}

// ApplySubBlock rejects the change: a flat page is never written.
func (p *flatPage) ApplySubBlock(id uint32, next block.SubBlock) error {
	return errors.New("okra page: flat page is read-only")
}

// IsNil reports whether the page is nil.
func (p *flatPage) IsNil() bool { return p == nil }

// BlockAliasIdentity returns the page's in-memory identity. Each page,
// embedded ones included, holds its own view of the block bytes.
func (p *flatPage) BlockAliasIdentity() *block.AliasIdentityToken { return &p.buf }

// newFlatPageBlock constructs an empty flat page.
func newFlatPageBlock() block.Block { return &flatPage{} }

// _ asserts the flat page block contracts.
var (
	_ block.Block                   = (*flatPage)(nil)
	_ block.BlockWithRefs           = (*flatPage)(nil)
	_ block.BlockWithSubBlocks      = (*flatPage)(nil)
	_ block.BlockWithSubBlockRange  = (*flatPage)(nil)
	_ block.SubBlock                = (*flatPage)(nil)
	_ block.DecodedBlockShareable   = (*flatPage)(nil)
	_ block.DecodedBlockValidator   = (*flatPage)(nil)
	_ block.DecodedBlockUnmarshaler = (*flatPage)(nil)
	_ block.DecodedBlockHeapSizer   = (*flatPage)(nil)
)
