package block

// ExtractBlockRefs extracts all outgoing BlockRefs from a block by
// walking BlockWithRefs and recursing into BlockWithSubBlocks.
//
// This captures refs at all nesting levels: direct refs on the block,
// refs inside sub-blocks (e.g., BlockRefSlice), and refs inside
// nested sub-blocks.
func ExtractBlockRefs(blk any) ([]*BlockRef, error) {
	if blk == nil {
		return nil, nil
	}

	var refs []*BlockRef

	if bwr, ok := blk.(BlockWithRefs); ok {
		m, err := bwr.GetBlockRefs()
		if err != nil {
			return nil, err
		}
		for _, ref := range m {
			if ref != nil && !ref.GetEmpty() {
				refs = append(refs, ref)
			}
		}
	}

	if bws, ok := blk.(BlockWithSubBlocks); ok {
		subs := bws.GetSubBlocks()
		for _, sub := range subs {
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
