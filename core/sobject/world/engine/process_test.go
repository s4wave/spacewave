package sobject_world_engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	alpha_testbed "github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestProcessApplyTxOpRejectsUninitializedWorld(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	tx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		t.Fatalf("build tx: %v", err)
	}
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}

	_, res, err := (&Controller{}).processOp(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		nil,
		opData,
		"test-op",
		pid,
		1,
		0,
		&InnerState{},
	)
	if err != nil {
		t.Fatalf("process op returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected rejection result")
	}
	if res.GetSuccess() {
		t.Fatal("expected apply tx op to be rejected")
	}
	details := res.GetErrorDetails()
	if details.GetErrorMsg() != "world is not initialized" {
		t.Fatalf("expected world-not-initialized rejection, got %q", details.GetErrorMsg())
	}
}

func TestProcessInitWorldOpWritesDisabledChangelogRoot(t *testing.T) {
	ctx := context.Background()
	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	store := newTestBlockStore(tb.EngineBucketID, tb.Volume)
	pid := newProcessTestPeerID(t)
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_InitWorld{
			InitWorld: &InitWorldOp{LastChangeDisable: true},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	nextState, res, err := (&Controller{
		le:   tb.Logger,
		bus:  tb.Bus,
		conf: &Config{},
		sfs:  tb.StepFactorySet,
	}).processOp(
		ctx,
		tb.Logger,
		&testSharedObject{blockStore: store},
		opData,
		"test-op",
		pid,
		1,
		0,
		&InnerState{},
	)
	if err != nil {
		t.Fatalf("process op returned error: %v", err)
	}
	if res == nil || !res.GetSuccess() {
		t.Fatalf("expected init success, got %#v", res)
	}
	headRef := nextState.GetHeadRef()
	if headRef.GetRootRef().GetEmpty() {
		t.Fatal("expected disabled changelog init to write an initial world root")
	}

	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: tb.Logger},
		tb.StepFactorySet,
		headRef.GetTransformConf(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs := block.NewTransaction(store, xfrm, headRef.GetRootRef(), nil)
	wi, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	worldState := wi.(*world_block.World)
	if !worldState.GetLastChangeDisable() {
		t.Fatal("expected initial world root to disable changelog")
	}
}

func TestSOWorldOpSpeculativeLocalQueueSafeSkipsGCSweep(t *testing.T) {
	gcTx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		t.Fatal(err)
	}
	gcOp := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: gcTx},
		},
	}
	if gcOp.speculativeLocalQueueSafe() {
		t.Fatal("GC sweep should wait for authoritative processing")
	}

	createTx, err := world_block_tx.NewTxCreateObject("obj", &bucket.ObjectRef{})
	if err != nil {
		t.Fatal(err)
	}
	createOp := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: createTx},
		},
	}
	if !createOp.speculativeLocalQueueSafe() {
		t.Fatal("regular world transactions should still be speculative")
	}

	batchTx, err := world_block_tx.NewTxBatch(&world_block_tx.TxBatch{Txs: []*world_block_tx.Tx{createTx, gcTx}})
	if err != nil {
		t.Fatal(err)
	}
	batchOp := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: batchTx},
		},
	}
	if batchOp.speculativeLocalQueueSafe() {
		t.Fatal("batched GC sweep should wait for authoritative processing")
	}
}

func TestProcessOpRejectsDisabledMaintenanceGCSweepBeforeBlockEngine(t *testing.T) {
	pid := newProcessTestPeerID(t)
	headState, err := BuildInitialInnerState(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	gcTx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		t.Fatal(err.Error())
	}
	explicitTx, err := world_block_tx.NewExplicitTxGCSweep()
	if err != nil {
		t.Fatal(err.Error())
	}
	ordinaryTx, err := world_block_tx.NewTxCreateObject("object", &bucket.ObjectRef{})
	if err != nil {
		t.Fatal(err.Error())
	}
	batchTx, err := world_block_tx.NewTxBatch(&world_block_tx.TxBatch{Txs: []*world_block_tx.Tx{ordinaryTx, gcTx}})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, test := range []struct {
		name string
		tx   *world_block_tx.Tx
	}{
		{name: "top-level", tx: gcTx},
		{name: "batch", tx: batchTx},
		{name: "explicit-reserved", tx: explicitTx},
	} {
		t.Run(test.name, func(t *testing.T) {
			opData := marshalApplyTxOpForProcessTest(t, test.tx)
			nextState, res, err := (&Controller{conf: &Config{}}).processOp(
				context.Background(),
				logrus.NewEntry(logrus.New()),
				nil,
				opData,
				"test-op",
				pid,
				1,
				0,
				headState,
			)
			if err != nil {
				t.Fatalf("process op returned error: %v", err)
			}
			if nextState != nil {
				t.Fatal("expected disabled maintenance rejection to leave state unchanged")
			}
			if res == nil || res.GetSuccess() {
				t.Fatalf("expected rejection result, got %#v", res)
			}
			if got := res.GetErrorDetails().GetErrorMsg(); got != "gc sweep maintenance disabled" {
				t.Fatalf("expected disabled maintenance rejection, got %q", got)
			}
		})
	}
}

