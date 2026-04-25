package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
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
		blockStore: block_store.NewStore(tb.EngineBucketID, tb.Volume),
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

type testSharedObject struct {
	blockStore bstore.BlockStore
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
