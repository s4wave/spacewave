package block

import (
	"maps"
	"slices"
)

// ExtractBlockRefs extracts all outgoing BlockRefs from a block by
// walking BlockWithRefs and recursing into BlockWithSubBlocks.
//
// The traversal captures refs at all nesting levels: direct refs on the block,
// refs inside sub-blocks (e.g., BlockRefSlice), and refs inside
// nested sub-blocks. References and sub-blocks are visited in field-ID order,
// preserving sequence order independently of Go map iteration.
func ExtractBlockRefs(blk any) ([]*BlockRef, error) {
	// A missing block contributes no references.
	if blk == nil {
		return nil, nil
	}

	// Direct references retain their declared field order.
	var refs []*BlockRef
	if bwr, ok := blk.(BlockWithRefs); ok {
		m, err := bwr.GetBlockRefs()
		if err != nil {
			return nil, err
		}
		for _, id := range slices.Sorted(maps.Keys(m)) {
			ref := m[id]
			if ref != nil && !ref.GetEmpty() {
				refs = append(refs, ref)
			}
		}
	}

	// Inline containers contribute their nested references in field order.
	if bws, ok := blk.(BlockWithSubBlocks); ok {
		subs := bws.GetSubBlocks()
		for _, id := range slices.Sorted(maps.Keys(subs)) {
			sub := subs[id]
			if sub != nil && !sub.IsNil() {
				subRefs, err := ExtractBlockRefs(sub)
				if err != nil {
					return nil, err
				}
				refs = append(refs, subRefs...)
			}
		}
	}

	return refs, nil
}
