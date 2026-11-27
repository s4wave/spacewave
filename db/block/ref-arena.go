package block

import "github.com/s4wave/spacewave/net/hash"

// RefArena hands out owned block references from three contiguous
// allocations, so copying a page's n references costs three allocations
// instead of 3n. The references share the arena's backing arrays and keep them
// alive together. An arena suits a set that lives and dies as one, such as the
// references of one decoded block.
type RefArena struct {
	refs    []BlockRef
	hashes  []hash.Hash
	digests []byte
}

// NewRefArena sizes an arena for n references holding digestBytes in total.
// References beyond that size are allocated one at a time.
func NewRefArena(n, digestBytes int) RefArena {
	return RefArena{
		refs:    make([]BlockRef, n),
		hashes:  make([]hash.Hash, n),
		digests: make([]byte, digestBytes),
	}
}

// Ref returns an owned reference to a copy of digest under hashType.
func (a *RefArena) Ref(hashType hash.HashType, digest []byte) *BlockRef {
	// Fall back to a single allocation once the arena is used up.
	if len(a.refs) == 0 || len(a.hashes) == 0 || len(a.digests) < len(digest) {
		return &BlockRef{Hash: &hash.Hash{HashType: hashType, Hash: append([]byte(nil), digest...)}}
	}

	// Take the next slots, capping the digest at its length so an append copies
	// instead of overwriting the next reference's digest.
	ref, h := &a.refs[0], &a.hashes[0]
	a.refs, a.hashes = a.refs[1:], a.hashes[1:]
	h.HashType, h.Hash = hashType, a.digests[:len(digest):len(digest)]
	copy(h.Hash, digest)
	a.digests = a.digests[len(digest):]
	ref.Hash = h
	return ref
}

// Clone returns an owned copy of ref, or nil for nil. A reference carrying
// unknown fields is cloned in full, since the arena copies only the known ones.
func (a *RefArena) Clone(ref *BlockRef) *BlockRef {
	// Copy only a reference the arena can copy exactly.
	if ref == nil {
		return nil
	}
	h := ref.GetHash()
	if h == nil || len(ref.unknownFields) != 0 {
		return ref.Clone()
	}

	// Reject the copy when the hash carried unknown fields the arena dropped.
	out := a.Ref(h.GetHashType(), h.GetHash())
	if out.Hash.SizeVT() != h.SizeVT() {
		return ref.Clone()
	}
	return out
}
