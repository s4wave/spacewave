package sobject_world_engine

import (
	"context"
	"testing"
	"time"

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

// TestWaitReadableSnapshotWaitsForReadmission keeps waiting while the
// participant is removed and returns once it is readmitted.
func TestWaitReadableSnapshotWaitsForReadmission(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	// Wait on a departed participant's snapshot.
	inspected := make(chan struct{}, 1)
	departed := &testSharedObjectSnapshot{readErr: sobject.ErrNotParticipant, inspected: inspected}
	state := ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](departed)
	done := make(chan error, 1)
	go func() {
		done <- waitReadableSnapshot(ctx, state)
	}()

	// The departed snapshot is inspected without ending the wait.
	select {
	case <-inspected:
	case err := <-done:
		t.Fatalf("departed participant ended the wait: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Readmission ends the wait.
	state.SetValue(&testSharedObjectSnapshot{
		participant: &sobject.SOParticipantConfig{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
	})
	if err := <-done; err != nil {
		t.Fatalf("wait returned %v", err)
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
	decodedBlocks.Wait()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	second, err := c.buildBlkEngine(secondCtx, tb.Logger, so, headRef.CloneVT(), transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer second.Release()

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

// testSharedObject serves a block store, a local state store and a fixed
// snapshot, and records the operations queued on it.
type testSharedObject struct {
	peerID     peer.ID
	blockStore bstore.BlockStore
	localStore kvtx.Store
	snapshot   sobject.SharedObjectStateSnapshot
	queued     [][]byte
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

func (s *testBlockStore) ReclaimStorage(context.Context, func(context.Context) error) (time.Time, error) {
	return time.Time{}, nil
}

func (s *testSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testSharedObject) GetPeerID() peer.ID {
	return s.peerID
}

func (s *testSharedObject) GetSharedObjectID() string {
	return ""
}

func (s *testSharedObject) GetBlockStore() bstore.BlockStore {
	return s.blockStore
}

func (s *testSharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	return s.localStore, func() {}, nil
}

func (s *testSharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snapshot, nil
}

func (s *testSharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, nil
}

func (s *testSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	s.queued = append(s.queued, op)
	return "", nil
}

type testSharedObjectSnapshot struct {
	participant  *sobject.SOParticipantConfig
	participants map[string]*sobject.SOParticipantConfig
	inspected    chan<- struct{}
	readErr      error
}

func (s *testSharedObjectSnapshot) GetParticipantConfig(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	if s.inspected != nil {
		s.inspected <- struct{}{}
	}
	return s.participant, s.readErr
}

func (s *testSharedObjectSnapshot) GetParticipantConfigForPeer(ctx context.Context, peerID string) (*sobject.SOParticipantConfig, error) {
	if s.participants != nil {
		participant, ok := s.participants[peerID]
		if !ok {
			return nil, sobject.ErrNotParticipant
		}
		return participant, nil
	}
	return s.GetParticipantConfig(ctx)
}

func (s *testSharedObjectSnapshot) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	return nil, nil
}

func (s *testSharedObjectSnapshot) GetTransformInfo(ctx context.Context) (*sobject.TransformInfo, error) {
	return nil, nil
}

func (s *testSharedObjectSnapshot) GetConfigByHash(ctx context.Context, hash []byte) (*sobject.SharedObjectConfig, error) {
	return nil, nil
}

func (s *testSharedObjectSnapshot) GetCheckpoint(ctx context.Context) (*sobject.SOCheckpointInner, error) {
	return &sobject.SOCheckpointInner{}, nil
}

func (s *testSharedObjectSnapshot) GetOperationSet(ctx context.Context) (*sobject.SOOperationSet, error) {
	return sobject.NewSOOperationSet("", &sobject.SOCheckpointInner{}), nil
}

func (s *testSharedObjectSnapshot) DecodeOperation(ctx context.Context, inner *sobject.SOOperationInner) ([]byte, error) {
	return inner.GetOpData(), nil
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
