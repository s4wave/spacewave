package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
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
	// Build the operation author and an object creation transaction.
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	tx, err := world_block_tx.NewTxCreateObject("object", &bucket.ObjectRef{})
	if err != nil {
		t.Fatalf("build tx: %v", err)
	}

	// Encode the transaction as an ApplyTx operation.
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}

	// Process the operation against an uninitialized World.
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

// TestProcessInitWorldOpWritesDisabledChangelogRoot checks that an init op
// disabling the changelog writes a World root that records it.
func TestProcessInitWorldOpWritesDisabledChangelogRoot(t *testing.T) {
	// Start a testbed.
	ctx := context.Background()
	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Encode an init op that disables the changelog.
	store := newTestBlockStore(tb.EngineBucketID, tb.Volume)
	pid := newProcessTestPeerID(t)
	initOp, err := NewInitWorldOp(&InitWorldOp{LastChangeDisable: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	opData, err := (&SOWorldOp{Body: &SOWorldOp_InitWorld{InitWorld: initOp}}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Process it from an empty state.
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

	// The written World root disables the changelog.
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

// TestProcessOpAppliesOrdinaryTx checks that replay applies an ordinary
// World transaction and produces the next World state.
func TestProcessOpAppliesOrdinaryTx(t *testing.T) {
	// Build the World and an object creation transaction.
	ctx := context.Background()
	pid := newProcessTestPeerID(t)
	c, so, headState := newProcessTestWorld(t, ctx)
	objectRef := headState.GetHeadRef().CloneVT()
	objectTx, err := world_block_tx.NewTxCreateObject("ordinary-object", objectRef)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Process the transaction and check it produced a new head.
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

// newProcessTestPeerID returns a fresh peer ID.
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