func TestProcessOpDisabledMaintenanceAllowsOrdinaryApplyTx(t *testing.T) {
	ctx := context.Background()
	pid := newProcessTestPeerID(t)
	c, so, headState := newProcessTestWorld(t, ctx)

	objectRef := headState.GetHeadRef().CloneVT()
	objectTx, err := world_block_tx.NewTxCreateObject("ordinary-object", objectRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	nextState, res, err := c.processOp(
		ctx,
		logrus.NewEntry(logrus.New()),
		so,
		marshalApplyTxOpForProcessTest(t, objectTx),
		"test-op",
		pid,
		1,
		0,
		headState,
	)
	if err != nil {
		t.Fatalf("process op returned error: %v", err)
	}
	if nextState == nil || nextState.GetHeadRef().GetEmpty() {
		t.Fatal("expected ordinary transaction to produce a next world state")
	}
	if res == nil || !res.GetSuccess() {
		t.Fatalf("expected ordinary transaction success, got %#v", res)
	}
}

func TestProcessOpCandidateRequiresSharedObjectRootUpdate(t *testing.T) {
	ctx := context.Background()
	sharedObjectID := "test-candidate-finalization"
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err.Error())
	}
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		t.Fatal(err.Error())
	}

	c, so, headState := newProcessTestWorld(t, ctx)
	baseStateData, err := headState.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	transformConf := newStateTestTransformConfig(t, &transform_gzip.Config{})
	grant, err := sobject.EncryptSOGrant(
		priv,
		pub,
		sharedObjectID,
		&sobject.SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{
		Logger: logrus.NewEntry(logrus.New()),
	}, sfs, transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootInnerData, err := (&sobject.SORootInner{
		Seqno:     1,
		StateData: baseStateData,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	encodedStateData, err := xfrm.EncodeBlock(rootInnerData)
	if err != nil {
		t.Fatal(err.Error())
	}

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: pid.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		Root: &sobject.SORoot{
			Inner:      encodedStateData,
			InnerSeqno: 1,
		},
		RootGrants: []*sobject.SOGrant{grant},
	}
	snap := sobject.NewSOStateParticipantHandle(
		logrus.NewEntry(logrus.New()),
		sfs,
		sharedObjectID,
		state,
		priv,
		pid,
	)

	objectTx, err := world_block_tx.NewTxCreateObject("candidate-object", headState.GetHeadRef())
	if err != nil {
		t.Fatal(err.Error())
	}
	encodedOpData, err := xfrm.EncodeBlock(marshalApplyTxOpForProcessTest(t, objectTx))
	if err != nil {
		t.Fatal(err.Error())
	}
	op, err := sobject.BuildSOOperation(
		sharedObjectID,
		priv,
		encodedOpData,
		1,
		sobject.NewSOOperationLocalID(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := state.QueueOperation(sharedObjectID, op); err != nil {
		t.Fatal(err.Error())
	}

	nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
		ctx,
		[]*sobject.SOOperation{op},
		func(ctx context.Context, currentStateData []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
			if len(ops) != 1 {
				t.Fatalf("expected 1 op, got %d", len(ops))
			}
			currentHead := &InnerState{}
			if err := currentHead.UnmarshalVT(currentStateData); err != nil {
				t.Fatal(err.Error())
			}
			nextState, res, err := c.processOp(
				ctx,
				logrus.NewEntry(logrus.New()),
				so,
				ops[0].GetOpData(),
				ops[0].GetLocalId(),
				pid,
				ops[0].GetNonce(),
				0,
				currentHead,
			)
			if err != nil {
				return nil, nil, err
			}
			nextStateData, err := nextState.MarshalVT()
			if err != nil {
				return nil, nil, err
			}
			return &nextStateData, []*sobject.SOOperationResult{res}, nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rejectedOps) != 0 {
		t.Fatalf("expected no rejected ops, got %d", len(rejectedOps))
	}
	if len(acceptedOps) != 1 {
		t.Fatalf("expected 1 accepted op, got %d", len(acceptedOps))
	}
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(rootInner.GetStateData(), baseStateData) {
		t.Fatal("candidate processing must not update the SharedObject root before UpdateRootState")
	}

	if err := state.UpdateRootState(sharedObjectID, nextRoot, pid.String(), rejectedOps, acceptedOps); err != nil {
		t.Fatal(err.Error())
	}
	rootInner, err = snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Equal(rootInner.GetStateData(), baseStateData) {
		t.Fatal("SharedObject root state should change only after UpdateRootState accepts the candidate")
	}
}

func TestProcessOpDisabledMaintenanceDurablyRejectsGCSweep(t *testing.T) {
	ctx := context.Background()
	sharedObjectID := "test-disabled-gc-sweep"
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err.Error())
	}
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		t.Fatal(err.Error())
	}

	c, _, headState := newProcessTestWorld(t, ctx)
	stateData, err := headState.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	transformConf := newStateTestTransformConfig(t, &transform_gzip.Config{})
	grant, err := sobject.EncryptSOGrant(
		priv,
		pub,
		sharedObjectID,
		&sobject.SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{
		Logger: logrus.NewEntry(logrus.New()),
	}, sfs, transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootInnerData, err := (&sobject.SORootInner{
		Seqno:     1,
		StateData: stateData,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	encodedStateData, err := xfrm.EncodeBlock(rootInnerData)
	if err != nil {
		t.Fatal(err.Error())
	}

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: pid.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		Root: &sobject.SORoot{
			Inner:      encodedStateData,
			InnerSeqno: 1,
		},
		RootGrants: []*sobject.SOGrant{grant},
	}
	snap := sobject.NewSOStateParticipantHandle(
		logrus.NewEntry(logrus.New()),
		sfs,
		sharedObjectID,
		state,
		priv,
		pid,
	)

	gcTx, err := world_block_tx.NewTxGCSweep()
	if err != nil {
		t.Fatal(err.Error())
	}
	encodedOpData, err := xfrm.EncodeBlock(marshalApplyTxOpForProcessTest(t, gcTx))
	if err != nil {
		t.Fatal(err.Error())
	}
	op, err := sobject.BuildSOOperation(
		sharedObjectID,
		priv,
		encodedOpData,
		1,
		sobject.NewSOOperationLocalID(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := state.QueueOperation(sharedObjectID, op); err != nil {
		t.Fatal(err.Error())
	}

	nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
		ctx,
		[]*sobject.SOOperation{op},
		func(ctx context.Context, currentStateData []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
			if len(ops) != 1 {
				t.Fatalf("expected 1 op, got %d", len(ops))
			}
			currentHead := &InnerState{}
			if err := currentHead.UnmarshalVT(currentStateData); err != nil {
				t.Fatal(err.Error())
			}
			nextState, res, err := c.processOp(
				ctx,
				logrus.NewEntry(logrus.New()),
				nil,
				ops[0].GetOpData(),
				ops[0].GetLocalId(),
				pid,
				ops[0].GetNonce(),
				0,
				currentHead,
			)
			if err != nil {
				return nil, nil, err
			}
			if nextState != nil {
				t.Fatal("expected disabled sweep rejection to leave state unchanged")
			}
			return nil, []*sobject.SOOperationResult{res}, nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(rejectedOps) != 1 {
		t.Fatalf("expected 1 rejected op, got %d", len(rejectedOps))
	}
	if len(acceptedOps) != 0 {
		t.Fatalf("expected no accepted ops, got %d", len(acceptedOps))
	}

	if err := state.UpdateRootState(sharedObjectID, nextRoot, pid.String(), rejectedOps, acceptedOps); err != nil {
		t.Fatal(err.Error())
	}
	if len(state.GetOps()) != 0 {
		t.Fatalf("expected rejected sweep op to be cleared, got %d queued ops", len(state.GetOps()))
	}
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(rootInner.GetStateData()) != string(stateData) {
		t.Fatal("expected rejection to preserve world state data")
	}
}

func newProcessTestPeerID(t *testing.T) peer.ID {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	return pid
}

func marshalApplyTxOpForProcessTest(t *testing.T, tx *world_block_tx.Tx) []byte {
	t.Helper()
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	return opData
}

func newProcessTestWorld(t *testing.T, ctx context.Context) (*Controller, *testSharedObject, *InnerState) {
	t.Helper()
	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	transformConf := newStateTestTransformConfig(t, &transform_gzip.Config{})
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, tb.StepFactorySet, transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}

	store := newTestBlockStore(tb.EngineBucketID, tb.Volume)
	tx, bcs := block.NewTransaction(store, xfrm, nil, nil)
	bcs.SetBlock(world_block.NewWorld(true), true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	return &Controller{
			le:   tb.Logger,
			bus:  tb.Bus,
			conf: &Config{},
			sfs:  tb.StepFactorySet,
		},
		&testSharedObject{blockStore: store},
		&InnerState{HeadRef: &bucket.ObjectRef{RootRef: rootRef, TransformConf: transformConf}}
}
