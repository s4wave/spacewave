package kvtx_block_okra

import (
	"bytes"
	"iter"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

const (
	entryChildRefIDBase = 1000000
	entryValueRefIDBase = 2000000
)

// NewPageBlock constructs a new Okra page block.
func NewPageBlock() block.Block {
	return &Page{}
}

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (p *Page) DecodedBlockCacheTypeKey() string {
	return "db/kvtx/block/okra.Page"
}

func entryChildRefID(index int) uint32 {
	return entryChildRefIDBase + uint32(index) //nolint:gosec // page entry indexes are bounded by the in-memory page representation.
}

func entryValueRefID(index int) uint32 {
	return entryValueRefIDBase + uint32(index) //nolint:gosec // page entry indexes are bounded by the in-memory page representation.
}

func entryIndexFromChildRefID(id uint32) (int, bool) {
	if id < entryChildRefIDBase || id >= entryValueRefIDBase {
		return 0, false
	}
	return int(id - entryChildRefIDBase), true
}

func entryIndexFromValueRefID(id uint32) (int, bool) {
	if id < entryValueRefIDBase {
		return 0, false
	}
	return int(id - entryValueRefIDBase), true
}

// Validate performs cursory checks of the packed page object.
func (p *Page) Validate() error {
	// Require page entries and a hash of the configured Okra size.
	entries := p.GetEntries()
	if len(entries) == 0 || len(p.GetPageHash()) != HashSize {
		return errors.Wrapf(
			ErrUnexpectedPageMetadata,
			"level %d entries %d page_hash_len %d",
			p.GetLevel(),
			len(entries),
			len(p.GetPageHash()),
		)
	}

	// Validate entry ordering and references while deriving the page bounds and size.
	var size uint64
	var prevKey []byte
	var firstKey []byte
	for idx, ent := range entries {
		if ent == nil || len(ent.GetHash()) != HashSize {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d hash_len %d", idx, len(ent.GetHash()))
		}
		if ent.GetAnchor() {
			if idx != 0 || len(ent.GetKey()) != 0 {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d invalid anchor", idx)
			}
		} else {
			key := ent.GetKey()
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

		// Check the references permitted at the page level and accumulate entry size.
		if p.GetLevel() == 0 {
			if !ent.GetChildRef().GetEmpty() {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d leaf child ref", idx)
			}
			if ent.GetAnchor() && (!ent.GetValueRef().GetEmpty() || ent.GetValueIsBlob() || ent.GetValueBlob() != nil) {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d anchor value", idx)
			}
		} else {
			if !ent.GetValueRef().GetEmpty() || ent.GetValueIsBlob() || ent.GetValueBlob() != nil {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d internal value", idx)
			}
			if !ent.GetChildRef().GetEmpty() {
				if err := ent.GetChildRef().Validate(false); err != nil {
					return errors.Wrap(err, "child_ref")
				}
			}
		}
		if ent.GetValueBlob() != nil && (!ent.GetValueIsBlob() || !ent.GetValueRef().GetEmpty()) {
			return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d ambiguous inline value", idx)
		}
		size += ent.GetSize()
	}

	// Check the stored page bounds, anchor marker and size against its entries.
	if p.GetStartsAtAnchor() != entries[0].GetAnchor() {
		return errors.Wrap(ErrUnexpectedPageMetadata, "starts_at_anchor mismatch")
	}
	if p.GetStartsAtAnchor() {
		if len(p.GetLowerBound()) != 0 && !bytes.Equal(p.GetLowerBound(), firstKey) {
			return errors.Wrap(ErrUnexpectedPageMetadata, "anchor page lower bound")
		}
	} else if !bytes.Equal(p.GetLowerBound(), firstKey) {
		return errors.Wrap(ErrUnexpectedPageMetadata, "lower bound mismatch")
	}
	if upper := p.GetUpperBound(); len(upper) != 0 && firstKey != nil &&
		bytes.Compare(firstKey, upper) >= 0 {
		return errors.Wrap(ErrUnexpectedPageMetadata, "upper bound before first key")
	}
	if p.GetSize() != size {
		return errors.Wrapf(ErrUnexpectedPageMetadata, "size %d != %d", p.GetSize(), size)
	}
	return nil
}

// ValidateDecodedBlock validates the page once when a cursor decodes it, so a
// page served from the decoded-block cache was validated when it was decoded.
func (p *Page) ValidateDecodedBlock() error {
	return p.Validate()
}

// ApplyBlockRef applies a ref change with a field id.
func (p *Page) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if idx, ok := entryIndexFromChildRefID(id); ok {
		if idx >= len(p.Entries) || p.Entries[idx] == nil {
			return ErrUnexpectedEntryMetadata
		}
		p.Entries[idx].ChildRef = ptr
		return nil
	}
	if idx, ok := entryIndexFromValueRefID(id); ok {
		if idx >= len(p.Entries) || p.Entries[idx] == nil {
			return ErrUnexpectedEntryMetadata
		}
		p.Entries[idx].ValueRef = ptr
		return nil
	}
	return nil
}

// EntryRef is a ref that a page's block stores for one of its entries.
type EntryRef struct {
	// ID is the ref's id within the block.
	ID uint32
	// Entry is the entry that holds the ref.
	Entry *Entry
	// Ref is the stored ref, never empty.
	Ref *block.BlockRef
	// Child is set for a child page's ref and unset for a value's.
	Child bool
}

// Refs yields each ref the page's block stores, in entry order, an entry's
// child before its value. Every walker of a page's edges, such as garbage
// collection, recovery and the typed walks, reads them here, so the refs a
// walker sees are exactly the refs the block stores.
func (p *Page) Refs() iter.Seq[EntryRef] {
	return func(yield func(EntryRef) bool) {
		for idx, ent := range p.GetEntries() {
			if ref := ent.GetChildRef(); !ref.GetEmpty() {
				if !yield(EntryRef{ID: entryChildRefID(idx), Entry: ent, Ref: ref, Child: true}) {
					return
				}
			}
			if ref := ent.GetValueRef(); !ref.GetEmpty() {
				if !yield(EntryRef{ID: entryValueRefID(idx), Entry: ent, Ref: ref}) {
					return
				}
			}
		}
	}
}

// GetBlockRefs returns all block references by ID.
func (p *Page) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if p == nil {
		return nil, nil
	}
	refs := make(map[uint32]*block.BlockRef)
	for r := range p.Refs() {
		refs[r.ID] = r.Ref
	}
	return refs, nil
}

