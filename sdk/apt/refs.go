package s4wave_apt

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// Invariant: every block an Apt record names is a graph edge of that record.
// Holds and collection keep only what an edge reaches from a held root, so a
// ref field these methods leave out names a block that is dropped once the
// stage that put it is released. A BlockRef field is listed by GetBlockRefs;
// an ObjectRef field is a sub-block, and its own GetBlockRefs lists its root
// when it is in the same bucket.

// ApplyBlockRef applies a ref change with a field id.
func (p *AptPackage) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 11 {
		p.DebRef = ptr.Clone()
	}
	return nil
}

// GetBlockRefs returns the .deb payload ref as field 11.
func (p *AptPackage) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{11: p.GetDebRef()}, nil
}

// GetBlockRefCtor returns nil: the .deb payload is raw bytes.
func (p *AptPackage) GetBlockRefCtor(id uint32) block.Ctor {
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
func (r *AptRepository) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 5 {
		r.SigningKeyRef = ptr.Clone()
	}
	return nil
}

// GetBlockRefs returns the signing key ref as field 5.
func (r *AptRepository) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{5: r.GetSigningKeyRef()}, nil
}

// GetBlockRefCtor returns nil: the signing key is raw bytes.
func (r *AptRepository) GetBlockRefCtor(id uint32) block.Ctor {
	return nil
}

// ApplySubBlock applies the index ref sub-block at field 6.
func (r *AptRepository) ApplySubBlock(id uint32, next block.SubBlock) error {
	// Only field 6 is a sub-block; decode it as an object ref.
	if id != 6 {
		return nil
	}
	ref, err := applyObjectRefSubBlock(next)
	if err != nil {
		return err
	}
	r.IndexRef = ref
	return nil
}

// GetSubBlocks returns the index ref as field 6.
func (r *AptRepository) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{6: r.GetIndexRef()}
}

// GetSubBlockCtor returns the index ref constructor for field 6.
func (r *AptRepository) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id != 6 {
		return nil
	}
	return func(create bool) block.SubBlock {
		if r.IndexRef == nil && create {
			r.IndexRef = &bucket.ObjectRef{}
		}
		return r.IndexRef
	}
}

// ApplySubBlock applies the source ref sub-block at field 2.
func (s *AptBuildSpec) ApplySubBlock(id uint32, next block.SubBlock) error {
	// Only field 2 is a sub-block; decode it as an object ref.
	if id != 2 {
		return nil
	}
	ref, err := applyObjectRefSubBlock(next)
	if err != nil {
		return err
	}
	s.SourceRef = ref
	return nil
}

// GetSubBlocks returns the source ref as field 2.
func (s *AptBuildSpec) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{2: s.GetSourceRef()}
}

// GetSubBlockCtor returns the source ref constructor for field 2.
func (s *AptBuildSpec) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id != 2 {
		return nil
	}
	return func(create bool) block.SubBlock {
		if s.SourceRef == nil && create {
			s.SourceRef = &bucket.ObjectRef{}
		}
		return s.SourceRef
	}
}

// applyObjectRefSubBlock converts a sub-block change to an ObjectRef field.
func applyObjectRefSubBlock(next block.SubBlock) (*bucket.ObjectRef, error) {
	if next == nil || next.IsNil() {
		return nil, nil
	}
	ref, ok := next.(*bucket.ObjectRef)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return ref, nil
}

// _ is a type assertion
var (
	_ block.BlockWithRefs      = (*AptPackage)(nil)
	_ block.BlockWithRefs      = (*AptRepository)(nil)
	_ block.BlockWithSubBlocks = (*AptRepository)(nil)
	_ block.BlockWithSubBlocks = (*AptBuildSpec)(nil)
)
