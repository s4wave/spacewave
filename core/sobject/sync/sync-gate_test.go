package sobject_sync

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	ulid "github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
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

// newMemHost builds an SOHost backed by a state container and an optional successful-write observer.
func newMemHost(soID string, initial *sobject.SOState, onWrite ...func()) (*sobject.SOHost, *ccontainer.CContainer[*sobject.SOState]) {
	if initial == nil {
		initial = &sobject.SOState{}
	}
	ctr := ccontainer.NewCContainerVT[*sobject.SOState](initial)
	watchFn := func(_ context.Context, _ string, _ func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		return ctr, func() {}, nil
	}
	var mutex csync.Mutex
	history := make(map[string]*sobject.SOConfigChange)
	lockFn := func(ctx context.Context, _ string) (sobject.SOStateLock, error) {
		release, err := mutex.Lock(ctx)
		if err != nil {
			return nil, err
		}
		return sobject.NewSOStateLock(ctr.GetValue(), func(_ context.Context, state *sobject.SOState, changes ...*sobject.SOConfigChange) error {
			for _, change := range changes {
				hash, err := sobject.HashSOConfigChange(change)
				if err != nil {
					return err
				}
				history[string(hash)] = change.CloneVT()
			}
			ctr.SetValue(state)
			if len(onWrite) != 0 {
				onWrite[0]()
			}
			return nil
		}, release), nil
	}
	syncFuncs := &sobject.SOHostSyncFuncs{History: func(ctx context.Context, _ string, base, target []byte) ([]*sobject.SOConfigChange, error) {
		release, err := mutex.Lock(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		return sobject.ReadConfigSuffix(ctx, base, target, func(_ context.Context, hash []byte) (*sobject.SOConfigChange, error) {
			return history[string(hash)], nil
		})
	}}
	return sobject.NewSOHost(context.Background(), watchFn, lockFn, soID, syncFuncs), ctr
}

// mustKeyPair generates a real participant signing key.
func mustKeyPair(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	return priv
}

// mustPeerIDStr encodes the identity belonging to the signing key.
func mustPeerIDStr(t *testing.T, priv crypto.PrivKey) string {
	t.Helper()
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err.Error())
	}
	return id.String()
}

// participantCfg constructs a participant with the requested role.
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

// runSnapshotExchange drives the authenticated data protocol with a requested candidate.
// Authentication itself is covered by runStream tests; this helper isolates host rejection.
func runSnapshotExchange(t *testing.T, s *SOSync, ctx context.Context, peerSnap *SOSyncMessage) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	localSess, remoteSess := pipeSessions(t)
	done := make(chan error, 1)
	go func() { done <- s.synchronize(ctx, gateLogger(), localSess, s.localObjectPeerID) }()
	defer remoteSess.Close()

	// Decline the local advertisement, then request adoption of the supplied candidate.
	localHead := &SOSyncMessage{}
	if err := remoteSess.RecvMsg(localHead); err != nil {
		return <-done
	}
	if err := remoteSess.SendMsg(syncAcknowledgment(localHead.GetHead().GetRevision())); err != nil {
		return <-done
	}
	state := &sobject.SOState{}
	if err := state.UnmarshalVT(peerSnap.GetSnapshot().GetSoState()); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(peerSnap.GetSnapshot().GetSoState())
	head := &SOSyncHead{Revision: 1, StateHash: digest[:], ConfigHash: state.GetConfig().GetConfigChainHash(), ConfigSeqno: state.GetConfig().GetConfigChainSeqno(), RootSeqno: peerSnap.GetSnapshot().GetRootSeqno()}
	if err := remoteSess.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Head{Head: head}}); err != nil {
		return <-done
	}
	request := &SOSyncMessage{}
	if err := remoteSess.RecvMsg(request); err != nil {
		return <-done
	}
	if request.GetHistoryRequest() == nil {
		cancel()
		remoteSess.Close()
		<-done
		return errors.New("candidate not requested")
	}
	snapshot := peerSnap.GetSnapshot().CloneVT()
	snapshot.Revision = 1
	snapshot.BaseHash = request.GetHistoryRequest().GetBaseHash()
	if err := remoteSess.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: snapshot}}); err != nil {
		return <-done
	}
	ack := &SOSyncMessage{}
	if err := remoteSess.RecvMsg(ack); err != nil {
		return <-done
	}
	if ack.GetAck().GetRevision() != 1 {
		t.Fatalf("unexpected snapshot acknowledgment: %v", ack)
	}
	cancel()
	remoteSess.Close()
	<-done
	return nil
}

