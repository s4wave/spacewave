package sobject_world_engine

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestCommitMaintenanceOpWaitsForWriteTransaction verifies that a maintenance
// operation cannot advance the SharedObject root while a write transaction
// holds its base.
func TestCommitMaintenanceOpWaitsForWriteTransaction(t *testing.T) {
	// Build a controller whose participant owns the SharedObject.
	c := &Controller{
		le:   logrus.NewEntry(logrus.New()),
		conf: &Config{},
	}
	so := &testMaintenanceSharedObject{
		snapshot: &testMaintenanceSnapshot{
			role: sobject.SOParticipantRole_SOParticipantRole_OWNER,
		},
	}

	// Hold the write lock as an open write transaction does.
	unlockWriteMtx, err := c.writeMtx.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer unlockWriteMtx()

	// A maintenance operation gives up without queueing.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	build := func(state *InnerState) (*SOWorldOp, error) {
		return &SOWorldOp{Body: &SOWorldOp_AdvanceStorageGeneration{
			AdvanceStorageGeneration: &AdvanceStorageGenerationOp{},
		}}, nil
	}
	if _, err := c.commitMaintenanceOp(ctx, so, build); !errors.Is(err, context.Canceled) {
		t.Fatalf("maintenance during write transaction: got %v, want context.Canceled", err)
	}
	if len(so.queueOps) != 0 {
		t.Fatal("maintenance op queued while a write transaction held its base")
	}
}

type testMaintenanceSharedObject struct {
	snapshot   sobject.SharedObjectStateSnapshot
	blockStore bstore.BlockStore
	queueOps   [][]byte
}

func (s *testMaintenanceSharedObject) GetBus() bus.Bus {
	return nil
}

func (s *testMaintenanceSharedObject) GetPeerID() peer.ID {
	return ""
}

func (s *testMaintenanceSharedObject) GetSharedObjectID() string {
	return ""
}

func (s *testMaintenanceSharedObject) GetBlockStore() bstore.BlockStore {
	return s.blockStore
}

func (s *testMaintenanceSharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	return nil, nil, nil
}

func (s *testMaintenanceSharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snapshot, nil
}

func (s *testMaintenanceSharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, nil
}

func (s *testMaintenanceSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	s.queueOps = append(s.queueOps, bytes.Clone(op))
	return "maintenance-op", nil
}

func (s *testMaintenanceSharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	return 0, false, nil
}

func (s *testMaintenanceSharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	return nil
}

func (s *testMaintenanceSharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	return nil
}

type testMaintenanceSnapshot struct {
	role sobject.SOParticipantRole
}

func (s *testMaintenanceSnapshot) GetParticipantConfig(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	return &sobject.SOParticipantConfig{Role: s.role}, nil
}

func (s *testMaintenanceSnapshot) GetParticipantConfigForPeer(ctx context.Context, _ string) (*sobject.SOParticipantConfig, error) {
	return s.GetParticipantConfig(ctx)
}

func (s *testMaintenanceSnapshot) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) GetTransformInfo(ctx context.Context) (*sobject.TransformInfo, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) GetOpQueue(ctx context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, nil, nil
}

func (s *testMaintenanceSnapshot) GetRootInner(ctx context.Context) (*sobject.SORootInner, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) GetRootState(ctx context.Context) (*sobject.SORoot, error) {
	return nil, nil
}

func (s *testMaintenanceSnapshot) ProcessOperations(
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
