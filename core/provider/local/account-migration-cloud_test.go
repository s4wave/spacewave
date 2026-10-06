package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	controller_api "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ulid"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// TestAccountMergeCloudProviders runs against an isolated local Worker with
// ENVIRONMENT=test and ENABLE_TEST_HELPERS=true. It never uses a saved account.
func TestAccountMergeCloudProviders(t *testing.T) {
	// Skip unless the pairing fixture is a loopback Worker.
	endpoint := os.Getenv("SPACEWAVE_PAIRING_CLOUD_ENDPOINT")
	if endpoint == "" {
		t.Skip("set SPACEWAVE_PAIRING_CLOUD_ENDPOINT to an isolated local Worker")
	}
	address, err := url.Parse(endpoint)
	if err != nil || (address.Hostname() != "127.0.0.1" && address.Hostname() != "localhost") {
		t.Fatal("migration fixture requires a loopback Worker")
	}
	for _, direction := range []string{"local-to-cloud", "cloud-to-local", "cloud-to-cloud"} {
		t.Run(direction, func(t *testing.T) {
			// Bound the subtest and start the local account on an in-process network.
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			network := inproc.NewNetwork()
			tb, _, local, localSession, release := setupProviderAndSessionInternal(ctx, t, func(p *Provider) {
				p.localNetwork = transport.WithInprocNetwork(network)
			})
			defer release()

			// Load the session controller and register the local session.
			tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
			tb.StaticResolver.AddFactory(provider_spacewave.NewFactory(tb.Bus, transport.WithInprocNetwork(network)))
			_, controllerRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{VolumeId: tb.EngineVolumeID}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer controllerRef.Release()

			// Look up the session controller and register the local session.
			controller, controllerLookup, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer controllerLookup.Release()
			if _, err := controller.RegisterSession(ctx, localSession.GetSessionRef(), nil); err != nil {
				t.Fatal(err)
			}

			// Install the replica-payload block type decoder.
			decoder := blocktype_controller.NewController(func(_ context.Context, typeID string) (blocktype.BlockType, error) {
				if typeID == "test/replica-payload" {
					return blocktype.NewBlockType(typeID, block_mock.NewRootBlock), nil
				}
				return nil, nil
			})
			releaseDecoder, err := tb.Bus.AddController(ctx, decoder, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseDecoder()

			// Load the spacewave cloud provider against the fixture endpoint.
			_, cloudRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_spacewave.Config{ProviderId: "spacewave", Endpoint: endpoint, AccountEndpoint: endpoint, PublicBaseUrl: endpoint, SigningEnvPrefix: "spacewave"}), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cloudRef.Release()
			cloud, cloudLookup, err := provider.ExLookupProvider(ctx, tb.Bus, "spacewave", false, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cloudLookup.Release()
			createCloud := func() (provider_migration.Account, session.Session) {
				// Name the cloud fixture account.
				t.Helper()
				name := "pair" + ulid.NewULID()

				// Exercise native signed registration with a low-cost password fixture.
				// Production password work is covered by the authentication package.
				params := &auth_method_password.Parameters{Salt: make([]byte, 16), ScryptN: 10, ScryptR: 8, ScryptP: 1}
				if _, err := rand.Read(params.Salt); err != nil {
					t.Fatal(err)
				}
				key, err := auth_method_password.NewPasswordMethod().Authenticate(params, []byte("local fixture password 2026"))
				if err != nil {
					t.Fatal(err)
				}
				pid, err := peer.IDFromPrivateKey(key)
				if err != nil {
					t.Fatal(err)
				}
				authParams, err := params.MarshalVT()
				if err != nil {
					t.Fatal(err)
				}

				// Register the cloud account and retain its entity key.
				client := provider_spacewave.NewEntityClientDirect(http.DefaultClient, endpoint, "spacewave", key, pid)
				id, err := client.RegisterAccount(ctx, name, auth_method_password.MethodID, authParams, "")
				if err != nil {
					t.Fatal(err)
				}
				bootstrap := cloud.(*provider_spacewave.Provider).RetainEntityKeyBootstrap(id, key, pid)
				defer bootstrap.Release()

				// Activate the fixture and request its billing account.
				setCloudFixture(ctx, t, endpoint, "set-subscription", map[string]any{"account_id": id, "subscription_status": "active"})
				setCloudFixture(ctx, t, endpoint, "set-email", map[string]any{"account_id": id, "email": name + "@example.test", "verified": true})
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/test/billing-account?account_id="+id, nil)
				if err != nil {
					t.Fatal(err)
				}
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}

				// Read the billing id and mark that subscription active.
				body, err := io.ReadAll(io.LimitReader(response.Body, 2000))
				response.Body.Close()
				billingID := fastjson.GetString(body, "ba_id")
				if err != nil || response.StatusCode != http.StatusOK || billingID == "" {
					t.Fatalf("billing fixture: %d %v", response.StatusCode, err)
				}
				setCloudFixture(ctx, t, endpoint, "set-billing-subscription", map[string]any{"billing_account_id": billingID, "subscription_status": "active"})

				// Mount a cloud session and register it.
				account, releaseAccount, err := cloud.AccessProviderAccount(ctx, id, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseAccount)
				ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{Id: ulid.NewULID(), ProviderId: "spacewave", ProviderAccountId: id}}
				mounted, releaseSession, err := account.(session.SessionProvider).MountSession(ctx, ref, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseSession)
				if _, err := controller.RegisterSession(ctx, ref, nil); err != nil {
					t.Fatal(err)
				}

				// Bump the local epoch and return the cloud account.
				account.(*provider_spacewave.ProviderAccount).BumpLocalEpoch()
				return account.(provider_migration.Account), mounted
			}

			// Choose the source and destination accounts for this direction.
			var source, destination provider_migration.Account = local, local
			var moving, target session.Session = localSession, localSession
			if direction != "local-to-cloud" {
				source, moving = createCloud()
			}
			if direction != "cloud-to-local" {
				destination, target = createCloud()
			}

			// Keep a distinct cloud Session locked while account authority moves.
			// Unlocking later must consume the provider's durable transition.
			var returning session.Session
			var returningIndex uint32
			if direction != "local-to-cloud" {
				ref := moving.GetSessionRef().CloneVT()
				ref.ProviderResourceRef.Id = ulid.NewULID()
				key, _, err := crypto.GenerateEd25519Key(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				id, err := peer.IDFromPrivateKey(key)
				if err != nil {
					t.Fatal(err)
				}
				cloudSource := source.(*provider_spacewave.ProviderAccount)
				if _, err := cloudSource.LinkSession(ctx, moving.GetPrivKey(), id, "Returning fixture"); err != nil {
					t.Fatal(err)
				}
				plain, err := keypem.MarshalPrivKeyPem(key)
				if err != nil {
					t.Fatal(err)
				}
				// Reuse the low-cost KDF fixture so race instrumentation measures
				// Session recovery rather than password-derivation throughput.
				encPriv, encSymKey, lockConfig, err := createLowCostPINLock(plain, []byte("123456"))
				if err != nil {
					t.Fatal(err)
				}
				store := moving.(provider_migration.CredentialSource).SessionCredentialStore()
				if err := session_lock.WritePINLock(ctx, store, ref.GetProviderResourceRef().GetId(), encPriv, encSymKey, lockConfig); err != nil {
					t.Fatal(err)
				}
				if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }, func(ctx context.Context, tx kvtx.Tx) error {
					return tx.Set(ctx, []byte(ref.GetProviderResourceRef().GetId()+"/registered"), []byte(id.String()))
				}); err != nil {
					t.Fatal(err)
				}
				if err := cloudSource.UnlockPINSession(ctx, ref, []byte("123456")); err != nil {
					t.Fatal(err)
				}
				var releaseReturning func()
				returning, releaseReturning, err = source.MountSession(ctx, ref, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseReturning)
				entry, err := controller.RegisterSession(ctx, ref, nil)
				if err != nil {
					t.Fatal(err)
				}
				returningIndex = entry.GetSessionIndex()
				if err := returning.LockSession(ctx); err != nil {
					t.Fatal(err)
				}
			}

			// Seed a space and payload on the source and the destination.
			var refs []*sobject.SharedObjectRef
			var leaves []*block.BlockRef
			var payloads [][]byte
			for _, account := range []provider_migration.Account{source, destination} {
				meta, err := (&space.SpaceSoMeta{Name: "Migration payload"}).MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space", BodyMeta: meta}, "", "")
				if err != nil {
					t.Fatal(err)
				}
				leaf, payload := seedProviderReplicaPayload(ctx, t, local, account, ref)
				refs, leaves, payloads = append(refs, ref), append(leaves, leaf), append(payloads, payload)
			}

			// For a local-to-cloud merge, enroll an offline local replica.
			var returningLocal *ProviderAccount
			var returningLocalSession *Session
			var returningLocalController session.SessionController
			var returningLocalEntry *session.SessionListEntry
			if direction == "local-to-cloud" {
				client, clientSession, clientController := setupMigrationClient(ctx, t, network)
				returningLocal, returningLocalSession = enrollMeshReplica(ctx, t, local, localSession, client, clientSession)
				returningLocalController = clientController
				returningLocalEntry, err = clientController.RegisterSession(ctx, returningLocalSession.GetSessionRef(), nil)
				if err != nil {
					t.Fatal(err)
				}
				waitReplicaCopy(ctx, t, returningLocal, refs[0])
				returningLocal.StopP2PSync()
				returningLocal.StopSessionTransport()
				cloudController, err := provider_spacewave.NewFactory(client.t.p.b, transport.WithInprocNetwork(network)).Construct(ctx, &provider_spacewave.Config{ProviderId: "spacewave", Endpoint: endpoint, AccountEndpoint: endpoint, PublicBaseUrl: endpoint, SigningEnvPrefix: "spacewave"}, controller_api.ConstructOpts{Logger: client.le})
				if err != nil {
					t.Fatal(err)
				}
				releaseCloud, err := client.t.p.b.AddController(ctx, cloudController, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer releaseCloud()
			}

			// Merge the accounts and require the moving session identity to stay.
			t.Log("source payload committed; merging account")
			commit, err := provider_migration.Merge(ctx, source, moving, destination, target.GetSessionRef())
			if err != nil {
				t.Fatal(err)
			}
			moved, err := commit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if moved.GetProviderResourceRef().GetId() != moving.GetSessionRef().GetProviderResourceRef().GetId() {
				t.Fatal("moving Session identity changed")
			}

			// Require the waiting local replica to follow the cloud destination.
			if returningLocal != nil {
				local.StopP2PSync()
				local.StopSessionTransport()
				for {
					state, changed := destination.(*provider_spacewave.ProviderAccount).GetTransportCompositionSnapshotWithWait(target.GetSessionRef().GetProviderResourceRef().GetId())
					if state.P2PState == provider_spacewave.TransportCompositionP2PStateNoPeers || state.P2PState == provider_spacewave.TransportCompositionP2PStateIdle || state.P2PState == provider_spacewave.TransportCompositionP2PStateActive {
						break
					}
					if state.P2PState == provider_spacewave.TransportCompositionP2PStateError {
						t.Fatal(state.LastError)
					}
					select {
					case <-ctx.Done():
						t.Fatal("cloud destination transport did not become ready")
					case <-changed:
					}
				}
				if err := returningLocal.EnsureConfiguredSessionTransport(ctx, returningLocalSession.GetPrivKey()); err != nil {
					t.Fatal(err)
				}
				if err := returningLocal.StartPersistentP2PSync(ctx, returningLocal.GetSessionTransport()); err != nil {
					t.Fatal(err)
				}
				for {
					var changed <-chan struct{}
					returningLocalController.GetSessionBroadcast().HoldLock(func(_ func(), getWait func() <-chan struct{}) { changed = getWait() })
					entry, err := returningLocalController.GetSessionByIdx(ctx, returningLocalEntry.GetSessionIndex())
					if err != nil {
						t.Fatal(err)
					}
					if entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() == destination.GetAccountID() {
						if entry.GetSessionRef().GetProviderResourceRef().GetId() != returningLocalSession.GetSessionRef().GetProviderResourceRef().GetId() {
							t.Fatal("returning local Session identity changed")
						}
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("offline local Session did not follow the cloud destination")
					case <-changed:
					}
				}
			}

			// Require the locked cloud session to unlock on the destination.
			if returning != nil {
				if err := returning.UnlockSession(ctx, []byte("123456")); err != nil {
					t.Fatal(err)
				}
				_, releaseReturning, err := source.MountSession(ctx, returning.GetSessionRef(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer releaseReturning()
				for {
					entries, err := controller.ListSessions(ctx)
					if err != nil {
						t.Fatal(err)
					}
					var attached *session.SessionRef
					for _, entry := range entries {
						if entry.GetSessionIndex() == returningIndex && entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() == destination.GetAccountID() {
							attached = entry.GetSessionRef()
						}
					}
					if attached != nil {
						resumed, releaseResumed, err := destination.MountSession(ctx, attached, nil)
						if err != nil {
							t.Fatal(err)
						}
						mode, locked, err := resumed.GetLockState(ctx)
						releaseResumed()
						if err != nil || locked || mode != session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
							t.Fatalf("returning PIN protection changed: mode=%v locked=%v err=%v", mode, locked, err)
						}
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("returning Session did not follow provider transition")
					case <-time.After(20 * time.Millisecond):
					}
				}
			}

			// Require each destination copy to hold the seeded payload.
			for i, ref := range refs {
				copiedRef := sobject.NewSharedObjectRef(destination.GetProviderID(), destination.GetAccountID(), ref.GetProviderResourceRef().GetId(), SobjectBlockStoreID(ref.GetProviderResourceRef().GetId()))
				object, releaseObject, err := destination.MountSharedObject(ctx, copiedRef, nil)
				if err != nil {
					t.Fatal(err)
				}
				data, found, err := object.GetBlockStore().GetBlock(ctx, leaves[i])
				releaseObject()
				if err != nil || !found || !bytes.Equal(data, payloads[i]) {
					t.Fatalf("destination payload %d differs: found=%v err=%v", i, found, err)
				}
			}
		})
	}
}

func setCloudFixture(ctx context.Context, t *testing.T, endpoint, action string, value map[string]any) {
	// Build the JSON body for the fixture action.
	t.Helper()
	var arena fastjson.Arena
	object := arena.NewObject()
	for key, value := range value {
		switch value := value.(type) {
		case string:
			object.Set(key, arena.NewString(value))
		case bool:
			if value {
				object.Set(key, arena.NewTrue())
			} else {
				object.Set(key, arena.NewFalse())
			}
		default:
			t.Fatalf("unsupported fixture value %s: %T", key, value)
		}
	}
	body := object.MarshalTo(nil)

	// Post the fixture action and require an ok response.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/test/"+action, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2000))
	if err != nil || response.StatusCode != http.StatusOK || !fastjson.GetBool(data, "ok") {
		t.Fatalf("fixture %s: status=%d body=%s err=%v", action, response.StatusCode, data, err)
	}
}
