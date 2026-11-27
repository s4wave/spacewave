package block

import (
	"cmp"
	"slices"
)

// ExtractBlockRefs extracts all outgoing BlockRefs from a block by
// walking BlockWithRefs and recursing into BlockWithSubBlocks.
//
// The traversal captures refs at all nesting levels: direct refs on the block,
// refs inside sub-blocks (e.g., BlockRefSlice), and refs inside
// nested sub-blocks. References and sub-blocks are visited in field-ID order,
// preserving sequence order independently of Go map iteration.
//
// A walk shares one result slice and one sub-block stack across its levels,
// and ranges a BlockWithSubBlockRange instead of collecting its map, so a
// block costs the allocations of its own GetBlockRefs and of the result's
// growth, not a map and a sorted key slice per level.
func ExtractBlockRefs(blk any) ([]*BlockRef, error) {
	// A missing block contributes no references.
	if blk == nil {
		return nil, nil
	}
	x := &refExtractor{}
	err := x.extract(blk)
	if err != nil {
		return nil, err
	}
	return x.refs, nil
}

// idSubBlock is a sub-block with its field ID.
type idSubBlock struct {
	// id is the sub-block's field ID.
	id uint32
	// sub is the sub-block.
	sub SubBlock
}

// refExtractor is the state of one ExtractBlockRefs walk.
type refExtractor struct {
	// refs are the references found so far, in walk order.
	refs []*BlockRef
	// subs is a stack of the sub-blocks of the blocks being walked. Each
	// level pushes its own above those of the levels below it, walks them,
	// and truncates the stack back.
	subs []idSubBlock
	// push pushes a ranged sub-block onto subs. It is built once per walk,
	// since a method value passed through an interface allocates.
	push func(id uint32, sub SubBlock)
}

// extract appends the references of blk and of its sub-blocks to x.refs.
func (x *refExtractor) extract(blk any) error {
	// Direct references retain their declared field order.
	if bwr, ok := blk.(BlockWithRefs); ok {
		m, err := bwr.GetBlockRefs()
		if err != nil {
			return err
		}
		var idBuf [8]uint32
		ids := idBuf[:0]
		for id, ref := range m {
			if ref != nil && !ref.GetEmpty() {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		for _, id := range ids {
			x.refs = append(x.refs, m[id])
		}
	}

	// Push the inline sub-blocks, ranging them when the block can.
	start := len(x.subs)
	switch b := blk.(type) {
	case BlockWithSubBlockRange:
		if x.push == nil {
			x.push = func(id uint32, sub SubBlock) {
				x.subs = append(x.subs, idSubBlock{id: id, sub: sub})
			}
		}
		b.RangeSubBlocks(x.push)
	case BlockWithSubBlocks:
		for id, sub := range b.GetSubBlocks() {
			x.subs = append(x.subs, idSubBlock{id: id, sub: sub})
		}
	}

	// Walk them in field order. A deeper level pushes above end and
	// truncates back to it, so each entry is read again by index.
	end := len(x.subs)
	slices.SortFunc(x.subs[start:end], func(a, b idSubBlock) int {
		return cmp.Compare(a.id, b.id)
	})
	for i := start; i != end; i++ {
		sub := x.subs[i].sub
		if sub == nil || sub.IsNil() {
			continue
		}
		if err := x.extract(sub); err != nil {
			return err
		}
	}
	x.subs = x.subs[:start]
	return nil
}
