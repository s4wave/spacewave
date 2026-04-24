package git_block

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	block_kvtx "github.com/aperturerobotics/hydra/kvtx/block"
	gconfig "github.com/go-git/go-git/v6/config"
	"github.com/pkg/errors"
)

// NewRepo constructs a new repo with default settings.
func NewRepo() *Repo {
	return &Repo{
		ReferencesStore: &ReferencesStore{
			KvtxRoot: block_kvtx.NewKeyValueStore(0),
		},
		EncodedObjectStore: &EncodedObjectStore{
			KvtxRoot:         block_kvtx.NewKeyValueStore(0),
			PackfileKvtxRoot: block_kvtx.NewKeyValueStore(0),
		},
		ModuleReferencesStore: &ModuleReferencesStore{
			KvtxRoot: block_kvtx.NewKeyValueStore(0),
		},
	}
}

// NewRepoBlock builds a new repo root block.
func NewRepoBlock() block.Block {
	return &Repo{}
}

// UnmarshalRepo unmarshals a repo from a cursor.
// If empty, returns nil, nil
func UnmarshalRepo(ctx context.Context, bcs *block.Cursor) (*Repo, error) {
	return block.UnmarshalBlock[*Repo](ctx, bcs, NewRepoBlock)
}

// Validate performs cursory checks on the repo block.
func (r *Repo) Validate() error {
	if gconf := r.GetGitConfig(); len(gconf) != 0 {
		nc := gconfig.NewConfig()
		if err := nc.Unmarshal([]byte(gconf)); err != nil {
			return errors.Wrap(err, "git_config")
		}
	}
	if err := r.GetReferencesStore().Validate(); err != nil {
		return errors.Wrap(err, "references_store")
	}
	if err := r.GetModuleReferencesStore().Validate(); err != nil {
		return errors.Wrap(err, "module_references_store")
	}
	if err := r.GetEncodedObjectStore().Validate(); err != nil {
		return errors.Wrap(err, "encoded_object_store")
	}
	// allow nil reference
	if err := r.GetShallowRefsStoreRef().Validate(true); err != nil {
		return errors.Wrap(err, "shallow_refs_store_ref")
	}
	return nil
}

// FollowReferencesStore returns the repo references sub-block.
func (r *Repo) FollowReferencesStore(ctx context.Context, bcs *block.Cursor) (*ReferencesStore, *block.Cursor, error) {
	cs := bcs.FollowSubBlock(1)
	v, err := cs.Unmarshal(ctx, NewReferencesStoreBlock)
	if err != nil {
		return nil, nil, err
	}
	nv, ok := v.(*ReferencesStore)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return nv, cs, nil
}

// FollowModuleReferencesStore returns the submodule references sub-block.
func (r *Repo) FollowModuleReferencesStore(ctx context.Context, bcs *block.Cursor) (*ModuleReferencesStore, *block.Cursor, error) {
	cs := bcs.FollowSubBlock(2)
	v, err := cs.Unmarshal(ctx, NewModuleReferencesStoreBlock)
	if err != nil {
		return nil, nil, err
	}
	nv, ok := v.(*ModuleReferencesStore)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return nv, cs, nil
}

// FollowEncodedObjectStore returns the encoded object store sub-block.
func (r *Repo) FollowEncodedObjectStore(ctx context.Context, bcs *block.Cursor) (*EncodedObjectStore, *block.Cursor, error) {
	cs := bcs.FollowSubBlock(3)
	v, err := cs.Unmarshal(ctx, NewEncodedObjectStoreBlock)
	if err != nil {
		return nil, nil, err
	}
	nv, ok := v.(*EncodedObjectStore)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return nv, cs, nil
}

// FollowShallowRefsStore returns the shallow refs store block.
func (r *Repo) FollowShallowRefsStore(ctx context.Context, bcs *block.Cursor) (*ShallowRefsStore, *block.Cursor, error) {
	cs := bcs.FollowRef(4, r.GetShallowRefsStoreRef())
	v, err := cs.Unmarshal(ctx, NewShallowRefsStoreBlock)
	if err != nil {
		return nil, nil, err
	}
	if v == nil {
		v = &ShallowRefsStore{}
		cs.SetBlock(v, true)
	}
	nv, ok := v.(*ShallowRefsStore)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return nv, cs, nil
}

// MarshalBlock marshals the block to binary.
func (r *Repo) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Repo) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *Repo) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*ReferencesStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.ReferencesStore = v
	case 2:
		v, ok := next.(*ModuleReferencesStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.ModuleReferencesStore = v
	case 3:
		v, ok := next.(*EncodedObjectStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.EncodedObjectStore = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *Repo) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetReferencesStore()
	v[2] = r.GetModuleReferencesStore()
	v[3] = r.GetEncodedObjectStore()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *Repo) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := r.GetReferencesStore()
			if create && v == nil {
				r.ReferencesStore = &ReferencesStore{}
				v = r.ReferencesStore
			}
			return v
		}
	case 2:
		return func(create bool) block.SubBlock {
			v := r.GetModuleReferencesStore()
			if create && v == nil {
				r.ModuleReferencesStore = &ModuleReferencesStore{}
				v = r.ModuleReferencesStore
			}
			return v
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := r.GetEncodedObjectStore()
			if create && v == nil {
				r.EncodedObjectStore = &EncodedObjectStore{}
				v = r.EncodedObjectStore
			}
			return v
		}
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *Repo) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 4:
		r.ShallowRefsStoreRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *Repo) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r == nil {
		return nil, nil
	}
	m := make(map[uint32]*block.BlockRef)
	m[4] = r.GetShallowRefsStoreRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *Repo) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 4:
		return NewShallowRefsStoreBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Repo)(nil))
	_ block.BlockWithSubBlocks = ((*Repo)(nil))
	_ block.BlockWithRefs      = ((*Repo)(nil))
)
