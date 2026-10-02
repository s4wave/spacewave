package sobject

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/sirupsen/logrus"
)

func TestSOStateParticipantHandleProcessOperationsBlankRoot(t *testing.T) {
	// Use one owner key.
	ctx := context.Background()
	p := createMockPeers(t, 1)[0]
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatalf("get peer privkey: %v", err)
	}
	pub, err := p.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract peer pubkey: %v", err)
	}

	// Grant the owner a root encryption key.
	transformConf := &block_transform.Config{
		Steps: []*block_transform.StepConfig{{
			Id: transform_blockenc.ConfigID,
			Config: mustMarshalVT(t, &transform_blockenc.Config{
				BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
				Key:      []byte("0123456789abcdef0123456789abcdef"),
			}),
		}},
	}
	grant, err := EncryptSOGrant(
		priv,
		pub,
		mockSharedObjectID,
		&SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		t.Fatalf("EncryptSOGrant: %v", err)
	}

	// Build the block transformer from the grant config.
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	le := logrus.New().WithField("test", t.Name())
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		transformConf,
	)
	if err != nil {
		t.Fatalf("NewTransformer: %v", err)
	}

	// Sign an encrypted operation at the first nonce.
	opData := []byte("init world")
	opDataEnc, err := xfrm.EncodeBlock(opData)
	if err != nil {
		t.Fatalf("EncodeBlock: %v", err)
	}
	op, err := BuildSOOperation(mockSharedObjectID, priv, opDataEnc, linkAt(nil, priv, 1), NewSOOperationLocalID())
	if err != nil {
		t.Fatalf("BuildSOOperation: %v", err)
	}

	// Hold the owner state with the grant and no root.
	state := &SOState{
		Config: &SharedObjectConfig{
			Participants: []*SOParticipantConfig{{
				PeerId: p.GetPeerID().String(),
				Role:   SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		RootGrants: []*SOGrant{grant},
	}
	snap := NewSOStateParticipantHandle(le, sfs, mockSharedObjectID, state, priv, p.GetPeerID())

	// Process the operation against the blank root.
	nextStateData := []byte("next state")
	nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
		ctx,
		[]*SOOperation{op},
		func(ctx context.Context, currentStateData []byte, ops []*SOOperationInner) (*[]byte, []*SOOperationResult, error) {
			if len(currentStateData) != 0 {
				t.Fatalf("expected blank current state, got %q", currentStateData)
			}
			if len(ops) != 1 {
				t.Fatalf("expected 1 decoded op, got %d", len(ops))
			}
			if !bytes.Equal(ops[0].GetOpData(), opData) {
				t.Fatalf("unexpected op data: %q", ops[0].GetOpData())
			}
			return &nextStateData, []*SOOperationResult{
				BuildSOOperationResult(p.GetPeerID().String(), 1, true, nil),
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("ProcessOperations: %v", err)
	}

	// The owner accepts it in a root it signs at seqno 1.
	if len(rejectedOps) != 0 {
		t.Fatalf("expected no rejections, got %d", len(rejectedOps))
	}
	if len(acceptedOps) != 1 {
		t.Fatalf("expected 1 accepted op, got %d", len(acceptedOps))
	}
	if nextRoot == nil {
		t.Fatal("expected non-nil next root")
	}
	if nextRoot.GetInnerSeqno() != 1 {
		t.Fatalf("expected inner seqno 1, got %d", nextRoot.GetInnerSeqno())
	}
	if len(nextRoot.GetValidatorSignatures()) != 1 {
		t.Fatalf("expected 1 validator signature, got %d", len(nextRoot.GetValidatorSignatures()))
	}
	if len(nextRoot.GetAccountNonces()) != 1 {
		t.Fatalf("expected 1 account nonce, got %d", len(nextRoot.GetAccountNonces()))
	}
	if nextRoot.GetAccountNonces()[0].GetNonce() != 1 {
		t.Fatalf("expected accepted nonce 1, got %d", nextRoot.GetAccountNonces()[0].GetNonce())
	}

	// The root inner holds the next state.
	innerDataDec, err := xfrm.DecodeBlock(nextRoot.GetInner())
	if err != nil {
		t.Fatalf("DecodeBlock(next root): %v", err)
	}
	rootInner := &SORootInner{}
	if err := rootInner.UnmarshalVT(innerDataDec); err != nil {
		t.Fatalf("UnmarshalVT(next root inner): %v", err)
	}
	if rootInner.GetSeqno() != 1 {
		t.Fatalf("expected root inner seqno 1, got %d", rootInner.GetSeqno())
	}
	if !bytes.Equal(rootInner.GetStateData(), nextStateData) {
		t.Fatalf("unexpected next state data: %q", rootInner.GetStateData())
	}

	// The owner signature validates.
	validSigs, err := nextRoot.ValidateSignatures(mockSharedObjectID, state.GetConfig().GetParticipants())
	if err != nil {
		t.Fatalf("ValidateSignatures: %v", err)
	}
	if validSigs != 1 {
		t.Fatalf("expected 1 valid signature, got %d", validSigs)
	}
}

// TestSOStateParticipantHandleProcessOperationsAllRejected verifies that a
// batch whose operations all fail to decode advances the root so the
// rejections commit and the queue drains.
func TestSOStateParticipantHandleProcessOperationsAllRejected(t *testing.T) {
	// Use one owner key.
	ctx := context.Background()
	p := createMockPeers(t, 1)[0]
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := p.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}

	// Grant the owner a root encryption key.
	transformConf := &block_transform.Config{
		Steps: []*block_transform.StepConfig{{
			Id: transform_blockenc.ConfigID,
			Config: mustMarshalVT(t, &transform_blockenc.Config{
				BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
				Key:      []byte("0123456789abcdef0123456789abcdef"),
			}),
		}},
	}
	grant, err := EncryptSOGrant(priv, pub, mockSharedObjectID, &SOGrantInner{TransformConf: transformConf})
	if err != nil {
		t.Fatal(err)
	}

	// Build a transformer that cannot decode plaintext.
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	le := logrus.New().WithField("test", t.Name())

	// Queue an unencrypted owner operation.
	state := &SOState{
		Config: &SharedObjectConfig{Participants: []*SOParticipantConfig{{
			PeerId: p.GetPeerID().String(),
			Role:   SOParticipantRole_SOParticipantRole_OWNER,
		}}},
		RootGrants: []*SOGrant{grant},
	}
	op, err := BuildSOOperation(mockSharedObjectID, priv, []byte("not encrypted"), linkAt(nil, priv, 1), NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.QueueOperation(mockSharedObjectID, op); err != nil {
		t.Fatal(err)
	}

	// Processing rejects it and still advances the root.
	snap := NewSOStateParticipantHandle(le, sfs, mockSharedObjectID, state, priv, p.GetPeerID())
	nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
		ctx,
		state.GetOps(),
		func(ctx context.Context, currentStateData []byte, ops []*SOOperationInner) (*[]byte, []*SOOperationResult, error) {
			if len(ops) != 0 {
				t.Fatalf("expected no decoded ops, got %d", len(ops))
			}
			return nil, nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejectedOps) != 1 || len(acceptedOps) != 0 {
		t.Fatalf("expected 1 rejection and 0 accepted, got %d and %d", len(rejectedOps), len(acceptedOps))
	}
	if nextRoot.GetInnerSeqno() != 1 {
		t.Fatalf("expected inner seqno 1, got %d", nextRoot.GetInnerSeqno())
	}
	if err := state.UpdateRootState(mockSharedObjectID, nextRoot, p.GetPeerID().String(), rejectedOps, acceptedOps); err != nil {
		t.Fatalf("UpdateRootState: %v", err)
	}
	if len(state.GetOps()) != 0 {
		t.Fatalf("expected drained queue, got %d ops", len(state.GetOps()))
	}
	if len(state.GetOpRejections()) != 1 {
		t.Fatalf("expected committed rejection, got %d groups", len(state.GetOpRejections()))
	}
}
