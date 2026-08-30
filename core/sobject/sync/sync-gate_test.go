package sobject_sync

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	ulid "github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// gateLogger returns a logger that discards output.
func gateLogger() *logrus.Entry {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return logrus.NewEntry(log)
}

// newMemHost builds an SOHost backed by an in-memory state container.
func newMemHost(soID string, initial *sobject.SOState) (*sobject.SOHost, *ccontainer.CContainer[*sobject.SOState]) {
	if initial == nil {
		initial = &sobject.SOState{}
	}
	ctr := ccontainer.NewCContainerVT[*sobject.SOState](initial)
	watchFn := func(_ context.Context, _ string, _ func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		return ctr, func() {}, nil
	}
	lockFn := func(_ context.Context, _ string) (sobject.SOStateLock, error) {
		return sobject.NewSOStateLock(ctr.GetValue(), func(_ context.Context, s *sobject.SOState) error {
			ctr.SetValue(s)
			return nil
		}, func() {}), nil
	}
	return sobject.NewSOHost(context.Background(), watchFn, lockFn, soID), ctr
}

func mustKeyPair(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return priv
}

func mustPeerIDStr(t *testing.T, priv crypto.PrivKey) string {
	t.Helper()
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err.Error())
	}
	return id.String()
}

func participantCfg(peerIDStr string, role sobject.SOParticipantRole) *sobject.SOParticipantConfig {
	return &sobject.SOParticipantConfig{PeerId: peerIDStr, Role: role}
}

// pipeSessions builds a paired send/receive packet session.
func pipeSessions(t *testing.T) (local *stream_packet.Session, remote *stream_packet.Session) {
	t.Helper()
	left, right := net.Pipe()
	t.Cleanup(func() {
		left.Close()
		right.Close()
	})
	return stream_packet.NewSession(left, maxMessageSize), stream_packet.NewSession(right, maxMessageSize)
}

// buildGrant constructs a valid transform grant from the owner to the local peer.
func buildGrant(t *testing.T, soID string, ownerPriv crypto.PrivKey, localPub crypto.PubKey) *sobject.SOGrant {
	t.Helper()
	grant, err := sobject.EncryptSOGrant(ownerPriv, localPub, soID, &sobject.SOGrantInner{
		TransformConf: &block_transform.Config{
			Steps: []*block_transform.StepConfig{{
				Id: transform_blockenc.ConfigID,
				Config: func() []byte {
					cfg := &transform_blockenc.Config{
						BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
						Key:      []byte("0123456789abcdef0123456789abcdef"),
					}
					data, err := cfg.MarshalVT()
					if err != nil {
						t.Fatal(err.Error())
					}
					return data
				}(),
			}},
		},
	})
	if err != nil {
		t.Fatalf("EncryptSOGrant: %v", err)
	}
	return grant
}

// runSnapshotExchange drives exchangeSnapshots with the peer snapshot as the
// remote side and returns any error from the local side.
func runSnapshotExchange(t *testing.T, s *SOSync, ctx context.Context, peerSnap *SOSyncMessage) error {
	t.Helper()
	localSess, remoteSess := pipeSessions(t)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.exchangeSnapshots(ctx, gateLogger(), localSess)
	}()

	// The local side sends its snapshot first; consume it.
	in := &SOSyncMessage{}
	if err := remoteSess.RecvMsg(in); err != nil {
		t.Fatalf("remote recv local snapshot: %v", err)
	}
	if err := remoteSess.SendMsg(peerSnap); err != nil {
		t.Fatalf("remote send snapshot: %v", err)
	}
	return <-errCh
}

func TestStreamOpsAppliesNewerSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	ownerPriv := mustKeyPair(t)
	writerPriv := mustKeyPair(t)
	ownerID := mustPeerIDStr(t, ownerPriv)
	writerID := mustPeerIDStr(t, writerPriv)
	participants := []*sobject.SOParticipantConfig{
		participantCfg(ownerID, sobject.SOParticipantRole_SOParticipantRole_OWNER),
		participantCfg(writerID, sobject.SOParticipantRole_SOParticipantRole_WRITER),
	}
	initial := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: participants},
		Root:   &sobject.SORoot{InnerSeqno: 1},
	}
	newer := initial.CloneVT()
	newer.Root.InnerSeqno = 2
	newerData, err := newer.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	host, stateCtr := newMemHost("stream-newer-snapshot", initial)
	writerPeerID, err := peer.IDFromPrivateKey(writerPriv)
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewSOSync(gateLogger(), nil, "stream-newer-snapshot", writerPeerID, host)
	localSess, remoteSess := pipeSessions(t)
	streamDone := make(chan struct{})
	go func() {
		syncer.streamOps(ctx, gateLogger(), localSess)
		close(streamDone)
	}()

	if err := remoteSess.SendMsg(&SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{
			SoState:   newerData,
			RootSeqno: 2,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := stateCtr.WaitValueChange(ctx, initial, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.GetRoot().GetInnerSeqno(); got != 2 {
		t.Fatalf("root seqno = %d, want 2", got)
	}

	// The local sender publishes the adopted root so both directions keep
	// following authoritative state after the initial exchange.
	for {
		out := &SOSyncMessage{}
		if err := remoteSess.RecvMsg(out); err != nil {
			t.Fatal(err)
		}
		if out.GetSnapshot().GetRootSeqno() == 2 {
			break
		}
	}
	cancel()
	if err := remoteSess.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
}

func TestSnapshotExchangeRejectsExcludedLocalPeer(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object"
	localPriv := mustKeyPair(t)
	localPeerStr := mustPeerIDStr(t, localPriv)
	ownerPriv := mustKeyPair(t)
	ownerPeerStr := mustPeerIDStr(t, ownerPriv)

	localHost, ctr := newMemHost(soID, &sobject.SOState{
		Config: &sobject.SharedObjectConfig{},
		Root:   &sobject.SORoot{InnerSeqno: 1},
	})
	s := NewSOSync(gateLogger(), nil, soID, peer.ID(localPeerStr), localHost)

	peerState := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{participantCfg(ownerPeerStr, sobject.SOParticipantRole_SOParticipantRole_OWNER)},
		},
		Root: &sobject.SORoot{InnerSeqno: 5},
	}
	snapData, err := peerState.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	err = runSnapshotExchange(t, s, ctx, &SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: snapData, RootSeqno: 5}},
	})
	if err == nil {
		t.Fatal("expected snapshot from config excluding the local peer to be rejected")
	}
	if got := ctr.GetValue().GetRoot().GetInnerSeqno(); got != 1 {
		t.Fatalf("local state advanced to seqno %d; expected rejection to keep seqno 1", got)
	}
}

func TestSnapshotExchangeRejectsSnapshotWithoutLocalGrant(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object-local-grant"
	localPriv := mustKeyPair(t)
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	ownerPeer := mustPeerIDStr(t, mustKeyPair(t))
	localHost, ctr := newMemHost(soID, &sobject.SOState{
		Config: &sobject.SharedObjectConfig{},
		Root:   &sobject.SORoot{InnerSeqno: 1},
	})
	validateAccess := func(_ context.Context, state *sobject.SOState) error {
		for _, grant := range state.GetRootGrants() {
			if grant.GetPeerId() == localPeer.String() {
				return nil
			}
		}
		return errors.New("no local root grant")
	}
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localHost, validateAccess)
	peerState := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(ownerPeer, sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 5},
	}
	snapData, err := peerState.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	err = runSnapshotExchange(t, s, ctx, &SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: snapData, RootSeqno: 5}},
	})
	if err == nil {
		t.Fatal("expected inaccessible snapshot to be rejected")
	}
	if got := ctr.GetValue().GetRoot().GetInnerSeqno(); got != 1 {
		t.Fatalf("local state advanced to seqno %d; expected rejection to keep seqno 1", got)
	}
}

func TestSnapshotExchangeRejectsTamperedGrant(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object-grant"
	localPriv, localPub, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	ownerPriv := mustKeyPair(t)
	ownerPeerStr := mustPeerIDStr(t, ownerPriv)

	localHost, ctr := newMemHost(soID, &sobject.SOState{
		Config: &sobject.SharedObjectConfig{},
		Root:   &sobject.SORoot{InnerSeqno: 1},
	})
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localHost)

	grant := buildGrant(t, soID, ownerPriv, localPub)
	grant.InnerData[0] ^= 0xFF

	peerState := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				participantCfg(ownerPeerStr, sobject.SOParticipantRole_SOParticipantRole_OWNER),
				participantCfg(localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER),
			},
		},
		Root:       &sobject.SORoot{InnerSeqno: 5},
		RootGrants: []*sobject.SOGrant{grant},
	}
	snapData, err := peerState.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	err = runSnapshotExchange(t, s, ctx, &SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: snapData, RootSeqno: 5}},
	})
	if err == nil {
		t.Fatal("expected snapshot with tampered root grant to be rejected")
	}
	if got := ctr.GetValue().GetRoot().GetInnerSeqno(); got != 1 {
		t.Fatalf("local state advanced to seqno %d; expected rejection to keep seqno 1", got)
	}
}

