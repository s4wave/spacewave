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
)

// DefaultKeyValueStoreImplForWorkload returns the measured backend policy for a
// new KVTX root.
func DefaultKeyValueStoreImplForWorkload(workload WorkloadClass) KVImplType {
	switch workload {
	case WorkloadClassGraphPrefixRead:
		return KVImplType_KV_IMPL_TYPE_OKRA_INLINE
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
	// Trace loading the KeyValueStore root block.
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block/load-key-value-store")
	defer task.End()

	// Decode the KeyValueStore and resolve its legacy implementation.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/load-key-value-store/unmarshal")
	b, err := block.UnmarshalBlock[KeyValueStore](taskCtx, bcs)
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
	// Trace construction of the KV block transaction.
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block/build-kv-transaction")
	defer task.End()

	// Load the KeyValueStore that selects the transaction backend.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/build-kv-transaction/load-key-value-store")
	kvs, err := LoadKeyValueStore(taskCtx, bcs)
	subtask.End()
	if err != nil {
		return nil, err
	}

	// Build the selected backend within its own trace task.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/kvtx-block/build-kv-transaction/build-impl")
	defer subtask.End()
	return kvs.BuildKvTransaction(taskCtx, bcs, write)
}

// layout returns the tree layout an implementation stores: IAVL, Okra or
// inline Okra, or 0 when the implementation is unknown. The node graph tags
// store the same layouts; they exist so a build that predates the node graph
// World rejects its trees.
func (i KVImplType) layout() KVImplType {
	switch i {
	case KVImplType_KV_IMPL_TYPE_IAVL, KVImplType_KV_IMPL_TYPE_IAVL_NODE_GRAPH:
		return KVImplType_KV_IMPL_TYPE_IAVL
	case KVImplType_KV_IMPL_TYPE_OKRA, KVImplType_KV_IMPL_TYPE_OKRA_NODE_GRAPH:
		return KVImplType_KV_IMPL_TYPE_OKRA
	case KVImplType_KV_IMPL_TYPE_OKRA_INLINE, KVImplType_KV_IMPL_TYPE_OKRA_INLINE_NODE_GRAPH:
		return KVImplType_KV_IMPL_TYPE_OKRA_INLINE
	}
	return 0
}

// Validate checks if the implementation is in the known set.
func (i KVImplType) Validate() error {
	if i.layout() == 0 {
		return NewErrUnknownImpl(i)
	}
	return nil
}

// Validate performs cursory checks of the KeyValueStore object.
func (k *KeyValueStore) Validate() error {
	if err := k.GetImplType().Validate(); err != nil {
		return err
	}
	switch k.GetImplType().layout() {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		if err := k.GetIavlRoot().Validate(); err != nil {
			return errors.Wrap(err, "iavl_root")
		}
	case KVImplType_KV_IMPL_TYPE_OKRA, KVImplType_KV_IMPL_TYPE_OKRA_INLINE:
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
	switch impl.layout() {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		treeBcs := bcs.FollowSubBlock(2)
		taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/key-value-store/build-kv-transaction/iavl-new-tx")
		defer subtask.End()
		return iavl.NewTx(taskCtx, treeBcs, nil, write, func(ncs *block.Cursor) {
			_ = ncs.SetAsSubBlock(2, bcs)
		})
	case KVImplType_KV_IMPL_TYPE_OKRA, KVImplType_KV_IMPL_TYPE_OKRA_INLINE:
		treeBcs := bcs.FollowSubBlock(3)
		taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block/key-value-store/build-kv-transaction/okra-new-tx")
		defer subtask.End()
		newTx := okra.NewTx
		if impl.layout() == KVImplType_KV_IMPL_TYPE_OKRA_INLINE {
			newTx = okra.NewTxWithInlineValues
		}
		return newTx(taskCtx, treeBcs, nil, write, func(ncs *block.Cursor) {
			_ = ncs.SetAsSubBlock(3, bcs)
		})
	default:
		return nil, NewErrUnknownImpl(impl)
	}
}
