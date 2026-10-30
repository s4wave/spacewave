package provider_local

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// dexRolePeers holds two independent accounts exchanging one SharedObject.
type dexRolePeers struct {
	// accounts holds the original owner followed by its invited successor.
	accounts [2]*ProviderAccount
	// sessions holds the authenticated transport identities.
	sessions [2]*Session
	// objects holds each account's mount of the same SharedObject.
	objects [2]*SharedObject
	// exchanges holds the production exchanges started by each provider.
	exchanges [2]*dex_solicit.Controller
}

// newDEXRolePeers joins two real providers over an in-memory network.
func newDEXRolePeers(t *testing.T, ctx context.Context) *dexRolePeers {
	// Start independent accounts with separate volumes on one in-memory network.
	t.Helper()
	peers := &dexRolePeers{}
	network := inproc.NewNetwork()
	for i := range peers.accounts {
		_, _, account, session, release := setupProviderAndSessionInternal(ctx, t, func(p *Provider) {
			p.localNetwork = transport.WithInprocNetwork(network)
		})
		t.Cleanup(release)
		peers.accounts[i], peers.sessions[i] = account, session
	}

	// Create an object without a background graph copy that could warm the peer.
	ref, err := peers.accounts[0].CreateSharedObject(ctx, "dex-roles", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, release, err := peers.accounts[0].MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	peers.objects[0] = mounted.(*SharedObject)

	// Invite the second Session through the same enrollment used in production.
	owner := peers.objects[0]
	invite, err := owner.CreateSOInviteOp(ctx, owner.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER})
	if err != nil {
		t.Fatal(err)
	}
	if err := peers.accounts[0].PrepareDirectInvite(ctx, peers.sessions[0].GetPrivKey(), owner.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(peers.accounts[0].StopSessionTransport)
	t.Cleanup(peers.accounts[0].StopP2PSync)

	// Redeem the invite and retain the joining account's transport.
	if _, err := peers.accounts[1].JoinViaInvite(ctx, peers.sessions[1].GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(peers.accounts[1].StopSessionTransport)
	t.Cleanup(peers.accounts[1].StopP2PSync)

	// Mount the invitee's object and retain each provider's actual DEX controller.
	for i, account := range peers.accounts {
		entries := account.soListCtr.GetValue().GetSharedObjects()
		idx := slices.IndexFunc(entries, func(entry *sobject.SharedObjectListEntry) bool {
			return entry.GetRef().GetProviderResourceRef().GetId() == owner.GetSharedObjectID()
		})
		if idx == -1 {
			t.Fatal("joined object is absent from account")
		}
		ref := entries[idx].GetRef()
		mounted, release, err := account.MountSharedObject(ctx, ref, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		peers.objects[i] = mounted.(*SharedObject)

		// Read the exchange under the provider's lifecycle lock.
		account.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			providerRef := ref.GetProviderResourceRef()
			peers.exchanges[i] = account.p2pSync.getExchange(BlockStoreBucketID(providerRef.GetProviderId(), providerRef.GetProviderAccountId(), ref.GetBlockStoreId()))
		})
		if peers.exchanges[i] == nil {
			t.Fatal("provider did not start the object's DEX")
		}
	}
	return peers
}

// TestDEXRequesterRole gates new requests on the stream opened before departure.
func TestDEXRequesterRole(t *testing.T) {
	for _, departure := range []string{"leave", "remove"} {
		t.Run(departure, func(t *testing.T) {
			// Bound the complete two-peer request and departure lifecycle.
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			t.Cleanup(cancel)
			peers := newDEXRolePeers(t, ctx)
			data := []byte("retained blocks need a current requester role")
			ref, _, err := peers.objects[0].GetBlockStore().PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
			if err != nil {
				t.Fatal(err)
			}

			// Fetch without writeback so the exact same reference remains remote.
			exchange := peers.exchanges[1]
			store := dex_solicit.NewStore(exchange)
			got, found, err := store.GetBlock(ctx, ref)
			if err != nil || !found || !bytes.Equal(got, data) {
				t.Fatalf("authorized fetch = %q/%v/%v", got, found, err)
			}
			starts := exchange.GetSessionStarts()

			// Revoke the requesting Session through the real authority writer.
			owner := peers.objects[0]
			if departure == "leave" {
				if err := peers.accounts[1].LeaveSharedObject(ctx, peers.sessions[1].GetPrivKey(), owner.GetSharedObjectID(), ""); err != nil {
					t.Fatal(err)
				}
			}
			if departure == "remove" {
				removed, err := sobject.RemoveSOParticipant(ctx, owner.soHost, peers.sessions[1].GetPeerId().String(), owner.GetPrivKey(), nil)
				if err != nil || !removed {
					t.Fatalf("remove requester = %v/%v", removed, err)
				}
			}

			// The open stream must reject the old known reference without leaking it.
			got, found, err = store.GetBlock(ctx, ref)
			if err == nil || !strings.Contains(err.Error(), "access denied") || found || len(got) != 0 {
				t.Fatalf("departed requester fetch = %q/%v/%v", got, found, err)
			}
			if exchange.GetSessionStarts() != starts {
				t.Fatal("departure replaced the stream instead of testing its request gate")
			}
		})
	}
}

// TestDEXDepartedOwnerServesSuccessor preserves one-way backfill after handover.
func TestDEXDepartedOwnerServesSuccessor(t *testing.T) {
	// Start the production two-peer exchange before the owner hands over.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	peers := newDEXRolePeers(t, ctx)
	owner, successor := peers.objects[0], peers.objects[1]
	data := []byte("successor backfill from a departed owner")
	ref, _, err := owner.GetBlockStore().PutBlock(ctx, data, &block.PutOpts{Refs: []*block.BlockRef{}, RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}

	// Transfer ownership and let the successor commit the departure.
	if err := peers.accounts[0].LeaveSharedObject(ctx, peers.sessions[0].GetPrivKey(), owner.GetSharedObjectID(), peers.sessions[1].GetPeerId().String()); err != nil {
		t.Fatal(err)
	}
	states, releaseStates, err := successor.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseStates)
	departed, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return !slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == owner.GetPeerID().String() || p.GetPeerId() == peers.sessions[0].GetPeerId().String()
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Have the former owner observe the successor's signed removal proof too.
	held, err := owner.soHost.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := successor.soHost.ReadConfigHistory(ctx, held.GetConfig().GetConfigChainHash(), departed.GetConfig().GetConfigChainHash())
	if err != nil {
		t.Fatal(err)
	}
	err = owner.soHost.ImportPeerSnapshot(ctx, &sobject.SOState{Config: departed.GetConfig()}, changes, owner.GetPeerID(), nil)
	if err != nil && !errors.Is(err, sobject.ErrParticipantRevoked) {
		t.Fatal(err)
	}

	// The old owner cannot use the reverse direction to read successor blocks.
	successorRef, _, err := successor.GetBlockStore().PutBlock(ctx, []byte("successor-only block"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := dex_solicit.NewStore(peers.exchanges[0]).GetBlock(ctx, successorRef)
	if err == nil || !strings.Contains(err.Error(), "access denied") || found || len(got) != 0 {
		t.Fatalf("departed owner request = %q/%v/%v", got, found, err)
	}

	// Fetch directly through DEX, not through either account's cached store.
	stored, err := dex_solicit.NewStore(peers.exchanges[1]).GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("successor backfill = %v/%v", stored, err)
	}
}

// TestDEXAuthorityStopsWithSource rejects cached grants after the mounted
// authority source stops, even while the requesting context remains live.
func TestDEXAuthorityStopsWithSource(t *testing.T) {
	// Retain a real provider authority watch under an independent request lifetime.
	requestCtx, cancelRequest := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancelRequest)
	sourceCtx, cancelSource := context.WithCancel(requestCtx)
	t.Cleanup(cancelSource)
	peers := newDEXRolePeers(t, sourceCtx)

	// Acquire authority while the source still reports the requester as readable.
	access, err := peers.accounts[0].authorizeDEXRequest(requestCtx, &dex_solicit.Config{
		ProtocolContext: []byte(peers.objects[0].GetSharedObjectID()),
	}, peers.sessions[1].GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(access.Release)
	if err := access.Check(); err != nil {
		t.Fatal(err)
	}

	// Watch authority before canceling the source, not the request.
	revoked := make(chan error, 1)
	go func() { revoked <- access.WaitRevoked(requestCtx) }()
	cancelSource()
	select {
	case err := <-revoked:
		if err == nil {
			t.Fatal("authority source stopped without rejecting the request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("authority watch kept cached access after its source stopped")
	}

	// Both synchronous checks and the watch must fail closed on source loss.
	if err := access.Check(); err == nil {
		t.Fatal("cached role still authorized after the authority source stopped")
	}
	if err := requestCtx.Err(); err != nil {
		t.Fatalf("source cancellation also ended the independent request: %v", err)
	}
}
