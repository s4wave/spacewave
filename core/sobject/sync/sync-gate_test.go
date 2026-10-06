package sobject_sync

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"strconv"
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

	// Check the condition before continuing.
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

	// helper.
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

	// sum256 digest via sha256.
	digest := sha256.Sum256(peerSnap.GetSnapshot().GetSoState())
	head := &SOSyncHead{Revision: 1, StateHash: digest[:], ConfigHash: state.GetConfig().GetConfigChainHash(), ConfigSeqno: state.GetConfig().GetConfigChainSeqno()}
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

	// getSnapshot snapshot via peerSnap.
	snapshot := peerSnap.GetSnapshot().CloneVT()
	snapshot.Revision = 1
	snapshot.BaseHash = request.GetHistoryRequest().GetBaseHash()
	if err := remoteSess.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: snapshot}}); err != nil {
		return <-done
	}
	ack := &SOSyncMessage{}

	// Abort on the error.
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

// gateState returns a state whose genesis config, signed by owner, names
// owner and local with role, holding the genesis checkpoint.
func gateState(t *testing.T, soID string, owner crypto.PrivKey, local string, role sobject.SOParticipantRole) *sobject.SOState {
	t.Helper()
	state := &sobject.SOState{Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		participantCfg(mustPeerIDStr(t, owner), sobject.SOParticipantRole_SOParticipantRole_OWNER),
		participantCfg(local, role),
	}}}
	trustSnapshotConfig(t, soID, state, owner)
	return state
}

// snapshotMessage encodes state as a snapshot frame.
func snapshotMessage(t *testing.T, state *sobject.SOState) *SOSyncMessage {
	t.Helper()
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: data}}}
}

func TestSnapshotExchangeRejectsExcludedLocalPeer(t *testing.T) {
	// Create the local reader.
	const soID = "gate-object"
	localPriv, ownerPriv := mustKeyPair(t), mustKeyPair(t)
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err)
	}
	held := gateState(t, soID, ownerPriv, localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER)
	localHost, ctr := newMemHost(soID, held.CloneVT())
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil)

	// The peer offers an unproven config without the local peer.
	peerState := held.CloneVT()
	peerState.Config.Participants = peerState.Config.Participants[:1]
	advanceSnapshotCheckpoint(t, soID, peerState, ownerPriv)
	if err := runSnapshotExchange(t, s, t.Context(), snapshotMessage(t, peerState)); err == nil {
		t.Fatal("expected snapshot from config excluding the local peer to be rejected")
	}
	if !ctr.GetValue().EqualVT(held) {
		t.Fatal("rejected snapshot changed the local state")
	}
}

func TestSnapshotExchangeRejectsSnapshotWithoutLocalGrant(t *testing.T) {
	// The local reader requires a key grant to accept a snapshot.
	const soID = "gate-object-local-grant"
	localPriv, ownerPriv := mustKeyPair(t), mustKeyPair(t)
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err)
	}
	held := gateState(t, soID, ownerPriv, localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER)
	localHost, ctr := newMemHost(soID, held.CloneVT())
	validateAccess := func(_ context.Context, state *sobject.SOState) error {
		if state.CurrentKeyEpoch().FindGrant(localPeer.String()) == nil {
			return errors.New("no local key grant")
		}
		return nil
	}
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil, validateAccess)

	// The peer offers a newer checkpoint without a grant.
	peerState := held.CloneVT()
	advanceSnapshotCheckpoint(t, soID, peerState, ownerPriv)
	if err := runSnapshotExchange(t, s, t.Context(), snapshotMessage(t, peerState)); err == nil {
		t.Fatal("expected inaccessible snapshot to be rejected")
	}
	if !ctr.GetValue().EqualVT(held) {
		t.Fatal("rejected snapshot changed the local state")
	}
}

func TestSnapshotExchangeRejectsTamperedGrant(t *testing.T) {
	// Create the local reader.
	const soID = "gate-object-grant"
	localPriv, localPub, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err)
	}

	// An owner's genesis makes it a reader, held by its host.
	ownerPriv := mustKeyPair(t)
	held := gateState(t, soID, ownerPriv, localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_READER)
	localHost, ctr := newMemHost(soID, held.CloneVT())
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil)

	// The peer offers a grant whose signed body was altered.
	grant := buildGrant(t, soID, ownerPriv, localPub)
	grant.InnerData[0] ^= 0xFF
	peerState := held.CloneVT()
	peerState.KeyEpochs = []*sobject.SOKeyEpoch{{Grants: []*sobject.SOGrant{grant}}}
	if err := runSnapshotExchange(t, s, t.Context(), snapshotMessage(t, peerState)); err == nil {
		t.Fatal("expected snapshot with tampered key grant to be rejected")
	}
	if !ctr.GetValue().EqualVT(held) {
		t.Fatal("rejected snapshot changed the local state")
	}
}

