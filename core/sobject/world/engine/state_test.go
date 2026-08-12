package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/util/blockenc"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	alpha_testbed "github.com/s4wave/spacewave/testbed"
)

// TestExecuteWatchSOStateOnceSignalsGCSweepMaintenance verifies that
// authoritative watch-state updates wake the GC maintenance routine.
func TestExecuteWatchSOStateOnceSignalsGCSweepMaintenance(t *testing.T) {
	ctx := context.Background()

	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	bengine, err := world_block.NewEngine(ctx, tb.Logger, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	headRef := bengine.GetRootRef().CloneVT()
	headRef.BucketId = ""
	stateData, err := (&InnerState{HeadRef: headRef}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	c := &Controller{le: tb.Logger}
	so := &testSharedObject{
		blockStore: newTestBlockStore(tb.EngineBucketID, tb.Volume),
	}
	soEngine := &soEngine{
		c:       c,
		so:      so,
		bengine: bengine,
	}
	snap := &testSharedObjectSnapshot{
		rootInner: &sobject.SORootInner{
			Seqno:     1,
			StateData: stateData,
		},
	}

	var waitCh <-chan struct{}
	c.writeBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	if err := c.executeWatchSOStateOnce(ctx, tb.Logger, so, snap, soEngine); err != nil {
		t.Fatal(err.Error())
	}

	select {
	case <-waitCh:
	default:
		t.Fatal("expected watch-state update to signal gc sweep maintenance")
	}
}

func TestBuildLookupWorldOpObservesStaticLookupSetAfterBuild(t *testing.T) {
	ctx := context.Background()
	c := &Controller{conf: &Config{DisableLookup: true}}
	lookup := c.buildLookupWorldOp(nil)

	op, err := lookup(ctx, world_mock.MockWorldOpId)
	if err != nil {
		t.Fatal(err.Error())
	}
	if op != nil {
		t.Fatal("lookup resolved operation before static lookup was set")
	}

	c.SetStaticLookupOp(world_mock.LookupMockOp)
	op, err = lookup(ctx, world_mock.MockWorldOpId)
	if err != nil {
		t.Fatal(err.Error())
	}
	if op == nil {
		t.Fatal("lookup did not observe static lookup set after build")
	}
}

func TestBuildBlkEngineBorrowsTransformAwareBlockStoreDecodedCache(t *testing.T) {
	ctx := context.Background()

	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	transformConf := newStateTestTransformConfig(t, &transform_gzip.Config{})
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, tb.StepFactorySet, transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}

	store := newTestBlockStore(tb.EngineBucketID, tb.Volume)
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()
	store.decodedBlocks = decodedBlocks

	tx, bcs := block.NewTransaction(store, xfrm, nil, nil)
	bcs.SetBlock(world_block.NewWorld(true), true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	c := &Controller{
		le:   tb.Logger,
		bus:  tb.Bus,
		conf: &Config{DisableLookup: true},
		sfs:  tb.StepFactorySet,
	}
	so := &testSharedObject{blockStore: store}
	headRef := &bucket.ObjectRef{RootRef: rootRef, TransformConf: transformConf}

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	first, err := c.buildBlkEngine(firstCtx, tb.Logger, so, headRef.CloneVT(), transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer first.Release()
	if first.decodedBlocks != decodedBlocks || first.ownDecodedBlocks {
		t.Fatal("first world engine did not borrow the block-store decoded cache")
	}
	decodedBlocks.Wait()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := c.buildBlkEngine(secondCtx, tb.Logger, so, headRef.CloneVT(), transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer second.Release()
	if second.decodedBlocks != decodedBlocks || second.ownDecodedBlocks {
		t.Fatal("second world engine did not borrow the block-store decoded cache")
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 ||
		firstSnapshot.DecodedBlockStoreAcceptedCount != 1 ||
		firstSnapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected first transformed world-engine counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected second transformed world-engine counters: %+v", secondSnapshot)
	}
}

type testSharedObject struct {
	blockStore bstore.BlockStore
}

type testBlockStore struct {
	block_store.Store
	decodedBlocks *block.DecodedBlockCache
}

func newTestBlockStore(id string, store block.StoreOps) *testBlockStore {
	return &testBlockStore{Store: block_store.NewStore(id, store)}
}

func (s *testBlockStore) GetDecodedBlockCache() *block.DecodedBlockCache {
	return s.decodedBlocks
}

func (s *testSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testSharedObject) GetPeerID() peer.ID {
	return ""
}

func (s *testSharedObject) GetSharedObjectID() string {
	return ""
}

func (s *testSharedObject) GetBlockStore() bstore.BlockStore {
	return s.blockStore
}

func (s *testSharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	return nil, nil, nil
}

func (s *testSharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return nil, nil
}

func (s *testSharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, nil
}

func (s *testSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	return "", nil
}

func (s *testSharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	return 0, false, nil
}

func (s *testSharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	return nil
}

func (s *testSharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	return nil
}

type testSharedObjectSnapshot struct {
	rootInner *sobject.SORootInner
}

func (s *testSharedObjectSnapshot) GetParticipantConfig(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	return nil, nil
}

func (s *testSharedObjectSnapshot) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	return nil, nil
}

func (s *testSharedObjectSnapshot) GetTransformInfo(ctx context.Context) (*sobject.TransformInfo, error) {
	return nil, nil
}

func (s *testSharedObjectSnapshot) GetOpQueue(ctx context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, nil, nil
}

func (s *testSharedObjectSnapshot) GetRootInner(ctx context.Context) (*sobject.SORootInner, error) {
	return s.rootInner, nil
}

func (s *testSharedObjectSnapshot) GetRootState(ctx context.Context) (*sobject.SORoot, error) {
	if s.rootInner == nil {
		return nil, nil
	}
	return &sobject.SORoot{InnerSeqno: s.rootInner.GetSeqno()}, nil
}

func (s *testSharedObjectSnapshot) ProcessOperations(
	ctx context.Context,
	ops []*sobject.SOOperation,
	cb sobject.SnapshotProcessOpsFunc,
) (
	nextRoot *sobject.SORoot,
	rejectedOps []*sobject.SOOperationRejection,
	acceptedOps []*sobject.SOOperation,
	err error,
) {
	return nil, nil, nil, nil
}

func newStateTestTransformConfig(t *testing.T, steps ...config.Config) *block_transform.Config {
	t.Helper()
	steps = append(steps, &transform_blockenc.Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      make([]byte, 32),
	})
	transformConf, err := block_transform.NewConfig(steps)
	if err != nil {
		t.Fatal(err.Error())
	}
	return transformConf
}
