package kvtx_block

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// LegacyKeyValueStoreImpl is the implementation used for persisted roots with a
// missing impl_type field.
const LegacyKeyValueStoreImpl = KVImplType_KV_IMPL_TYPE_IAVL

// DefaultKeyValueStoreImpl is the default implementation for new writable roots.
const DefaultKeyValueStoreImpl = KVImplType_KV_IMPL_TYPE_IAVL

// WorkloadClass classifies the KVTX workload behind a new root.
type WorkloadClass uint8

const (
	// WorkloadClassDefault is the conservative writable KVTX policy.
	WorkloadClassDefault WorkloadClass = iota
	// WorkloadClassTinyMetadata is for small metadata stores with mixed reads and writes.
	WorkloadClassTinyMetadata
	// WorkloadClassGraphPrefixRead is for read-heavy graph/prefix scans.
	WorkloadClassGraphPrefixRead
	// WorkloadClassIndexedLog is for append logs that find the latest sorted key.
	WorkloadClassIndexedLog
	// WorkloadClassCursorValueRead is for read-heavy cursor-valued stores.
	WorkloadClassCursorValueRead
	// WorkloadClassWriteChurn is for repeated set/delete/commit workloads.
	WorkloadClassWriteChurn
	// WorkloadClassGCRefGraph is for GC/refgraph metadata stores.
	WorkloadClassGCRefGraph
)

// DefaultKeyValueStoreImplForWorkload returns the measured backend policy for a
// new KVTX root.
func DefaultKeyValueStoreImplForWorkload(workload WorkloadClass) KVImplType {
	switch workload {
	case WorkloadClassGraphPrefixRead:
		return KVImplType_KV_IMPL_TYPE_OKRA
	default:
		return DefaultKeyValueStoreImpl
	}
}

// NewKeyValueStoreForWorkload constructs a new key-value store for a workload
// class.
func NewKeyValueStoreForWorkload(workload WorkloadClass) *KeyValueStore {
	return NewKeyValueStore(DefaultKeyValueStoreImplForWorkload(workload))
}

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
		b.ImplType = LegacyKeyValueStoreImpl
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
	case KVImplType_KV_IMPL_TYPE_OKRA:
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
	switch k.GetImplType() {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		if err := k.GetIavlRoot().Validate(); err != nil {
			return errors.Wrap(err, "iavl_root")
		}
	case KVImplType_KV_IMPL_TYPE_OKRA:
		if err := k.GetOkraRoot().Validate(); err != nil {
			return errors.Wrap(err, "okra_root")
		}
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
	case KVImplType_KV_IMPL_TYPE_OKRA:
		treeBcs := bcs.FollowSubBlock(3)
		taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/key-value-store/build-kv-transaction/okra-new-tx")
		defer subtask.End()
		return okra.NewTx(taskCtx, treeBcs, nil, write, func(ncs *block.Cursor) {
			_ = ncs.SetAsSubBlock(3, bcs)
		})
	default:
		return nil, NewErrUnknownImpl(impl)
	}
}
