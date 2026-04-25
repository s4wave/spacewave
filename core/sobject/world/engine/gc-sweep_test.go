package sobject_world_engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	alpha_testbed "github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestQueueGCSweepTxRolePromotion verifies that GC sweep queueing follows the
// current participant role instead of the startup role.
func TestQueueGCSweepTxRolePromotion(t *testing.T) {
	ctx := context.Background()

	c := &Controller{
		le:   logrus.NewEntry(logrus.New()),
		conf: &Config{},
	}
	snap := &testGCSweepSnapshot{
		role: sobject.SOParticipantRole_SOParticipantRole_READER,
	}
	so := &testGCSweepSharedObject{
		snapshot: snap,
	}

	queued, err := c.queueGCSweepTx(ctx, so)
	if err != nil {
		t.Fatal(err.Error())
	}
	if queued {
		t.Fatal("expected reader role to skip gc sweep queueing")
	}
	if len(so.queueOps) != 0 {
		t.Fatal("reader role should not queue operations")
	}

	snap.role = sobject.SOParticipantRole_SOParticipantRole_OWNER
	queued, err = c.queueGCSweepTx(ctx, so)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !queued {
		t.Fatal("expected owner role to queue gc sweep")
	}
	if len(so.queueOps) != 1 {
		t.Fatalf("expected 1 queued op, got %d", len(so.queueOps))
	}

	op := &SOWorldOp{}
	if err := op.UnmarshalVT(so.queueOps[0]); err != nil {
		t.Fatal(err.Error())
	}
	body, ok := op.GetBody().(*SOWorldOp_ApplyTxOp)
	if !ok {
		t.Fatal("expected queued gc sweep op to be ApplyTxOp")
	}
	if body.ApplyTxOp.GetTx().GetTxType() != world_block_tx.TxType_TxType_GC_SWEEP {
		t.Fatalf("expected GC_SWEEP tx, got %s", body.ApplyTxOp.GetTx().GetTxType().String())
	}
}

// TestExecuteGCSweepMaintenanceWaitsForRoleChanges verifies that the
// maintenance routine no longer exits immediately when the peer starts
// unauthorized.
func TestExecuteGCSweepMaintenanceWaitsForRoleChanges(t *testing.T) {
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

	so := &testGCSweepSharedObject{
		snapshot: &testGCSweepSnapshot{
			role: sobject.SOParticipantRole_SOParticipantRole_READER,
		},
		blockStore: block_store.NewStore(tb.EngineBucketID, tb.Volume),
	}
	c := &Controller{
		le:   tb.Logger,
		conf: &Config{},
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.executeGCSweepMaintenance(runCtx, so, bengine)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("maintenance routine returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("maintenance routine did not exit after cancel")
	}
}

type testGCSweepSharedObject struct {
	snapshot   sobject.SharedObjectStateSnapshot
	blockStore block_store.Store
	queueOps   [][]byte
}

func (s *testGCSweepSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testGCSweepSharedObject) GetPeerID() peer.ID {
	return ""
}

func (s *testGCSweepSharedObject) GetSharedObjectID() string {
	return ""
}

func (s *testGCSweepSharedObject) GetBlockStore() bstore.BlockStore {
	return s.blockStore
}

func (s *testGCSweepSharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	return nil, nil, nil
}

func (s *testGCSweepSharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snapshot, nil
}

func (s *testGCSweepSharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, nil
}

func (s *testGCSweepSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	s.queueOps = append(s.queueOps, append([]byte(nil), op...))
	return "gc-sweep-op", nil
}

func (s *testGCSweepSharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	return 0, false, nil
}

func (s *testGCSweepSharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	return nil
}

func (s *testGCSweepSharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	return nil
}

type testGCSweepSnapshot struct {
	role sobject.SOParticipantRole
}

func (s *testGCSweepSnapshot) GetParticipantConfig(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	return &sobject.SOParticipantConfig{Role: s.role}, nil
}

func (s *testGCSweepSnapshot) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	return nil, nil
}

func (s *testGCSweepSnapshot) GetTransformInfo(ctx context.Context) (*sobject.TransformInfo, error) {
	return nil, nil
}

func (s *testGCSweepSnapshot) GetOpQueue(ctx context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, nil, nil
}

func (s *testGCSweepSnapshot) GetRootInner(ctx context.Context) (*sobject.SORootInner, error) {
	return nil, nil
}

func (s *testGCSweepSnapshot) ProcessOperations(
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