func TestSnapshotExchangeRejectsExcludedLocalPeer(t *testing.T) {
	ctx := context.Background()
	soID := "gate-object"
	localPriv := mustKeyPair(t)
	localPeerStr := mustPeerIDStr(t, localPriv)
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err)
	}
	ownerPriv := mustKeyPair(t)
	ownerPeerStr := mustPeerIDStr(t, ownerPriv)

	held := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(ownerPeerStr, sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(localPeerStr, sobject.SOParticipantRole_SOParticipantRole_READER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	trustSnapshotConfig(t, held, ownerPriv)
	localHost, ctr := newMemHost(soID, held)
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil)

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
	ownerPriv := mustKeyPair(t)
	ownerPeer := mustPeerIDStr(t, ownerPriv)
	held := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(ownerPeer, sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	trustSnapshotConfig(t, held, ownerPriv)
	localHost, ctr := newMemHost(soID, held)
	validateAccess := func(_ context.Context, state *sobject.SOState) error {
		for _, grant := range state.GetRootGrants() {
			if grant.GetPeerId() == localPeer.String() {
				return nil
			}
		}
		return errors.New("no local root grant")
	}
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil, validateAccess)
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

	held := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(ownerPeerStr, sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	trustSnapshotConfig(t, held, ownerPriv)
	localHost, ctr := newMemHost(soID, held)
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil)

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
	trustSnapshotConfig(t, localState, ownerPriv)
	if err := localState.QueueOperation(soID, pending); err != nil {
		t.Fatal(err.Error())
	}
	localHost, ctr := newMemHost(soID, localState)
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil)

	peerState := &sobject.SOState{
		Config:     localState.GetConfig().CloneVT(),
		Root:       &sobject.SORoot{InnerSeqno: 5},
		RootGrants: []*sobject.SOGrant{grant},
	}
	signSnapshotRoot(t, soID, peerState, ownerPriv)
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

// TestPeerImportDropsDemotedWriterQueue exercises queue merging under verified authority.
func TestPeerImportDropsDemotedWriterQueue(t *testing.T) {
	const soID = "gate-object-writer-demotion"
	owner, writer := mustKeyPair(t), mustKeyPair(t)
	writerID, err := peer.IDFromPrivateKey(writer)
	if err != nil {
		t.Fatal(err)
	}
	previous := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(mustPeerIDStr(t, owner), sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(writerID.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER),
		}},
		Root: &sobject.SORoot{InnerSeqno: 1},
	}
	trustSnapshotConfig(t, previous, owner)
	signSnapshotRoot(t, soID, previous, owner)
	operation, err := sobject.BuildSOOperation(soID, writer, []byte("pending-before-demotion"), 1, ulid.NewULID())
	if err != nil {
		t.Fatal(err)
	}
	if err := previous.QueueOperation(soID, operation); err != nil {
		t.Fatal(err)
	}

	// The owner demotes the writer without changing the accepted root.
	candidate := previous.CloneVT()
	candidate.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_READER
	change, err := sobject.BuildSOConfigChange(previous.Config, candidate.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Config, err = sobject.VerifyConfigChange(previous.Config, change)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Ops = nil
	candidate.QueuedAccountNonces = nil
	host, state := newMemHost(soID, previous)
	t.Cleanup(host.ClearContext)
	if err := host.ImportPeerSnapshot(t.Context(), candidate, []*sobject.SOConfigChange{change}, writerID, nil); err != nil {
		t.Fatal(err)
	}
	if len(state.GetValue().GetOps()) != 0 || state.GetValue().GetConfig().GetParticipants()[1].GetRole() != sobject.SOParticipantRole_SOParticipantRole_READER {
		t.Fatal("demoted writer's pending operation survived import")
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
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, host, nil)

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
			s := NewSOSync(gateLogger(), nil, soID, writerPeer, writerPriv, host, nil)
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
	s := NewSOSync(gateLogger(), nil, soID, writerPeer, writerPriv, host, nil)

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

// trustSnapshotConfig establishes a signed genesis checkpoint already held locally.
func trustSnapshotConfig(t *testing.T, state *sobject.SOState, owner crypto.PrivKey) {
	t.Helper()
	entry, err := sobject.BuildSOConfigChange(state.GetConfig(), state.GetConfig(), sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = sobject.VerifyConfigChange(state.GetConfig(), entry)
	if err != nil {
		t.Fatal(err)
	}
}

// signSnapshotRoot signs the candidate sequence and content with a real participant key.
func signSnapshotRoot(t *testing.T, soID string, state *sobject.SOState, signer crypto.PrivKey) {
	t.Helper()
	state.Root.Inner = []byte("snapshot-root")
	state.Root.ValidatorSignatures = nil
	if err := state.Root.SignInnerData(signer, soID, state.Root.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
}
