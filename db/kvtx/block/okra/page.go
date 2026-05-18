package kvtx_block_okra

import (
	"bytes"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	entryChildRefIDBase = 1000000
	entryValueRefIDBase = 2000000
)

// NewPageBlock constructs a new Okra page block.
func NewPageBlock() block.Block {
	return &Page{}
}

func entryChildRefID(index int) uint32 {
	return entryChildRefIDBase + uint32(index)
}

func entryValueRefID(index int) uint32 {
	return entryValueRefIDBase + uint32(index)
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

		if p.GetLevel() == 0 {
			if !ent.GetChildRef().GetEmpty() {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d leaf child ref", idx)
			}
			if ent.GetAnchor() && (!ent.GetValueRef().GetEmpty() || ent.GetValueIsBlob()) {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d anchor value", idx)
			}
		} else {
			if !ent.GetValueRef().GetEmpty() || ent.GetValueIsBlob() {
				return errors.Wrapf(ErrUnexpectedEntryMetadata, "entry %d internal value", idx)
			}
			if !ent.GetChildRef().GetEmpty() {
				if err := ent.GetChildRef().Validate(false); err != nil {
					return errors.Wrap(err, "child_ref")
				}
			}
		}
		size += ent.GetSize()
	}

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

// MarshalBlock marshals the block to binary.
func (p *Page) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (p *Page) UnmarshalBlock(data []byte) error {
	return p.UnmarshalVT(data)
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

// GetBlockRefs returns all block references by ID.
func (p *Page) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if p == nil {
		return nil, nil
	}
	refs := make(map[uint32]*block.BlockRef)
	for idx, ent := range p.GetEntries() {
		if ent == nil {
			continue
		}
		if !ent.GetChildRef().GetEmpty() {
			refs[entryChildRefID(idx)] = ent.GetChildRef()
		}
		if !ent.GetValueRef().GetEmpty() {
			refs[entryValueRefID(idx)] = ent.GetValueRef()
		}
	}
	return refs, nil
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
	return cursor.FollowRef(entryValueRefID(index), ent.GetValueRef())
}

// _ is a type assertion
var (
	_ block.Block         = ((*Page)(nil))
	_ block.BlockWithRefs = ((*Page)(nil))
)
