package provider_spacewave

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const testSharedObjectID = "test-shared-object"

func TestApplyChangeLogEntryRootPrunesAcceptedAndRejectedOps(t *testing.T) {
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("validator peer: %v", err)
	}
	writer1, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("writer1 peer: %v", err)
	}
	writer2, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("writer2 peer: %v", err)
	}

	validatorPriv, err := validator.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("validator privkey: %v", err)
	}
	writer1Priv, err := writer1.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("writer1 privkey: %v", err)
	}
	writer2Priv, err := writer2.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("writer2 privkey: %v", err)
	}

	op1 := buildTestSOOperation(t, writer1Priv, 1)
	op2 := buildTestSOOperation(t, writer1Priv, 2)
	op3 := buildTestSOOperation(t, writer2Priv, 1)
	rejection := buildTestSOOperationRejection(t, validatorPriv, writer1.GetPeerID(), 2, op2)
	state := &sobject.SOState{
		Config: buildTestSharedObjectConfig(validator, writer1, writer2),
		Root:   buildTestSORoot(t, validatorPriv, 1, nil),
		Ops:    []*sobject.SOOperation{op1, op2, op3},
		OpRejections: []*sobject.SOPeerOpRejections{{
			PeerId:     writer1.GetPeerID().String(),
			Rejections: []*sobject.SOOperationRejection{rejection},
		}},
	}
	root := buildTestSORoot(t, validatorPriv, 2, []*sobject.SOAccountNonce{{
		PeerId: writer1.GetPeerID().String(),
		Nonce:  1,
	}})
	rootData, err := (&api.PostRootRequest{Root: root}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal post root request: %v", err)
	}

	err = applyChangeLogEntry(testSharedObjectID, state, &api.SOStateDeltaEntry{
		ChangeType: "root",
		ChangeData: rootData,
	})
	if err != nil {
		t.Fatalf("applyChangeLogEntry(root): %v", err)
	}
	if len(state.GetOps()) != 1 {
		t.Fatalf("expected 1 pending op after prune, got %d", len(state.GetOps()))
	}

	inner, err := state.GetOps()[0].UnmarshalInner()
	if err != nil {
		t.Fatalf("unmarshal remaining op: %v", err)
	}
	if inner.GetPeerId() != writer2.GetPeerID().String() || inner.GetNonce() != 1 {
		t.Fatalf("unexpected remaining op: peer=%s nonce=%d", inner.GetPeerId(), inner.GetNonce())
	}
}

func TestApplyChangeLogEntryRootIsIdempotentForMatchingRoot(t *testing.T) {
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("validator peer: %v", err)
	}
	validatorPriv, err := validator.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("validator privkey: %v", err)
	}
	root := buildTestSORoot(t, validatorPriv, 6, nil)
	reqData, err := (&api.PostRootRequest{Root: root}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal post root request: %v", err)
	}

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: validator.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			}},
		},
		Root: root.CloneVT(),
	}
	if err := applyChangeLogEntry(testSharedObjectID, state, &api.SOStateDeltaEntry{
		ChangeType: "root",
		ChangeData: reqData,
	}); err != nil {
		t.Fatalf("applyChangeLogEntry(root): %v", err)
	}
	if !state.GetRoot().EqualVT(root) {
		t.Fatal("expected matching root replay to leave cached root unchanged")
	}
}

func TestVerifyPulledStateIgnoresChangeLogSeqno(t *testing.T) {
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("validator peer: %v", err)
	}
	validatorPriv, err := validator.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("validator privkey: %v", err)
	}
	cachedRoot := buildTestSORoot(t, validatorPriv, 6, nil)
	h := &cloudSOHost{
		soID:     testSharedObjectID,
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](&sobject.SOState{Root: cachedRoot}),
		le:       logrus.New().WithField("test", t.Name()),
	}
	h.lastSeqno = 10

	next := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: validator.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			}},
		},
		Root: cachedRoot.CloneVT(),
	}
	if err := h.verifyPulledState(next); err != nil {
		t.Fatalf("verifyPulledState should ignore changelog seqno for rollback checks: %v", err)
	}
}

func TestVerifyChangeLogSeqnoUsesSnapshotCounter(t *testing.T) {
	h := &cloudSOHost{
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil),
		le:       logrus.New().WithField("test", t.Name()),
	}
	h.lastSeqno = 10

	if err := h.verifyChangeLogSeqno(6); err == nil {
		t.Fatal("expected changelog rollback error")
	}
	if err := h.verifyChangeLogSeqno(11); err != nil {
		t.Fatalf("expected changelog seqno 11 to be accepted: %v", err)
	}
}

func buildTestSharedObjectConfig(
	validator peer.Peer,
	writer1 peer.Peer,
	writer2 peer.Peer,
) *sobject.SharedObjectConfig {
	return &sobject.SharedObjectConfig{
		ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
		Participants: []*sobject.SOParticipantConfig{
			{
				PeerId: validator.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			},
			{
				PeerId: writer1.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
			},
			{
				PeerId: writer2.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
			},
		},
	}
}

func buildTestSOOperation(
	t *testing.T,
	privKey crypto.PrivKey,
	nonce uint64,
) *sobject.SOOperation {
	t.Helper()

	op, err := sobject.BuildSOOperation(
		testSharedObjectID,
		privKey,
		[]byte("op"),
		nonce,
		sobject.NewSOOperationLocalID(),
	)
	if err != nil {
		t.Fatalf("build op: %v", err)
	}
	return op
}

func buildTestSOOperationRejection(
	t *testing.T,
	validatorPrivKey crypto.PrivKey,
	submitterPeerID peer.ID,
	nonce uint64,
	op *sobject.SOOperation,
) *sobject.SOOperationRejection {
	t.Helper()

	inner, err := op.UnmarshalInner()
	if err != nil {
		t.Fatalf("unmarshal op inner: %v", err)
	}
	rejection, err := sobject.BuildSOOperationRejection(
		validatorPrivKey,
		testSharedObjectID,
		submitterPeerID,
		nonce,
		inner.GetLocalId(),
		nil,
	)
	if err != nil {
		t.Fatalf("build rejection: %v", err)
	}
	return rejection
}

func buildTestSORoot(
	t *testing.T,
	validatorPrivKey crypto.PrivKey,
	seqno uint64,
	accountNonces []*sobject.SOAccountNonce,
) *sobject.SORoot {
	t.Helper()

	innerData, err := (&sobject.SORootInner{
		Seqno:     seqno,
		StateData: []byte("state"),
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root inner: %v", err)
	}
	root := &sobject.SORoot{
		Inner:         innerData,
		InnerSeqno:    seqno,
		AccountNonces: accountNonces,
	}
	if err := root.SignInnerData(
		validatorPrivKey,
		testSharedObjectID,
		seqno,
		hash.HashType_HashType_BLAKE3,
	); err != nil {
		t.Fatalf("sign root: %v", err)
	}
	return root
}
