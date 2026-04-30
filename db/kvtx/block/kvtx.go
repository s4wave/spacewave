package kvtx_block

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	trace "github.com/s4wave/spacewave/db/traceutil"
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
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block/load-key-value-store")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/load-key-value-store/unmarshal")
	b, err := block.UnmarshalBlock[*KeyValueStore](taskCtx, bcs, NewKeyValueStoreBlock)
	subtask.End()
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
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block/build-kv-transaction")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/build-kv-transaction/load-key-value-store")
	kvs, err := LoadKeyValueStore(taskCtx, bcs)
	subtask.End()
	if err != nil {
		return nil, err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/kvtx-block/build-kv-transaction/build-impl")
	defer subtask.End()
	return kvs.BuildKvTransaction(taskCtx, bcs, write)
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
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block/key-value-store/build-kv-transaction")
	defer task.End()

	impl := k.GetImplType()
	switch impl {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		treeBcs := bcs.FollowSubBlock(2)
		taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/key-value-store/build-kv-transaction/iavl-new-tx")
		defer subtask.End()
		return iavl.NewTx(taskCtx, treeBcs, nil, write, func(ncs *block.Cursor) {
			_ = ncs.SetAsSubBlock(2, bcs)
		})
	default:
		return nil, NewErrUnknownImpl(impl)
	}
}
