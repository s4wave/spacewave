package provider_local

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// TestDEXHoldersFromSpace proves the block exchange of a Space knows which
// peers hold its blocks: a Space no other device reads reports a missing block
// at once, a miss each connected device answered for is a miss, and a device
// that went offline leaves the read unavailable, naming its identities.
func TestDEXHoldersFromSpace(t *testing.T) {
	// Three devices of one account share one in-process network.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	network := inproc.NewNetwork()
	var accounts []*ProviderAccount
	var sessions []*Session
	for range 3 {
		_, _, account, sess, release := setupProviderAndSessionInternal(ctx, t, func(p *Provider) {
			p.localNetwork = transport.WithInprocNetwork(network)
		})
		defer release()
		if err := account.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, account)
		sessions = append(sessions, sess)
	}

	// A creates a Space that no other device reads yet.
	a := accounts[0]
	ref, err := a.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	seedAccountReplicaPayload(ctx, t, a, ref)
	if err := a.StartPersistentP2PSync(ctx, a.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	absent, err := block.BuildBlockRef([]byte("no device holds this block"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// With no other reader, a missing block is a miss without waiting for a peer.
	waitHolderRead(ctx, t, a, ref, absent, "solo miss", func(found bool, err error) bool {
		return !found && err == nil
	})

	// B and C join the Space as devices of the account.
	b, _ := enrollMeshReplica(ctx, t, a, sessions[0], accounts[1], sessions[1])
	waitReplicaCopy(ctx, t, b, ref)
	c, cSession := enrollMeshReplica(ctx, t, a, sessions[0], accounts[2], sessions[2])
	waitReplicaCopy(ctx, t, c, ref)

	// Each device answers for its Session and its storage identity.
	waitHolderRead(ctx, t, a, ref, absent, "connected miss", func(found bool, err error) bool {
		return !found && err == nil
	})

	// With C offline, the read is unavailable and names only C's identities.
	cStorage, err := c.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	cIDs := []string{cSession.GetPeerId().String(), cStorage.GetPeerID().String()}
	c.StopP2PSync()
	c.StopSessionTransport()
	waitHolderRead(ctx, t, a, ref, absent, "offline holder", func(found bool, err error) bool {
		var unavailable *block.UnavailableError
		if found || !errors.As(err, &unavailable) || len(unavailable.Holders) == 0 {
			return false
		}
		return !slices.ContainsFunc(unavailable.Holders, func(holder string) bool {
			return !slices.Contains(cIDs, holder)
		}) && slices.Contains(unavailable.Holders, cIDs[0])
	})
}

// waitHolderRead reads ref from account's block exchange for the Space until
// accept takes the result, failing with the last result when ctx ends.
func waitHolderRead(
	ctx context.Context,
	t *testing.T,
	account *ProviderAccount,
	space *sobject.SharedObjectRef,
	ref *block.BlockRef,
	what string,
	accept func(found bool, err error) bool,
) {
	// Address the Space's bucket.
	t.Helper()
	provRef := space.GetProviderResourceRef()
	bucketID := BlockStoreBucketID(provRef.GetProviderId(), provRef.GetProviderAccountId(), space.GetBlockStoreId())

	// Read until the result is accepted.
	var found bool
	var err error
	for {
		var exchange *dex_solicit.Controller
		if state := account.p2pSyncCur.Load(); state != nil {
			exchange = state.getExchange(bucketID)
		}
		if exchange != nil {
			readCtx, readCancel := context.WithTimeout(ctx, time.Second)
			_, found, err = dex_solicit.NewStore(exchange).GetBlock(readCtx, ref)
			readCancel()
			if accept(found, err) {
				return
			}
		}

		// Retry shortly while the devices connect.
		select {
		case <-ctx.Done():
			t.Fatalf("%s: last read found=%v err=%v", what, found, err)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
