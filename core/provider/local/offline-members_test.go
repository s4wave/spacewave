package provider_local_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/testbed"
)

// offlineMember is one member's mounted Space and the World it replays.
type offlineMember struct {
	t       *testing.T
	name    string
	tb      *testbed.Testbed
	account *provider_local.ProviderAccount
	object  *provider_local.SharedObject
	ref     *sobject.SharedObjectRef
	engine  sobject_world_engine.Engine
	release directive.Reference
}

// TestOfflineMembersConverge has an owner and a joined writer edit the same
// Space while disconnected, including a conflicting creation of one object.
// Each member sees its own edits at once and keeps them across a World engine
// restart. After reconnecting, both reach the same World root, and the member
// whose creation lost is told so in its health. The members run on separate
// daemons, and as two accounts on one daemon.
func TestOfflineMembersConverge(t *testing.T) {
	// Full peer sync runs natively under one deadline.
	skipFullP2PSyncUnderGoScript(t)
	t.Run("separate daemons", func(t *testing.T) {
		// Start one daemon for each member.
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()
		ownerTb, _, ownerAccount, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
		t.Cleanup(releaseOwner)
		memberTb, _, memberAccount, memberSession, releaseMember := setupProviderAndSession(ctx, t)
		t.Cleanup(releaseMember)

		// Run the case with a World engine on each daemon.
		ownerTb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(ownerTb.Bus))
		memberTb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(memberTb.Bus))
		testOfflineMembersConverge(ctx, t, ownerTb, ownerAccount, ownerSession, memberTb, memberAccount, memberSession)
	})
	t.Run("one daemon", func(t *testing.T) {
		// Start one daemon with both members' accounts.
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()
		tb, _, ownerAccount, ownerSession, release := setupProviderAndSession(ctx, t)
		t.Cleanup(release)
		memberAccount, memberSession := addLocalSession(ctx, t, tb)

		// Run the case with a World engine on the daemon.
		tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
		testOfflineMembersConverge(ctx, t, tb, ownerAccount, ownerSession, tb, memberAccount, memberSession)
	})
}

// testOfflineMembersConverge runs the offline editing case between an owner
// and a member that joins it.
func testOfflineMembersConverge(
	ctx context.Context,
	t *testing.T,
	ownerTb *testbed.Testbed,
	ownerAccount *provider_local.ProviderAccount,
	ownerSession *provider_local.Session,
	memberTb *testbed.Testbed,
	memberAccount *provider_local.ProviderAccount,
	memberSession *provider_local.Session,
) {
	// The owner creates the Space.
	ref, err := ownerAccount.CreateSharedObject(ctx, "offline-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := ownerAccount.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	ownerObject := mounted.(*provider_local.SharedObject)

	// Invite a writer over the owner's session transport.
	invite, err := ownerObject.CreateSOInviteOp(ctx, ownerObject.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := ownerAccount.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), ownerObject.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ownerAccount.StopSessionTransport)
	t.Cleanup(ownerAccount.StopP2PSync)

	// The member joins over its session transport.
	if err := memberAccount.EnsureConfiguredSessionTransport(ctx, memberSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(memberAccount.StopSessionTransport)
	t.Cleanup(memberAccount.StopP2PSync)
	prepareSessionTransportBackends(ctx, t, ownerAccount.GetSessionTransport(), memberAccount.GetSessionTransport())
	if _, err := memberAccount.JoinViaInvite(ctx, memberSession.GetPrivKey(), invite, ""); err != nil {
		t.Fatal(err)
	}
	memberObject := mountJoined(ctx, t, memberAccount, ownerObject.GetSharedObjectID())
	entries := memberAccount.GetSOListCtr().GetValue().GetSharedObjects()
	memberRef := entries[slices.IndexFunc(entries, func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == ownerObject.GetSharedObjectID()
	})].GetRef()

	// Both members replay the Space into a World.
	owner := &offlineMember{t: t, name: "owner", tb: ownerTb, account: ownerAccount, object: ownerObject, ref: ref}
	member := &offlineMember{t: t, name: "member", tb: memberTb, account: memberAccount, object: memberObject, ref: memberRef}
	for _, m := range []*offlineMember{owner, member} {
		m.start(ctx)
		t.Cleanup(m.stop)
	}

	// Disconnect them. Each creates its own object and then the shared one,
	// and sees both at once.
	ownerAccount.StopP2PSync()
	memberAccount.StopP2PSync()
	for _, m := range []*offlineMember{owner, member} {
		m.create(ctx, m.name+"-own")
		m.create(ctx, "shared")
		m.requireObjects(ctx, m.name+"-own", "shared")
	}

	// The member's World engine restarts offline and keeps its edits.
	member.stop()
	member.start(ctx)
	member.requireObjects(ctx, "member-own", "shared")

	// Reconnect, and wait until each holds the other's two operations.
	for _, m := range []*offlineMember{owner, member} {
		if err := m.account.StartPersistentP2PSync(ctx, m.account.GetSessionTransport()); err != nil {
			t.Fatal(err)
		}
	}
	owner.waitOperations(ctx, member.object.GetPeerID().String(), 2)
	member.waitOperations(ctx, owner.object.GetPeerID().String(), 2)

	// Both reach the same World root block with every object. Each account
	// keeps it in its own bucket.
	ownerRoot := owner.root(ctx)
	if memberRoot := member.root(ctx); !memberRoot.EqualVT(ownerRoot) {
		t.Fatalf("member World root %v; want the owner's %v", memberRoot, ownerRoot)
	}
	owner.requireObjects(ctx, "owner-own", "member-own", "shared")

	// Exactly one member is told that its creation lost to the other.
	var rejected []*sobject.SORejectedEdit
	var loser, winner *offlineMember
	for _, m := range []*offlineMember{owner, member} {
		edits := m.health(ctx).GetRejectedEdits()
		if len(edits) != 0 {
			loser = m
		} else {
			winner = m
		}
		rejected = append(rejected, edits...)
	}
	if len(rejected) != 1 || loser == nil || winner == nil {
		t.Fatalf("rejected edits %v; want one, on one member", rejected)
	}
	edit := rejected[0]
	if want := []string{winner.object.GetPeerID().String()}; edit.GetReason() == "" || !slices.Equal(edit.GetLostToPeerIds(), want) {
		t.Fatalf("%s rejected edit %v; want a reason and a loss to %q", loser.name, edit, want)
	}
}