// TreeValueRef reports whether the ref with id is an entry's stored value,
// and whether the entry marks it a Blob.
func (p *Page) TreeValueRef(id uint32) (value, isBlob bool) {
	idx, ok := entryIndexFromValueRefID(id)
	if !ok || idx >= len(p.GetEntries()) || p.GetEntries()[idx].GetValueRef().GetEmpty() {
		return false, false
	}
	return true, p.GetEntries()[idx].GetValueIsBlob()
}

// HasChild reports whether entry index stores a child page.
func (p *Page) HasChild(index int) bool {
	entries := p.GetEntries()
	return index >= 0 && index < len(entries) && !entries[index].GetChildRef().GetEmpty()
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
func (p *Page) GetBlockRefCtor(id uint32) block.Ctor {
	if _, ok := entryIndexFromChildRefID(id); ok {
		return NewPageBlock
	}
	return nil
}

// FollowChild follows an internal entry child page.
func (p *Page) FollowChild(cursor *block.Cursor, index int) *block.Cursor {
	if p == nil || index < 0 || index >= len(p.GetEntries()) {
		return nil
	}
	ent := p.GetEntries()[index]
	return cursor.FollowRef(entryChildRefID(index), ent.GetChildRef())
}

// FollowValue follows a leaf entry value.
func (p *Page) FollowValue(cursor *block.Cursor, index int) *block.Cursor {
	if p == nil || index < 0 || index >= len(p.GetEntries()) {
		return nil
	}
	ent := p.GetEntries()[index]
	if ent.GetValueBlob() != nil {
		return cursor.FollowSubBlock(entryValueRefID(index))
	}
	return cursor.FollowRef(entryValueRefID(index), ent.GetValueRef())
}

// GetSubBlocks exposes inline Blobs through the ordinary cursor lifecycle.
func (p *Page) GetSubBlocks() map[uint32]block.SubBlock {
	return block.CollectSubBlocks(p)
}

// RangeSubBlocks calls fn with each entry's inline value Blob.
func (p *Page) RangeSubBlocks(fn func(id uint32, subBlock block.SubBlock)) {
	for index, entry := range p.GetEntries() {
		if blob := entry.GetValueBlob(); blob != nil {
			fn(entryValueRefID(index), blob)
		}
	}
}

// SharesDecodedBlock marks decoded pages as shareable between transactions:
// readers never mutate a page, and writers build new pages.
// FollowValue follows only existing inline Blobs, so traversal never creates
// a field on a shared page.
func (p *Page) SharesDecodedBlock() {}

// GetSubBlockCtor returns the constructor for an inline Blob value.
func (p *Page) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	index, ok := entryIndexFromValueRefID(id)
	if !ok || index >= len(p.Entries) || p.Entries[index] == nil {
		return nil
	}
	return blob.NewBlobSubBlockCtor(&p.Entries[index].ValueBlob)
}

// ApplySubBlock applies an inline value update to its page entry.
func (p *Page) ApplySubBlock(id uint32, next block.SubBlock) error {
	index, ok := entryIndexFromValueRefID(id)
	if !ok || index >= len(p.Entries) || p.Entries[index] == nil {
		return ErrUnexpectedEntryMetadata
	}
	return block.ApplySubBlock(&p.Entries[index].ValueBlob, next)
}

// Verify the Page block contracts.
var (
	_ block.Block                 = (*Page)(nil)
	_ block.BlockWithRefs         = (*Page)(nil)
	_ block.BlockWithSubBlocks    = (*Page)(nil)
	_ block.DecodedBlockShareable = (*Page)(nil)
	_ block.DecodedBlockValidator = (*Page)(nil)
)

// _ is a type assertion
var _ block.BlockWithSubBlockRange = (*Page)(nil)