func TestSnapshotExchangeAcceptsObjectPeerDistinctFromTransportPeer(t *testing.T) {
	// The local writer's object identity differs from its transport identity.
	const soID = "gate-object-valid"
	transportPeer := mustPeerIDStr(t, mustKeyPair(t))
	localPriv, localPub, err := crypto.GenerateKeyPair(crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	localPeer, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatal(err)
	}
	if localPeer.String() == transportPeer {
		t.Fatal("local object and transport peers unexpectedly match")
	}
	ownerPriv := mustKeyPair(t)

	// The local writer holds a pending write; the peer holds a newer checkpoint.
	genesis := gateState(t, soID, ownerPriv, localPeer.String(), sobject.SOParticipantRole_SOParticipantRole_WRITER)
	genesis.KeyEpochs = []*sobject.SOKeyEpoch{{Grants: []*sobject.SOGrant{buildGrant(t, soID, ownerPriv, localPub)}}}
	localState := genesis.CloneVT()
	pending := writeSyncOp(t, soID, localState, localPriv, "pending-local-write")
	localHost, ctr := newMemHost(soID, localState)
	s := NewSOSync(gateLogger(), nil, soID, localPeer, localPriv, localHost, nil)
	peerState := genesis.CloneVT()
	advanceSnapshotCheckpoint(t, soID, peerState, ownerPriv)

	// The local writer adopts the checkpoint and keeps its pending write.
	if err := runSnapshotExchange(t, s, t.Context(), snapshotMessage(t, peerState)); err != nil {
		t.Fatalf("validated snapshot should converge: %v", err)
	}
	got := ctr.GetValue()
	if !got.GetCheckpoint().EqualVT(peerState.GetCheckpoint()) {
		t.Fatal("local writer did not adopt the peer checkpoint")
	}
	if len(got.GetOps()) != 1 || !got.GetOps()[0].EqualVT(pending) {
		t.Fatalf("pending local writes = %d, want the one local write", len(got.GetOps()))
	}
}

// trustSnapshotConfig establishes a signed genesis checkpoint already held locally.
func trustSnapshotConfig(t *testing.T, soID string, state *sobject.SOState, owner crypto.PrivKey) {

	// helper.
	t.Helper()
	entry, err := sobject.BuildSOConfigChange(soID, state.GetConfig(), state.GetConfig(), sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = sobject.VerifyConfigChange(soID, state.GetConfig(), entry)
	if err != nil {
		t.Fatal(err)
	}
	state.Checkpoint, err = sobject.BuildGenesisSOCheckpoint(owner, soID, state.Config.GetConfigChainHash(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

// advanceSnapshotCheckpoint signs, as signer, the checkpoint after the one
// state holds, covering its operations, and installs it without an authority
// check so tests can also build forged successors.
func advanceSnapshotCheckpoint(t *testing.T, soID string, state *sobject.SOState, signer crypto.PrivKey) {
	// Sign the next checkpoint over the held one and drop the covered operations.
	t.Helper()
	inner, err := state.GetCheckpointInner()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("snapshot-checkpoint-" + strconv.FormatUint(inner.GetHeight()+1, 10))
	checkpoint, err := state.BuildNextCheckpoint(soID, signer, data)
	if err != nil {
		t.Fatal(err)
	}
	state.Checkpoint = checkpoint
	state.Ops = nil
}

// writeSyncOp signs data as priv at the author's next link and adds it to state.
func writeSyncOp(t *testing.T, soID string, state *sobject.SOState, priv crypto.PrivKey, data string) *sobject.SOOperation {
	// Sign the operation at the author's next link and add it.
	t.Helper()
	set, err := state.OperationSet(soID)
	if err != nil {
		t.Fatal(err)
	}
	link := state.NextOperationLink(set, mustPeerIDStr(t, priv))
	op, err := sobject.BuildSOOperation(soID, priv, []byte(data), link, ulid.NewULID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(soID, op); err != nil {
		t.Fatal(err)
	}
	return op
}
