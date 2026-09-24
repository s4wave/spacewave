package provider_local

import (
	"bytes"
	"context"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/blocktype"
	blocktype_controller "github.com/s4wave/spacewave/db/blocktype/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// TestPairingAccountChoice verifies sign-in and merge between populated accounts,
// including bilateral preview and preservation of source recovery.
func TestPairingAccountChoice(t *testing.T) {
	for _, outcome := range []pairing.AccountOutcome{
		pairing.AccountOutcome_AccountOutcome_SIGN_IN_OFFERED,
		pairing.AccountOutcome_AccountOutcome_SIGN_IN_RECEIVING,
		pairing.AccountOutcome_AccountOutcome_MERGE_INTO_OFFERED,
		pairing.AccountOutcome_AccountOutcome_MERGE_INTO_RECEIVING,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			// Mount two isolated stores with their existing account Sessions.
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			t.Cleanup(cancel)
			network := inproc.NewNetwork()
			var accounts []*ProviderAccount
			var sessions []*Session
			var controllers []session.SessionController
			var spaces []*sobject.SharedObjectRef
			var payloadRefs []*block.BlockRef
			var payloads [][]byte
			for range 2 {
				tb, _, account, sess, release := setupProviderAndSessionInternal(ctx, t, func(p *Provider) {
					p.localNetwork = transport.WithInprocNetwork(network)
				})
				t.Cleanup(release)
				if err := account.EnsureConfiguredSessionTransport(ctx, sess.GetPrivKey()); err != nil {
					t.Fatal(err)
				}
				decoder := blocktype_controller.NewController(func(_ context.Context, typeID string) (blocktype.BlockType, error) {
					if typeID == "test/replica-payload" {
						return blocktype.NewBlockType(typeID, block_mock.NewRootBlock), nil
					}
					return nil, nil
				})
				releaseDecoder, err := account.t.p.b.AddController(ctx, decoder, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseDecoder)
				tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
				_, reference, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{VolumeId: tb.EngineVolumeID}), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(reference.Release)
				controller, lookup, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(lookup.Release)
				if _, err := controller.RegisterSession(ctx, sess.GetSessionRef(), nil); err != nil {
					t.Fatal(err)
				}
				ref, err := account.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
				if err != nil {
					t.Fatal(err)
				}
				accounts = append(accounts, account)
				sessions = append(sessions, sess)
				controllers = append(controllers, controller)
				spaces = append(spaces, ref)
				payloadRef, payload := seedAccountReplicaPayload(ctx, t, account, ref)
				payloadRefs = append(payloadRefs, payloadRef)
				payloads = append(payloads, payload)
			}

			// Exchange both selected accounts over the production pairing protocol.
			engines := []*pairing.Engine{pairingEngineForTest(t, sessions[0]), pairingEngineForTest(t, sessions[1])}
			left, right := net.Pipe()
			t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
			finished := make(chan struct{}, 2)
			go func() {
				engines[0].StartOnStream(ctx, left, sessions[1].GetPeerId(), true, true, "")
				finished <- struct{}{}
			}()
			go func() {
				engines[1].StartOnStream(ctx, right, sessions[0].GetPeerId(), false, true, "")
				finished <- struct{}{}
			}()
			for _, engine := range engines {
				waitForPairingStatus(ctx, t, engine, pairing.StatusSelectingAccount)
				snapshot, _ := engine.Snapshot()
				if snapshot.Choice.GetOfferedAccount().GetAccountId() != accounts[0].GetAccountID() || snapshot.Choice.GetReceivingAccount().GetAccountId() != accounts[1].GetAccountID() {
					t.Fatal("account choice did not identify both existing accounts")
				}
			}
			if err := engines[0].SelectAccount(outcome); err == nil {
				t.Fatal("code creator changed the receiving client's proposal")
			}
			if err := engines[1].SelectAccount(outcome); err != nil {
				t.Fatal(err)
			}

			// Both clients review the same outcome before either account gains access.
			for i, engine := range engines {
				waitForPairingStatus(ctx, t, engine, pairing.StatusVerifyingEmoji)
				snapshot, _ := engine.Snapshot()
				if snapshot.Choice.GetOutcome() != outcome {
					t.Fatal("approval screen changed the proposed account outcome")
				}
				entries, err := controllers[i].ListSessions(ctx)
				if err != nil || len(entries) != 1 {
					t.Fatalf("unapproved choice added account access: %v, %v", entries, err)
				}
			}
			if err := engines[1].SelectAccount(outcome); err == nil {
				t.Fatal("account proposal changed after approval began")
			}
			for _, engine := range engines {
				engine.ConfirmSAS(true)
			}
			for range 2 {
				select {
				case <-ctx.Done():
					t.Fatal("selected sign-in did not finish")
				case <-finished:
				}
			}

			// The chosen direction controls attachment; both original accounts remain intact.
			source, receiving := 0, 1
			if outcome == pairing.AccountOutcome_AccountOutcome_SIGN_IN_RECEIVING || outcome == pairing.AccountOutcome_AccountOutcome_MERGE_INTO_RECEIVING {
				source, receiving = 1, 0
			}
			for _, engine := range engines {
				waitForPairingStatus(ctx, t, engine, pairing.StatusBothConfirmed)
			}
			ref, err := engines[receiving].Result(sessions[source].GetPeerId())
			if err != nil || ref.GetProviderResourceRef().GetProviderAccountId() != accounts[source].GetAccountID() {
				t.Fatalf("wrong selected account attachment: %v, %v", ref, err)
			}
			entries, err := controllers[receiving].ListSessions(ctx)
			if err != nil || len(entries) != 2 {
				t.Fatalf("sign-in did not retain the original Session: %v, %v", entries, err)
			}
			for i, account := range accounts {
				_, release, err := account.MountSharedObject(ctx, spaces[i], nil)
				if err != nil {
					t.Fatalf("sign-in lost an original Space: %v", err)
				}
				release()
			}
			account, releaseAccount, err := accounts[receiving].t.p.AccessProviderAccount(ctx, accounts[source].GetAccountID(), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(releaseAccount)
			space, releaseSpace, err := account.(*ProviderAccount).MountSharedObject(ctx, spaces[source], nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(releaseSpace)
			state, err := space.GetSharedObjectState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.GetRootInner(ctx); err != nil {
				t.Fatalf("selected account Space is not readable: %v", err)
			}
			if outcome == pairing.AccountOutcome_AccountOutcome_MERGE_INTO_OFFERED || outcome == pairing.AccountOutcome_AccountOutcome_MERGE_INTO_RECEIVING {
				merged := account.(*ProviderAccount)
				settings, err := accounts[receiving].readAccountSettings(ctx)
				if err != nil || settings.GetTransition().GetDestination().GetProviderAccountId() != accounts[source].GetAccountID() {
					t.Fatalf("merge did not persist the source redirect: %v, %v", settings, err)
				}
				moved, releaseMoved, err := merged.MountSession(ctx, ref, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseMoved)
				if moved.GetPeerId() != sessions[receiving].GetPeerId() {
					t.Fatal("account merge replaced the client's Session identity")
				}
				for _, original := range spaces {
					id := original.GetProviderResourceRef().GetId()
					if !slices.ContainsFunc(merged.soListCtr.GetValue().GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
						return entry.GetRef().GetProviderResourceRef().GetId() == id
					}) {
						t.Fatalf("merge omitted Space %s", id)
					}
				}
				copiedRef := sobject.NewSharedObjectRef(merged.GetProviderID(), merged.GetAccountID(), spaces[receiving].GetProviderResourceRef().GetId(), spaces[receiving].GetBlockStoreId())
				copied, releaseCopied, err := merged.MountSharedObject(ctx, copiedRef, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(releaseCopied)
				data, found, err := copied.GetBlockStore().(*BlockStore).store.GetBlock(ctx, payloadRefs[receiving])
				if err != nil || !found || !bytes.Equal(data, payloads[receiving]) {
					t.Fatalf("merge completed without a durable file copy: found=%v err=%v", found, err)
				}
			}
		})
	}
}
