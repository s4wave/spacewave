package sobject_world_engine

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	world "github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/crypto"
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
		t.Fatal(err)
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
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("expected owner role to queue gc sweep")
	}
	if len(so.queueOps) != 1 {
		t.Fatalf("expected 1 queued op, got %d", len(so.queueOps))
	}

	op := &SOWorldOp{}
	if err := op.UnmarshalVT(so.queueOps[0]); err != nil {
		t.Fatal(err)
	}
	body, ok := op.GetBody().(*SOWorldOp_ApplyTxOp)
	if !ok {
		t.Fatal("expected queued gc sweep op to be ApplyTxOp")
	}
	if body.ApplyTxOp.GetTx().GetTxType() != world_block_tx.TxType_TxType_GC_SWEEP {
		t.Fatalf("expected GC_SWEEP tx, got %s", body.ApplyTxOp.GetTx().GetTxType().String())
	}
	if body.ApplyTxOp.GetTx().GetTxGcSweep().GetIntent() != world_block_tx.TxGCSweepIntent_TxGCSweepIntent_MAINTENANCE {
		t.Fatalf("expected maintenance sweep intent, got %s", body.ApplyTxOp.GetTx().GetTxGcSweep().GetIntent().String())
	}
}

// TestExecuteGCSweepMaintenanceWaitsForRoleChanges verifies that the
// maintenance routine no longer exits immediately when the peer starts
// unauthorized.
func TestExecuteGCSweepMaintenanceWaitsForRoleChanges(t *testing.T) {
	ctx := context.Background()

	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ocs.Release()

	bengine, err := world_block.NewEngine(ctx, tb.Logger, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	so := &testGCSweepSharedObject{
		snapshot: &testGCSweepSnapshot{
			role: sobject.SOParticipantRole_SOParticipantRole_READER,
		},
		blockStore: newTestBlockStore(tb.EngineBucketID, tb.Volume),
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

// TestExecuteGCSweepMaintenanceDefersSubthresholdIdleJournal keeps sparse garbage for the backstop instead of every idle interval.
func TestExecuteGCSweepMaintenanceDefersSubthresholdIdleJournal(t *testing.T) {
	ctx := context.Background()

	c := &Controller{
		le: logrus.NewEntry(logrus.New()),
		conf: &Config{
			GcSweepIdleWindowDur: uint64(time.Millisecond),
		},
	}
	so := &testGCSweepSharedObject{
		snapshot: &testGCSweepSnapshot{
			role: sobject.SOParticipantRole_SOParticipantRole_OWNER,
		},
		queueCh: make(chan struct{}, 1),
	}
	counter := &testGCJournalEntryCounter{entries: 1}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.executeGCSweepMaintenance(runCtx, so, counter)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("maintenance routine returned early: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	c.notifyGCSweepMaintenance()

	select {
	case err := <-errCh:
		t.Fatalf("maintenance routine returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if len(so.queueOps) != 0 {
		t.Fatalf("subthreshold idle journal queued %d gc sweep ops", len(so.queueOps))
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

// TestExecuteGCSweepMaintenanceQueuesThresholdJournal queues maintenance when the journal reaches its work threshold.
func TestExecuteGCSweepMaintenanceQueuesThresholdJournal(t *testing.T) {
	ctx := context.Background()

	c := &Controller{
		le: logrus.NewEntry(logrus.New()),
		conf: &Config{
			GcSweepIdleWindowDur:       uint64(time.Millisecond),
			GcSweepBackstopIntervalDur: uint64(time.Millisecond),
		},
	}
	so := &testGCSweepSharedObject{
		snapshot: &testGCSweepSnapshot{
			role: sobject.SOParticipantRole_SOParticipantRole_OWNER,
		},
		queueCh: make(chan struct{}, 1),
	}
	counter := &testGCJournalEntryCounter{entries: gcSweepJournalThreshold}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.executeGCSweepMaintenance(runCtx, so, counter)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("maintenance routine returned early: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	c.notifyGCSweepMaintenance()

	select {
	case <-so.queueCh:
	case err := <-errCh:
		t.Fatalf("maintenance routine returned early: %v", err)
	case <-time.After(time.Second):
		t.Fatal("threshold journal did not queue gc sweep")
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

// TestTwoPeerRemoteDeleteQueuesMaintenanceGCSweep reconciles remote deletions through authorized signed maintenance operations.
func TestTwoPeerRemoteDeleteQueuesMaintenanceGCSweep(t *testing.T) {
	ctx := t.Context()

	c, so, headState := newProcessTestWorld(t, ctx)
	c.conf.GcSweepIdleWindowDur = 1
	keys, baselineEntries := seedGCSweepTestObjects(t, ctx, c, so, headState, gcSweepJournalThreshold)

	sharedObjectID := "two-peer-gc-sweep"
	remotePriv, remoteID := newGCSweepTestPeer(t)
	maintenancePriv, maintenanceID := newGCSweepTestPeer(t)
	state, maintenanceSnap, xfrm := newTwoPeerGCSweepTestState(
		t,
		ctx,
		c,
		sharedObjectID,
		headState,
		remoteID,
		maintenancePriv,
		maintenanceID,
	)

	for i, key := range keys {
		tx, err := world_block_tx.NewTxDeleteObject(key)
		if err != nil {
			t.Fatal(err)
		}
		queueGCSweepTestTx(t, ctx, sharedObjectID, state, xfrm, remotePriv, uint64(i+1), sobject.NewSOOperationLocalID(), tx)
	}

	deleteHead := processGCSweepTestStateOps(t, ctx, c, so, state, maintenanceSnap, sharedObjectID, maintenanceID)
	pending := getGCSweepTestJournalEntries(t, ctx, c, so, deleteHead)
	if pending <= baselineEntries {
		t.Fatalf("remote deletes left %d pending gc journal entries, want more than baseline %d", pending, baselineEntries)
	}

	queueSO := &testGCSweepSharedObject{
		snapshot:   maintenanceSnap,
		blockStore: so.blockStore,
	}
	queued, err := c.queueGCSweepTx(ctx, queueSO)
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("authorized maintenance peer did not queue gc sweep")
	}
	if len(queueSO.queueOps) != 1 {
		t.Fatalf("expected 1 queued maintenance op, got %d", len(queueSO.queueOps))
	}
	queuedOp := &SOWorldOp{}
	if err := queuedOp.UnmarshalVT(queueSO.queueOps[0]); err != nil {
		t.Fatal(err)
	}
	body, ok := queuedOp.GetBody().(*SOWorldOp_ApplyTxOp)
	if !ok {
		t.Fatal("expected maintenance peer to queue ApplyTxOp")
	}
	if body.ApplyTxOp.GetTx().GetTxType() != world_block_tx.TxType_TxType_GC_SWEEP {
		t.Fatalf("expected queued GC_SWEEP tx, got %s", body.ApplyTxOp.GetTx().GetTxType().String())
	}

	// Maintenance reconciles a bounded journal chunk, and committing its new
	// root can append another entry. Require progress across signed operations
	// and retain the original final garbage-reduction bound.
	for nonce := uint64(1); nonce <= 4; nonce++ {
		queueGCSweepTestRawOp(t, ctx, sharedObjectID, state, xfrm, maintenancePriv, nonce, sobject.NewSOOperationLocalID(), queueSO.queueOps[0])
		sweepHead := processGCSweepTestStateOps(t, ctx, c, so, state, maintenanceSnap, sharedObjectID, maintenanceID)
		entries := getGCSweepTestJournalEntries(t, ctx, c, so, sweepHead)
		if entries >= pending {
			t.Fatalf("maintenance operation %d left %d entries after %d: no progress", nonce, entries, pending)
		}
		if entries <= baselineEntries {
			return
		}
		pending = entries
	}
	t.Fatalf("bounded maintenance left %d pending journal entries, want at most baseline %d", pending, baselineEntries)
}

// newGCSweepTestPeer creates an independent signer for operation authority checks.
func newGCSweepTestPeer(t *testing.T) (crypto.PrivKey, peer.ID) {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pid
}

// seedGCSweepTestObjects persists a populated World and returns its pre-deletion journal size.
func seedGCSweepTestObjects(
	t *testing.T,
	ctx context.Context,
	c *Controller,
	so *testSharedObject,
	headState *InnerState,
	count uint64,
) ([]string, uint64) {
	t.Helper()
	ws, err := c.buildBlkEngine(ctx, c.le, so, headState.GetHeadRef().CloneVT(), headState.GetHeadRef().GetTransformConf())
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Release()

	worldState := world.NewEngineWorldState(ws.bengine, true)
	keys := make([]string, 0, count)
	for i := range count {
		key := "gc-sweep-remote-delete-" + strconv.FormatUint(i, 10)
		if _, err := world_block.BuildMockObject(ctx, worldState, key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}

	if _, err := ws.bengine.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	baselineEntries := ws.bengine.GetGCJournalEntries()

	headState.HeadRef = ws.bengine.GetRootRef()
	headState.HeadRef.BucketId = ""
	return keys, baselineEntries
}

// newTwoPeerGCSweepTestState grants a writer and maintenance owner access to the same World.
func newTwoPeerGCSweepTestState(
	t *testing.T,
	ctx context.Context,
	c *Controller,
	sharedObjectID string,
	headState *InnerState,
	remoteID peer.ID,
	maintenancePriv crypto.PrivKey,
	maintenanceID peer.ID,
) (*sobject.SOState, sobject.SharedObjectStateSnapshot, *block_transform.Transformer) {
	t.Helper()
	transformConf := headState.GetHeadRef().GetTransformConf()
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: c.le}, c.sfs, transformConf)
	if err != nil {
		t.Fatal(err)
	}

	stateData, err := headState.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	rootInnerData, err := (&sobject.SORootInner{
		Seqno:     1,
		StateData: stateData,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	encodedStateData, err := xfrm.EncodeBlock(rootInnerData)
	if err != nil {
		t.Fatal(err)
	}

	remoteGrant := newGCSweepTestGrant(t, sharedObjectID, maintenancePriv, remoteID, transformConf)
	maintenanceGrant := newGCSweepTestGrant(t, sharedObjectID, maintenancePriv, maintenanceID, transformConf)
	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				{
					PeerId: remoteID.String(),
					Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
				},
				{
					PeerId: maintenanceID.String(),
					Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
				},
			},
		},
		Root: &sobject.SORoot{
			Inner:      encodedStateData,
			InnerSeqno: 1,
		},
		RootGrants: []*sobject.SOGrant{remoteGrant, maintenanceGrant},
	}

	snap := sobject.NewSOStateParticipantHandle(
		c.le,
		c.sfs,
		sharedObjectID,
		state,
		maintenancePriv,
		maintenanceID,
	)
	if _, err := snap.GetRootInner(ctx); err != nil {
		t.Fatal(err)
	}
	return state, snap, xfrm
}

// newGCSweepTestGrant encrypts the World transform for a test participant.
func newGCSweepTestGrant(
	t *testing.T,
	sharedObjectID string,
	signerPriv crypto.PrivKey,
	recipientID peer.ID,
	transformConf *block_transform.Config,
) *sobject.SOGrant {
	t.Helper()
	recipientPub, err := recipientID.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	grant, err := sobject.EncryptSOGrant(
		signerPriv,
		recipientPub,
		sharedObjectID,
		&sobject.SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

// queueGCSweepTestTx encodes a World transaction into a signed operation.
func queueGCSweepTestTx(
	t *testing.T,
	ctx context.Context,
	sharedObjectID string,
	state *sobject.SOState,
	xfrm *block_transform.Transformer,
	priv crypto.PrivKey,
	nonce uint64,
	localID string,
	tx *world_block_tx.Tx,
) {
	t.Helper()
	queueGCSweepTestRawOp(t, ctx, sharedObjectID, state, xfrm, priv, nonce, localID, marshalApplyTxOpForProcessTest(t, tx))
}

// queueGCSweepTestRawOp queues an encrypted operation signed by the selected participant.
func queueGCSweepTestRawOp(
	t *testing.T,
	ctx context.Context,
	sharedObjectID string,
	state *sobject.SOState,
	xfrm *block_transform.Transformer,
	priv crypto.PrivKey,
	nonce uint64,
	localID string,
	opData []byte,
) {
	t.Helper()
	encodedOpData, err := xfrm.EncodeBlock(opData)
	if err != nil {
		t.Fatal(err)
	}
	op, err := sobject.BuildSOOperation(sharedObjectID, priv, encodedOpData, nonce, localID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.QueueOperation(sharedObjectID, op); err != nil {
		t.Fatal(err)
	}
}

// processGCSweepTestStateOps applies queued operations through the real participant snapshot.
func processGCSweepTestStateOps(
	t *testing.T,
	ctx context.Context,
	c *Controller,
	so *testSharedObject,
	state *sobject.SOState,
	snap sobject.SharedObjectStateSnapshot,
	sharedObjectID string,
	validatorID peer.ID,
) *InnerState {
	t.Helper()
	queuedOps := slices.Clone(state.GetOps())
	if len(queuedOps) == 0 {
		t.Fatal("expected queued operations")
	}

	nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
		ctx,
		queuedOps,
		func(ctx context.Context, currentStateData []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
			currentHead := &InnerState{}
			if err := currentHead.UnmarshalVT(currentStateData); err != nil {
				return nil, nil, err
			}
			results := make([]*sobject.SOOperationResult, 0, len(ops))
			for i, op := range ops {
				opPeerID, err := peer.IDB58Decode(op.GetPeerId())
				if err != nil {
					return nil, nil, err
				}
				nextHead, res, err := c.processOp(
					ctx,
					c.le,
					so,
					op.GetOpData(),
					op.GetLocalId(),
					opPeerID,
					op.GetNonce(),
					i,
					currentHead,
				)
				if err != nil {
					return nil, nil, err
				}
				if res == nil || !res.GetSuccess() {
					t.Fatalf("operation %s rejected: %#v", op.GetLocalId(), res)
				}
				if nextHead != nil {
					currentHead = nextHead
				}
				results = append(results, res)
			}
			nextStateData, err := currentHead.MarshalVT()
			if err != nil {
				return nil, nil, err
			}
			return &nextStateData, results, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejectedOps) != 0 {
		t.Fatalf("expected no rejected operations, got %d", len(rejectedOps))
	}
	if len(acceptedOps) != len(queuedOps) {
		t.Fatalf("expected %d accepted operations, got %d", len(queuedOps), len(acceptedOps))
	}
	if err := state.UpdateRootState(sharedObjectID, nextRoot, validatorID.String(), rejectedOps, acceptedOps); err != nil {
		t.Fatal(err)
	}

	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := &InnerState{}
	if err := head.UnmarshalVT(rootInner.GetStateData()); err != nil {
		t.Fatal(err)
	}
	return head
}

// getGCSweepTestJournalEntries reads the journal from a reopened committed World head.
func getGCSweepTestJournalEntries(
	t *testing.T,
	ctx context.Context,
	c *Controller,
	so *testSharedObject,
	headState *InnerState,
) uint64 {
	t.Helper()
	ws, err := c.buildBlkEngine(ctx, c.le, so, headState.GetHeadRef().CloneVT(), headState.GetHeadRef().GetTransformConf())
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Release()
	return ws.bengine.GetGCJournalEntries()
}

type testGCSweepSharedObject struct {
	snapshot   sobject.SharedObjectStateSnapshot
	blockStore bstore.BlockStore
	queueOps   [][]byte
	queueCh    chan struct{}
}

type testGCJournalEntryCounter struct {
	entries uint64
}

func (c *testGCJournalEntryCounter) GetGCJournalEntries() uint64 {
	return c.entries
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
	s.queueOps = append(s.queueOps, bytes.Clone(op))
	if s.queueCh != nil {
		select {
		case s.queueCh <- struct{}{}:
		default:
		}
	}
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

func (s *testGCSweepSnapshot) GetRootState(ctx context.Context) (*sobject.SORoot, error) {
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
