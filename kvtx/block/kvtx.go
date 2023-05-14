package kvtx_block

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	"github.com/pkg/errors"
)

// DefaultKeyValueStoreImpl is the default implementation.
const DefaultKeyValueStoreImpl = KVImplType_KV_IMPL_TYPE_IAVL

// NewKeyValueStore constructs a new key-value store with the given impl.
//
// Pass 0 to use the default implementation.
func NewKeyValueStore(impl KVImplType) *KeyValueStore {
	if impl == 0 {
		impl = DefaultKeyValueStoreImpl
	}
	// all other values are valid empty
	return &KeyValueStore{ImplType: impl}
}

// LoadKeyValueStore loads a key-value store block from a block cursor.
func LoadKeyValueStore(ctx context.Context, bcs *block.Cursor) (*KeyValueStore, error) {
	b, err := block.UnmarshalBlock[*KeyValueStore](ctx, bcs, NewKeyValueStoreBlock)
	if err != nil {
		return nil, err
	}
	if b.GetImplType() == 0 {
		b.ImplType = DefaultKeyValueStoreImpl
	}
	return b, nil
}

// BuildKvTransaction builds a key/value transaction from a KeyValueStore block.
//
// The root ref field in bcs is updated when commit is called.
func BuildKvTransaction(ctx context.Context, bcs *block.Cursor, write bool) (kvtx.BlockTx, error) {
	kvs, err := LoadKeyValueStore(ctx, bcs)
	if err != nil {
		return nil, err
	}

	return kvs.BuildKvTransaction(ctx, bcs, write)
}

// Validate checks if the implementation is in the known set.
func (i KVImplType) Validate() error {
	switch i {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		return nil
	default:
		return NewErrUnknownImpl(i)
	}
}

// Validate performs cursory checks of the KeyValueStore object.
func (k *KeyValueStore) Validate() error {
	if err := k.GetImplType().Validate(); err != nil {
		return err
	}
	if err := k.GetIavlRoot().Validate(); err != nil {
		return errors.Wrap(err, "iavl_root")
	}
	return nil
}

// BuildKvTransaction constructs the kvtx tx from the underlying key value structure.
func (k *KeyValueStore) BuildKvTransaction(ctx context.Context, bcs *block.Cursor, write bool) (kvtx.BlockTx, error) {
	impl := k.GetImplType()
	switch impl {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		treeBcs := bcs.FollowRef(2, k.GetIavlRoot())
		return iavl.NewTx(ctx, treeBcs, nil, write, func(ncs *block.Cursor) {
			bcs.SetRef(2, ncs)
		})
	default:
		return nil, NewErrUnknownImpl(impl)
	}
}