func TestSnapshotExchangeAcceptsObjectPeerDistinctFromTransportPeer(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object-valid"
	transportPeer := mustPeerIDStr(t, mustKeyPair(t))
	localPriv, localPub, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	if localPeer.String() == transportPeer {
		t.Fatal("local object and transport peers unexpectedly match")
	}
	ownerPriv := mustKeyPair(t)
	ownerPeerStr := mustPeerIDStr(t, ownerPriv)
	participants := []*sobject.SOParticipantConfig{
		participantCfg(ownerPeerStr, sobject.SOParticipantRole_SOParticipantRole_OWNER),
		participantCfg(localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
	}
	grant := buildGrant(t, soID, ownerPriv, localPub)
	pendingID := ulid.NewULID()
	pending, err := sobject.BuildSOOperation(soID, localPriv, []byte("pending-local-write"), 1, pendingID)
	if err != nil {
		t.Fatal(err.Error())
	}
	localState := &sobject.SOState{
		Config:     &sobject.SharedObjectConfig{Participants: participants},
		Root:       &sobject.SORoot{InnerSeqno: 1},
		RootGrants: []*sobject.SOGrant{grant},
	}
	if err := localState.QueueOperation(soID, pending); err != nil {
		t.Fatal(err.Error())
	}
	localHost, ctr := newMemHost(soID, localState)
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localHost)

	peerState := &sobject.SOState{
		Config:     &sobject.SharedObjectConfig{Participants: participants},
		Root:       &sobject.SORoot{InnerSeqno: 5},
		RootGrants: []*sobject.SOGrant{grant},
	}
	snapData, err := peerState.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := runSnapshotExchange(t, s, ctx, &SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: snapData, RootSeqno: 5}},
	}); err != nil {
		t.Fatalf("validated snapshot should converge: %v", err)
	}
	got := ctr.GetValue()
	if got.GetRoot().GetInnerSeqno() != 5 {
		t.Fatalf("expected converged root seqno 5, got %d", got.GetRoot().GetInnerSeqno())
	}
	if len(got.GetConfig().GetParticipants()) != 2 {
		t.Fatalf("expected applied config participants, got %d", len(got.GetConfig().GetParticipants()))
	}
	if len(got.GetOps()) != 1 {
		t.Fatalf("pending local write count = %d, want 1", len(got.GetOps()))
	}
	inner, err := got.GetOps()[0].UnmarshalInner()
	if err != nil {
		t.Fatal(err.Error())
	}
	if inner.GetLocalId() != pendingID {
		t.Fatalf("pending local write = %q, want %q", inner.GetLocalId(), pendingID)
	}
}