// addLocalSession creates another local account and session on the provider of
// tb, as a second user of one daemon.
func addLocalSession(ctx context.Context, t *testing.T, tb *testbed.Testbed) (*provider_local.ProviderAccount, *provider_local.Session) {
	// Look up the local provider.
	t.Helper()
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	localProv := prov.(*provider_local.Provider)

	// Create the account and its session.
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	// Access the session's account.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	acc, releaseAcc, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAcc)
	account := acc.(*provider_local.ProviderAccount)

	// Mount the session on it.
	sess, releaseSess, err := account.MountSession(ctx, sessRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSess)
	return account, sess.(*provider_local.Session)
}

// start starts the World engine and waits for its World.
func (m *offlineMember) start(ctx context.Context) {
	// Start the engine controller.
	t := m.t
	t.Helper()
	conf := sobject_world_engine.NewConfig("offline-world-"+m.name, m.ref)
	ctrl, _, ref, err := sobject_world_engine.StartEngineWithConfig(ctx, m.tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the replayed World.
	m.release = ref
	m.engine, err = ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

// stop stops the World engine.
func (m *offlineMember) stop() {
	if m.release != nil {
		m.release.Release()
		m.release = nil
	}
}

// create creates object key in its own operation.
func (m *offlineMember) create(ctx context.Context, key string) {
	m.t.Helper()
	m.write(ctx, func(tx world.Tx) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample(m.name+" wrote "+key), true)
			return nil
		})
		world.ReleaseObjectState(obj)
		return err
	})
}

// write commits one transaction.
func (m *offlineMember) write(ctx context.Context, fn func(tx world.Tx) error) {
	// Open the transaction.
	t := m.t
	t.Helper()
	tx, err := m.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Apply and commit it.
	if err := fn(tx); err != nil {
		t.Fatalf("%s: %v", m.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("%s commit: %v", m.name, err)
	}
}

// requireObjects checks that the World holds every key.
func (m *offlineMember) requireObjects(ctx context.Context, keys ...string) {
	// Read the World as of the latest operation set.
	t := m.t
	t.Helper()
	m.sync(ctx)
	tx, err := m.engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Look up each key.
	for _, key := range keys {
		obj, found, err := tx.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("%s World lacks %q", m.name, key)
		}
	}
}

// waitOperations waits until the Space holds n operations by author.
func (m *offlineMember) waitOperations(ctx context.Context, author string, n int) {
	// Watch the readable state.
	t := m.t
	t.Helper()
	states, release, err := m.object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Count the author's operations in each state.
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		// Read the snapshot's operation set.
		if snapshot == nil {
			return false, nil
		}
		set, err := snapshot.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}

		// Count the operations the author signed.
		count := 0
		for _, h := range set.Order() {
			if set.Get(h).GetPeerId() == author {
				count++
			}
		}
		return count >= n, nil
	}, nil); err != nil {
		t.Fatalf("%s did not receive %d operations by %s: %v", m.name, n, author, err)
	}
}

// sync replays the latest operation set into the World, as each write
// transaction does before it forks.
func (m *offlineMember) sync(ctx context.Context) {
	m.t.Helper()
	tx, err := m.engine.NewTransaction(ctx, true)
	if err != nil {
		m.t.Fatal(err)
	}
	tx.Discard()
}

// root replays the latest operation set and returns the World root block.
func (m *offlineMember) root(ctx context.Context) *block.BlockRef {
	// Bring the World up to the latest operation set.
	t := m.t
	t.Helper()
	m.sync(ctx)

	// Read its root.
	var root *block.BlockRef
	if err := m.engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		root = cursor.GetRef().GetRootRef().CloneVT()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return root
}

// health returns the current health of the Space.
func (m *offlineMember) health(ctx context.Context) *sobject.SharedObjectHealth {
	// Read the current health value.
	t := m.t
	t.Helper()
	healths, release, err := m.object.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	return healths.GetValue()
}
