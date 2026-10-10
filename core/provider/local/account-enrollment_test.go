package provider_local

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// TestAccountReplicaEnrollment checks account identity, independent credentials,
// authorized Space state, and durable Session readback across isolated stores.
func TestAccountReplicaEnrollment(t *testing.T) {
	// Open isolated source and receiver providers.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	_, _, source, sourceSession, releaseSource := setupProviderAndSessionInternal(ctx, t)
	defer releaseSource()
	_, _, receiver, receiverSession, releaseReceiver := setupProviderAndSessionInternal(ctx, t)
	defer releaseReceiver()

	// Seed a real Space before another machine attaches to its account.
	spaceRef, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	settingsRef, err := source.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	offer := &pairing.AccountOffer{AccountId: source.GetAccountID(), SettingsId: settingsRef.GetProviderResourceRef().GetId(), OperationId: ulid.NewULID()}

	// Prepare a fresh Session in the same logical account on the receiving store.
	ref := sourceSession.GetSessionRef().CloneVT()
	ref.ProviderResourceRef.Id = ulid.NewULID()
	account, releaseAccount, err := receiver.t.p.AccessProviderAccount(ctx, source.GetAccountID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseAccount()
	replica := account.(*ProviderAccount)

	// Mount a Session with its own credential, never a reused one.
	sess, releaseSession, err := replica.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSession()
	if sess.GetPeerId() == sourceSession.GetPeerId() || sess.GetPeerId() == receiverSession.GetPeerId() {
		t.Fatal("enrollment reused an existing Session credential")
	}

	// Its pairing identity proves the offer from both existing Sessions.
	identity, err := replica.buildPairingIdentity(ctx, offer, sess, sourceSession.GetPeerId(), receiverSession.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.ValidateIdentity(offer, identity, sourceSession.GetPeerId(), receiverSession.GetPeerId()); err != nil {
		t.Fatal(err)
	}

	// Reject a proof reused for a different account before issuing any grants.
	other := offer.CloneVT()
	other.AccountId = receiver.GetAccountID()
	if err := pairing.ValidateIdentity(other, identity, sourceSession.GetPeerId(), receiverSession.GetPeerId()); err == nil {
		t.Fatal("accepted a proof for another account")
	}
	if err := replica.bindPairingSettings(ctx, offer); err != nil {
		t.Fatal(err)
	}

	// Transfer each authorized checkpoint through the production host contract.
	for _, entry := range source.GetSOListCtr().GetValue().GetSharedObjects() {
		object, err := source.enrollPairingObject(ctx, entry, identity)
		if err != nil {
			t.Fatal(err)
		}
		if err := replica.installPairingObject(ctx, offer, object, sourceSession.GetPeerId()); err != nil {
			t.Fatal(err)
		}
	}

	// The replica binds the canonical settings and lists both objects.
	bound, err := replica.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bound.EqualVT(settingsRef) {
		t.Fatal("replica did not bind the canonical account settings")
	}
	if len(replica.GetSOListCtr().GetValue().GetSharedObjects()) != 2 {
		t.Fatal("replica did not discover the original Space")
	}

	// The receiving storage key reads the Space checkpoint.
	so, releaseSO, err := replica.MountSharedObject(ctx, spaceRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.GetCheckpoint(ctx); err != nil {
		t.Fatalf("receiving storage key cannot read the Space: %v", err)
	}

	// Reopen the Session from its encrypted record and retain its own peer key.
	peerID := sess.GetPeerId()
	releaseSession()
	reopened, releaseReopened, err := replica.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReopened()
	if reopened.GetPeerId() != peerID || reopened.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() != source.GetAccountID() {
		t.Fatal("Session readback lost its account or independent identity")
	}
}

// TestAccountPairingExchange exercises the actual duplex protocol, bilateral
// decision, provider bootstrap, and durable Session registration together.
func TestAccountPairingExchange(t *testing.T) {
	for _, approve := range []bool{false, true} {
		name := "reject"
		if approve {
			name = "enroll"
		}
		t.Run(name, func(t *testing.T) {
			// Bound the exchange.
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()

			// Open isolated source and receiver providers on one authenticated
			// packet network. The network is set before each Session mounts and
			// starts its transport; provider stores stay isolated.
			network := inproc.NewNetwork()
			shareNetwork := func(p *Provider) { p.localNetwork = transport.WithInprocNetwork(network) }
			_, _, source, sourceSession, releaseSource := setupProviderAndSessionInternal(ctx, t, shareNetwork)
			defer releaseSource()
			tb, _, receiver, receivingSession, releaseReceiver := setupProviderAndSessionInternal(ctx, t, shareNetwork)
			defer releaseReceiver()

			// Wait for both Session transports to run.
			for _, endpoint := range []struct {
				account *ProviderAccount
				session *Session
			}{{source, sourceSession}, {receiver, receivingSession}} {
				if err := endpoint.account.EnsureConfiguredSessionTransport(ctx, endpoint.session.GetPrivKey()); err != nil {
					t.Fatal(err)
				}
			}

			// Use the real local Session list controller and existing account fixtures.
			tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
			_, controllerRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{VolumeId: tb.EngineVolumeID}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer controllerRef.Release()
			controller, lookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer lookupRef.Release()
			if _, err := controller.RegisterSession(ctx, receivingSession.GetSessionRef(), nil); err != nil {
				t.Fatal(err)
			}

			// Create the offered Space, seeding a payload when enrollment proceeds.
			spaceRef, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			var payloadRef *block.BlockRef
			var payload []byte
			if approve {
				payloadRef, payload = seedAccountReplicaPayload(ctx, t, source, spaceRef)
			}

			// Run both pairing engines over one in-memory duplex stream.
			const agentLabel = "Test agent on build host"
			sourceEngine := pairingEngineForTest(t, sourceSession)
			receivingEngine := pairingEngineForTest(t, receivingSession)
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()

			// Each engine signals finished when its side of the exchange ends.
			finished := make(chan struct{}, 2)
			go func() {
				defer left.Close()
				sourceEngine.StartOnStream(ctx, left, receivingSession.GetPeerId(), true, true, "")
				finished <- struct{}{}
			}()
			go func() {
				defer right.Close()
				receivingEngine.StartOnStream(ctx, right, sourceSession.GetPeerId(), false, false, agentLabel)
				finished <- struct{}{}
			}()

			// Matching emoji alone must neither register a Session nor import
			// Spaces. The approving client sees the receiver's label.
			verifying := waitForPairingStatus(ctx, t, sourceEngine, pairing.StatusVerifyingEmoji)
			waitForPairingStatus(ctx, t, receivingEngine, pairing.StatusVerifyingEmoji)
			if verifying.RemoteLabel != agentLabel {
				t.Fatalf("approval screen label = %q, want %q", verifying.RemoteLabel, agentLabel)
			}
			entries, err := controller.ListSessions(ctx)
			if err != nil || len(entries) != 1 {
				t.Fatalf("unapproved pairing changed the Session list: %v, %v", entries, err)
			}
			sourceEngine.ConfirmSAS(approve)
			if approve {
				receivingEngine.ConfirmSAS(true)
			}
			for range 2 {
				select {
				case <-ctx.Done():
					t.Fatal("account pairing did not finish")
				case <-finished:
				}
			}

			// Rejection before local approval must terminate both peers without enrollment.
			entries, err = controller.ListSessions(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !approve {
				if len(entries) != 1 {
					t.Fatal("rejected pairing registered account access")
				}
				waitForPairingStatus(ctx, t, receivingEngine, pairing.StatusPairingRejected)
				return
			}

			// Successful completion keeps the original account and attaches a distinct Session.
			waitForPairingStatus(ctx, t, sourceEngine, pairing.StatusBothConfirmed)
			waitForPairingStatus(ctx, t, receivingEngine, pairing.StatusBothConfirmed)
			ref, err := receivingEngine.Result(sourceSession.GetPeerId())
			if err != nil || ref == nil {
				t.Fatalf("missing durable pairing result: %v", err)
			}
			if len(entries) != 2 || ref.GetProviderResourceRef().GetProviderAccountId() != source.GetAccountID() {
				t.Fatal("pairing did not add the offered account alongside the existing account")
			}
			repeated, err := receivingEngine.Result(sourceSession.GetPeerId())
			if err != nil || !repeated.EqualVT(ref) {
				t.Fatal("completion retry did not return the same Session")
			}

			// The source account names the new Session with the receiver's label.
			settings, err := source.readAccountSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			named := false
			for _, presentation := range settings.GetSessionPresentations() {
				if presentation.GetLabel() == agentLabel {
					member := settings.FindAccountSession(presentation.GetPeerId())
					named = member != nil && !member.GetRevoked() && presentation.GetPeerId() != sourceSession.GetPeerId().String()
				}
			}
			if !named {
				t.Fatal("paired Session was not named with the receiver's label")
			}

			// The approving client learns the enrolled Session, not the pairing link.
			enrolled := settings.FindAccountSession(verifying.SessionPeerID)
			if enrolled == nil || enrolled.GetRevoked() || verifying.SessionPeerID == receivingSession.GetPeerId().String() {
				t.Fatalf("approval named Session peer %q, want the enrolled Session", verifying.SessionPeerID)
			}

			// Open the offered account on the receiving store.
			account, releaseAccount, err := receiver.t.p.AccessProviderAccount(ctx, source.GetAccountID(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseAccount()
			replica := account.(*ProviderAccount)

			// The replica reads the original Space checkpoint.
			so, releaseSO, err := replica.MountSharedObject(ctx, spaceRef, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseSO()
			snapshot, err := so.GetSharedObjectState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := snapshot.GetCheckpoint(ctx); err != nil {
				t.Fatalf("paired replica cannot read the original Space: %v", err)
			}

			// Background replication must copy the nested payload before a file
			// read can request it. The final read bypasses the network store.
			for {
				progress, changed := replica.GetAccountCopyProgress()
				complete := false
				for _, item := range progress {
					if item.GetObjectId() == spaceRef.GetProviderResourceRef().GetId() && item.GetComplete() {
						complete = true
					}
				}
				if complete {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("background copy did not finish: %v", progress)
				case <-changed:
				}
			}

			// The payload counted as peer traffic, split across the peers.
			localStore := so.GetBlockStore().(*BlockStore).store
			trafficBefore, _ := replica.GetAccountTransferSnapshot()
			if trafficBefore.DownloadedBytes < uint64(len(payload)) {
				t.Fatal("peer traffic did not count the transferred payload")
			}
			var peerBytes uint64
			for _, peer := range trafficBefore.Peers {
				peerBytes += peer.DownloadedBytes
			}
			if peerBytes != trafficBefore.DownloadedBytes {
				t.Fatal("per-peer traffic does not add up to the account total")
			}

			// Reading the local copy adds no peer traffic.
			copied, found, err := localStore.GetBlock(ctx, payloadRef)
			if err != nil || !found || !bytes.Equal(copied, payload) {
				t.Fatalf("background payload is not local: found=%v err=%v", found, err)
			}
			trafficAfter, _ := replica.GetAccountTransferSnapshot()
			if trafficAfter.DownloadedBytes != trafficBefore.DownloadedBytes {
				t.Fatal("local read was counted as peer traffic")
			}

			// A later Space must arrive through canonical catalog sync and the real
			// authenticated replica service without reopening the pairing exchange.
			later, err := source.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := replica.soListCtr.WaitValueWithValidator(ctx, func(list *sobject.SharedObjectList) (bool, error) {
				for _, entry := range list.GetSharedObjects() {
					if entry.GetRef().EqualVT(later) {
						return true, nil
					}
				}
				return false, nil
			}, nil); err != nil {
				t.Fatalf("later Space did not reach the paired replica: %v", err)
			}

			// Catalog metadata and deletion follow the same live account state.
			renamed, err := space.NewSharedObjectMeta("Renamed Space")
			if err != nil {
				t.Fatal(err)
			}
			if err := source.UpdateSharedObjectMeta(ctx, later.GetProviderResourceRef().GetId(), renamed); err != nil {
				t.Fatal(err)
			}
			if _, err := replica.soListCtr.WaitValueWithValidator(ctx, func(list *sobject.SharedObjectList) (bool, error) {
				for _, entry := range list.GetSharedObjects() {
					if entry.GetRef().EqualVT(later) {
						return entry.GetMeta().EqualVT(renamed), nil
					}
				}
				return false, nil
			}, nil); err != nil {
				t.Fatalf("catalog metadata did not reach the paired replica: %v", err)
			}
			if err := source.DeleteSharedObject(ctx, later.GetProviderResourceRef().GetId()); err != nil {
				t.Fatal(err)
			}
			if _, err := replica.soListCtr.WaitValueWithValidator(ctx, func(list *sobject.SharedObjectList) (bool, error) {
				for _, entry := range list.GetSharedObjects() {
					if entry.GetRef().EqualVT(later) {
						return false, nil
					}
				}
				return true, nil
			}, nil); err != nil {
				t.Fatalf("catalog deletion did not reach the paired replica: %v", err)
			}

			// A revoked Session cannot fetch another checkpoint, and both its
			// transport and storage grants are removed from the surviving objects.
			paired, releasePaired, err := replica.MountSession(ctx, ref, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer releasePaired()
			settingsRef, err := source.GetAccountSettingsRef(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// The active Session fetches a checkpoint until the source unlinks it.
			open := stream_srpc.NewOpenStreamFunc(replica.GetSessionTransport().GetChildBus(), accountReplicaProtocol, paired.GetPeerId(), sourceSession.GetPeerId(), 0)
			client := NewSRPCAccountReplicaServiceClient(srpc.NewClient(open))
			request := &AccountReplicaObjectRequest{SettingsId: settingsRef.GetProviderResourceRef().GetId(), ObjectId: spaceRef.GetProviderResourceRef().GetId()}
			if _, err := client.FetchObject(ctx, request); err != nil {
				t.Fatalf("active account Session could not fetch its checkpoint: %v", err)
			}
			if err := source.UnlinkDevice(ctx, paired.GetPeerId()); err != nil {
				t.Fatal(err)
			}
			if _, err := client.FetchObject(ctx, request); err == nil {
				t.Fatal("revoked account Session fetched a checkpoint")
			}

			// No object keeps a grant for the Session or its storage peer.
			storage, err := replica.vol.GetPeer(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range source.soListCtr.GetValue().GetSharedObjects() {
				state := getSOState(ctx, t, source, entry.GetRef(), entry.GetRef().GetProviderResourceRef().GetId())
				for _, epoch := range state.GetKeyEpochs() {
					if epoch.FindGrant(paired.GetPeerId().String()) != nil || epoch.FindGrant(storage.GetPeerID().String()) != nil {
						t.Fatal("unlink retained an enrolled Session or storage grant")
					}
				}
			}
		})
	}
}

// seedAccountReplicaPayload publishes a real nested block graph under a signed
// Space checkpoint. Its leaf is large enough to exercise authenticated DEX transfer.
func seedAccountReplicaPayload(ctx context.Context, t *testing.T, account *ProviderAccount, ref *sobject.SharedObjectRef) (*block.BlockRef, []byte) {
	return seedProviderReplicaPayload(ctx, t, account, account, ref)
}

// seedProviderReplicaPayload exercises either native provider on the fixture bus.
func seedProviderReplicaPayload(ctx context.Context, t *testing.T, fixture *ProviderAccount, account sobject.SharedObjectProvider, ref *sobject.SharedObjectRef) (*block.BlockRef, []byte) {
	// Mount the Space to seed.
	t.Helper()
	so, release, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// A cloud mount can precede its initial readable root. Wait for the same
	// grant-backed state required by a Space viewer before submitting an edit.
	snapshots, releaseSnapshots, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSnapshots()
	if _, err := snapshots.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		_, err := snapshot.GetTransformer(ctx)
		if errors.Is(err, sobject.ErrCannotDecode) {
			return false, nil
		}
		return err == nil, err
	}, nil); err != nil {
		t.Fatalf("wait for readable Space before seeding: %v", err)
	}

	// Write a large leaf under a nested root block.
	store := so.GetBlockStore()
	data, err := (&block_mock.Example{Msg: string(bytes.Repeat([]byte("paired account payload\n"), 8192))}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	leaf, _, err := store.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := block.PutBlock(ctx, store, &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: leaf}})
	if err != nil {
		t.Fatal(err)
	}

	// Build a World over the store that names the root as an object.
	cursor := bucket_lookup.NewCursor(ctx, fixture.t.p.b, fixture.le, fixture.t.p.sfs, store, nil, &bucket.ObjectRef{}, nil, nil)
	defer cursor.Release()
	ws, err := world_block.BuildWorldStateFromCursor(ctx, fixture.le, true, cursor, world.NewWorldStorageFromCursor(cursor), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Discard()
	{
		createdObject, err := ws.CreateObject(ctx, "payload", &bucket.ObjectRef{RootRef: root})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Type the object, commit the World, and encode its head as checkpoint state.
	if err := world_types.SetObjectType(ctx, ws, "payload", "test/replica-payload"); err != nil {
		t.Fatal(err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := (&sobject_world_engine.InnerState{HeadRef: &bucket.ObjectRef{RootRef: ws.GetRootRef()}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Publish the World as a checkpoint signed by the owner.
	host, ok := so.(sobject.InviteHost)
	if !ok {
		t.Fatal("Space host cannot sign a checkpoint")
	}
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt the state with the current key.
	xfrm, err := snapshot.GetTransformer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stateEnc, err := xfrm.EncodeBlock(state)
	if err != nil {
		t.Fatal(err)
	}

	// Adopt the next checkpoint and wait until readers see it.
	id := so.GetSharedObjectID()
	if err := host.GetSOHost().UpdateSOState(ctx, func(next *sobject.SOState) error {
		checkpoint, err := next.BuildNextCheckpoint(id, host.GetPrivKey(), stateEnc)
		if err != nil {
			return err
		}
		return next.AdoptCheckpoint(id, checkpoint)
	}); err != nil {
		t.Fatalf("publish the replica fixture checkpoint: %v", err)
	}
	if _, err := snapshots.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		checkpoint, err := snapshot.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		return bytes.Equal(checkpoint.GetStateData(), state), nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	return leaf, data
}