func TestMergePendingOperationsAcceptsAuthoritativeWriterDemotion(t *testing.T) {
	soID := "gate-object-writer-demotion"
	writerPriv := mustKeyPair(t)
	writerPeer, err := peer.IDFromPrivateKey(writerPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	previous := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(writerPeer.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	operation, err := sobject.BuildSOOperation(soID, writerPriv, []byte("pending-before-demotion"), 1, ulid.NewULID())
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := previous.QueueOperation(soID, operation); err != nil {
		t.Fatal(err.Error())
	}
	authoritative := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(writerPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 2},
	}

	if err := mergePendingOperations(gateLogger(), soID, authoritative, previous); err != nil {
		t.Fatal(err.Error())
	}
	if len(authoritative.GetOps()) != 0 {
		t.Fatalf("authoritative queue contains %d demoted-writer operations, want 0", len(authoritative.GetOps()))
	}
	if got := authoritative.GetConfig().GetParticipants()[0].GetRole(); got != sobject.SOParticipantRole_SOParticipantRole_READER {
		t.Fatalf("authoritative writer role = %s, want READER", got.String())
	}
}

func TestRemoteOpNonparticipantRejected(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object-op"
	localPriv := mustKeyPair(t)
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err.Error())
	}
	strangerPriv := mustKeyPair(t)

	ownerPriv := mustKeyPair(t)
	ownerPeerStr := mustPeerIDStr(t, ownerPriv)

	host, ctr := newMemHost(soID, &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				participantCfg(ownerPeerStr, sobject.SOParticipantRole_SOParticipantRole_OWNER),
				participantCfg(localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
			},
		},
		Root: &sobject.SORoot{InnerSeqno: 1},
	})
	s := NewSOSync(gateLogger(), nil, soID, localPeer, host)

	opLocalID := ulid.NewULID()
	op, err := sobject.BuildSOOperation(soID, strangerPriv, []byte("op-data"), 1, opLocalID)
	if err != nil {
		t.Fatal(err.Error())
	}
	s.handleRemoteOp(ctx, gateLogger(), &SOSyncOp{Operation: func() []byte {
		data, err := op.MarshalVT()
		if err != nil {
			t.Fatal(err.Error())
		}
		return data
	}()})

	if got := len(ctr.GetValue().GetOps()); got != 0 {
		t.Fatalf("nonparticipant op was queued (%d ops)", got)
	}
}

func TestRemoteOpReplayIsIdempotent(t *testing.T) {
	for _, test := range []struct {
		name       string
		rootNonce  uint64
		queueFirst bool
	}{
		{name: "applied root", rootNonce: 1},
		{name: "pending queue", queueFirst: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			soID := "gate-object-op-replay"
			writerPriv := mustKeyPair(t)
			writerPeer, err := peer.IDFromPrivateKey(writerPriv)
			if err != nil {
				t.Fatal(err.Error())
			}
			state := &sobject.SOState{
				Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
					participantCfg(writerPeer.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
				}},
				Root: &sobject.SORoot{InnerSeqno: 2},
			}
			if test.rootNonce != 0 {
				state.Root.AccountNonces = []*sobject.SOAccountNonce{{
					PeerId: writerPeer.String(), Nonce: test.rootNonce,
				}}
			}
			op, err := sobject.BuildSOOperation(soID, writerPriv, []byte("replayed"), 1, ulid.NewULID())
			if err != nil {
				t.Fatal(err.Error())
			}
			if test.queueFirst {
				if err := state.QueueOperation(soID, op); err != nil {
					t.Fatal(err.Error())
				}
			}
			host, ctr := newMemHost(soID, state)
			s := NewSOSync(gateLogger(), nil, soID, writerPeer, host)
			opData, err := op.MarshalVT()
			if err != nil {
				t.Fatal(err.Error())
			}
			s.handleRemoteOp(ctx, gateLogger(), &SOSyncOp{Operation: opData})
			wantOps := 0
			if test.queueFirst {
				wantOps = 1
			}
			if got := len(ctr.GetValue().GetOps()); got != wantOps {
				t.Fatalf("operation queue length = %d, want %d", got, wantOps)
			}
		})
	}
}

func TestRemoteOpTamperedSignatureRejected(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object-optamper"
	writerPriv := mustKeyPair(t)
	writerPeer, err := peer.IDFromPrivateKey(writerPriv)
	if err != nil {
		t.Fatal(err.Error())
	}

	host, ctr := newMemHost(soID, &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{
				participantCfg(writerPeer.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
			},
		},
		Root: &sobject.SORoot{InnerSeqno: 1},
	})
	s := NewSOSync(gateLogger(), nil, soID, writerPeer, host)

	opLocalID := ulid.NewULID()
	op, err := sobject.BuildSOOperation(soID, writerPriv, []byte("op-data"), 1, opLocalID)
	if err != nil {
		t.Fatal(err.Error())
	}
	op.Inner[0] ^= 0xFF

	s.handleRemoteOp(ctx, gateLogger(), &SOSyncOp{Operation: func() []byte {
		data, err := op.MarshalVT()
		if err != nil {
			t.Fatal(err.Error())
		}
		return data
	}()})

	if got := len(ctr.GetValue().GetOps()); got != 0 {
		t.Fatalf("tampered op was queued (%d ops)", got)
	}
}
