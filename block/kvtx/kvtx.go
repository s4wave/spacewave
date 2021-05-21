package block_kvtx

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/iavl"
	"github.com/aperturerobotics/hydra/kvtx"
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
func LoadKeyValueStore(bcs *block.Cursor) (*KeyValueStore, error) {
	b, err := bcs.Unmarshal(NewKeyValueStoreBlock)
	if err != nil {
		return nil, err
	}
	if b == nil {
		b = &KeyValueStore{}
		bcs.SetBlock(b, false)
	}
	v, ok := b.(*KeyValueStore)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return v, nil
}

// BuildKvTransaction builds a key/value transaction from a KeyValueStore block.
func BuildKvTransaction(bcs *block.Cursor, write bool) (kvtx.BlockTx, error) {
	kvs, err := LoadKeyValueStore(bcs)
	if err != nil {
		return nil, err
	}

	return kvs.BuildKvTransaction(bcs, write)
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

// BuildKvTransaction constructs the kvtx tx from the underlying key value structure.
func (k *KeyValueStore) BuildKvTransaction(bcs *block.Cursor, write bool) (kvtx.BlockTx, error) {
	impl := k.GetImplType()
	switch impl {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		treeBcs := bcs.FollowRef(2, k.GetIavlRoot())
		return iavl.NewTx(treeBcs, write, func(ncs *block.Cursor) {
			bcs.SetRef(2, ncs)
		})
	default:
		return nil, NewErrUnknownImpl(impl)
	}
}

// NewKeyValueStoreSubBlockCtor returns the sub-block constructor.
func NewKeyValueStoreSubBlockCtor(r **KeyValueStore) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &KeyValueStore{}
			*r = v
		}
		return v
	}
}
