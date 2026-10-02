//go:build !tinygo && !goscript

package spacewave_chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	chat "github.com/s4wave/spacewave/sdk/chat"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	"github.com/s4wave/spacewave/testbed"
)

// TestChatResourceCrossSessionAppend retains sends prepared against independent
// Session snapshots and presents the accepted order from both replicas.
func TestChatResourceCrossSessionAppend(t *testing.T) {
	t.Run("sender-retry", func(t *testing.T) {
		testChatResourceCrossSessionAppend(t, "same-transaction")
	})
	t.Run("unkeyed", func(t *testing.T) {
		testChatResourceCrossSessionAppend(t, "")
	})
}

// testChatResourceCrossSessionAppend checks keyed and unkeyed sends through replica replay.
func testChatResourceCrossSessionAppend(t *testing.T, transactionID string) {
	// Start a testbed for the provider and World engines.
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Start the local provider that hosts both Sessions.
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	_, providerController, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local", PeerId: tb.Volume.GetPeerID().String(), StorageId: tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerController.Release)

	// Give two authenticated Sessions independent account stores on a local network.
	rawProvider, providerRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	local := rawProvider.(*provider_local.Provider)
	owner, ownerSession := newChatSession(t, ctx, local)
	writer, writerSession := newChatSession(t, ctx, local)

	// Create and mount the owner's Space.
	ref, err := owner.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ownerObject, releaseOwnerObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOwnerObject)

	// Create the channel before inviting the second Session to its own replica.
	ownerEngine := mountChatEngine(t, ctx, tb, ref, "chat-owner")
	ownerWorld := world.NewEngineWorldState(ownerEngine, true)
	if _, _, err := ownerWorld.ApplyWorldOp(ctx, &chat.CreateChatChannelOp{
		ObjectKey: chat.GeneralChannelKey, Name: "General", Timestamp: timestamppb.Now(),
	}, ownerSession.GetPeerId()); err != nil {
		t.Fatal(err)
	}

	// Seed history the writer will receive with its replica.
	first, err := chat.NewChatResource(ctx, ownerWorld, ownerEngine, chat.GeneralChannelKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Close)
	seed, err := first.SendMessage(ctx, &chat_rpc.SendMessageRequest{Text: "shared history", TransactionId: "seed"})
	if err != nil {
		t.Fatal(err)
	}

	// Invite the writer directly.
	host := ownerObject.(sobject.InviteHost)
	invite, err := host.CreateSOInviteOp(ctx, host.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, TargetPeerId: writerSession.GetPeerId().String(), MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), host.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}

	// Join as the writer and mount its replica.
	joined, err := writer.JoinViaInvite(ctx, writerSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatal(err)
	}
	writerRef := sobject.NewSharedObjectRef("local", writer.GetAccountID(), joined.SharedObjectID, provider_local.SobjectBlockStoreID(joined.SharedObjectID))
	writerObject, releaseWriterObject, err := writer.MountSharedObject(ctx, writerRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseWriterObject)

	// Retain the seed graph so replica reads and disconnected preparation need no remote blocks.
	snapshot, err := ownerObject.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := snapshot.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := &sobject_world_engine.InnerState{}
	if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
		t.Fatal(err)
	}
	if err := block.CopyGraph(ctx, ownerObject.GetBlockStore(), writerObject.GetBlockStore(), head.GetHeadRef().GetRootRef(), nil); err != nil {
		t.Fatal(err)
	}

	// The writer's replica history starts with the seed.
	writerEngine := mountChatEngine(t, ctx, tb, writerRef, "chat-writer")
	writerWorld := world.NewEngineWorldState(writerEngine, true)
	second, err := chat.NewChatResource(ctx, writerWorld, writerEngine, chat.GeneralChannelKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	initial, err := second.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	requireSessionHistory(t, initial.GetMessages(), []string{seed.GetMessageKey()})

	// Keep the writer on the common history while the owner accepts its next send.
	owner.StopP2PSync()
	writer.StopP2PSync()
	request := &chat_rpc.SendMessageRequest{Text: "owner message", TransactionId: transactionID}
	accepted, err := first.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	// The writer has not observed the owner's send.
	stale, err := second.GetChannelInfo(ctx, &chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if stale.GetMessageCount() != 1 {
		t.Fatal("writer synchronized before preparing its append")
	}

	// Watch the writer's SharedObject state for its queued append.
	states, releaseStates, err := writerObject.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseStates)

	// Send the writer's append with the owner's transaction identifier.
	writerRequest := &chat_rpc.SendMessageRequest{Text: "writer message", TransactionId: request.GetTransactionId()}
	var writerAccepted *chat_rpc.SendMessageResponse
	var writerErr error
	done := make(chan struct{})
	go func() {
		writerAccepted, writerErr = second.SendMessage(ctx, writerRequest)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Wait for the stale append to enter the real SharedObject queue before sync.
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		queued, local, err := snapshot.GetOpQueue(ctx)
		return len(queued)+len(local) != 0, err
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Resume sync so the writer replays its append on the owner's history.
	if err := owner.StartPersistentP2PSync(ctx, owner.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	if err := writer.StartPersistentP2PSync(ctx, writer.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if writerErr != nil {
			t.Fatal(writerErr)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if writerAccepted.GetMessageKey() == accepted.GetMessageKey() {
		t.Fatal("different senders shared a transaction message key")
	}

	// Fence each engine against the accepted SharedObject snapshot before readback.
	for _, engine := range []world.Engine{ownerEngine, writerEngine} {
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		tx.Discard()
	}
	want := []string{seed.GetMessageKey(), accepted.GetMessageKey(), writerAccepted.GetMessageKey()}
	for _, resource := range []*chat.ChatResource{first, second} {
		page, err := resource.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
		if err != nil {
			t.Fatal(err)
		}
		requireSessionHistory(t, page.GetMessages(), want)
		for i, text := range []string{"shared history", request.GetText(), writerRequest.GetText()} {
			if page.GetMessages()[i].GetText() != text {
				t.Fatalf("message %d content changed during replay", i)
			}
			engine := ownerEngine
			if i == 2 {
				engine = writerEngine
			}
			device, person, err := engine.OperationAuthor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if page.GetMessages()[i].GetSenderPeerId() != device.String() {
				t.Fatalf("message %d lost its authenticated author during replay", i)
			}
			if page.GetMessages()[i].GetPersonId() != person {
				t.Fatalf("message %d lost its person attribution during replay", i)
			}
		}

		// A new watch must project the same complete order as paginated history.
		client := chat_rpc.NewSRPCChatResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(resource.GetMux()))))
		stream, err := client.WatchMessages(ctx, &chat_rpc.WatchMessagesRequest{})
		if err != nil {
			t.Fatal(err)
		}
		batch, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		stream.Close()
		requireSessionHistory(t, batch.GetMessages(), want)
	}

	// Each device retry resolves its stable key without advancing channel history.
	if transactionID == "" {
		return
	}
	for i, resource := range []*chat.ChatResource{first, second} {
		retryRequest := []*chat_rpc.SendMessageRequest{request, writerRequest}[i]
		retry, err := resource.SendMessage(ctx, retryRequest)
		if err != nil {
			t.Fatal(err)
		}
		if retry.GetMessageKey() != want[i+1] {
			t.Fatal("retry changed the accepted message key")
		}
		page, err := resource.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
		if err != nil {
			t.Fatal(err)
		}
		requireSessionHistory(t, page.GetMessages(), want)

		// Conflicting retries fail; explicit reuse keeps the original accepted body.
		conflict := retryRequest.CloneVT()
		conflict.Text = "changed retry"
		if _, err := resource.SendMessage(ctx, conflict); err == nil {
			t.Fatal("conflicting retry changed an accepted message")
		}
		conflict.ReuseAcceptedTransaction = true
		reused, err := resource.SendMessage(ctx, conflict)
		if err != nil {
			t.Fatal(err)
		}
		if reused.GetMessageKey() != want[i+1] {
			t.Fatal("explicit reuse changed an accepted message key")
		}
	}
}

// newChatSession mounts an isolated account and retains its authenticated Session.
func newChatSession(t *testing.T, ctx context.Context, local *provider_local.Provider) (*provider_local.ProviderAccount, session.Session) {
	t.Helper()

	// Create independent account storage within the provider's local network.
	ref, err := local.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, releaseAccount, err := local.AccessProviderAccount(ctx, ref.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAccount)
	account := raw.(*provider_local.ProviderAccount)
	sess, releaseSession, err := account.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)
	t.Cleanup(account.StopSessionTransport)
	t.Cleanup(account.StopP2PSync)
	return account, sess
}

// mountChatEngine retains a real SharedObject World engine on the replica's store.
func mountChatEngine(t *testing.T, ctx context.Context, tb *testbed.Testbed, ref *sobject.SharedObjectRef, id string) world.Engine {
	t.Helper()

	// Keep the engine controller mounted until all chat reads and writes finish.
	controller, _, release, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus, sobject_world_engine.NewConfig(id, ref), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release.Release)
	controller.SetStaticLookupOp(optypes.LookupWorldOp)
	engine, err := controller.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

// requireSessionHistory checks complete, contiguous history and stable identities.
func requireSessionHistory(t *testing.T, messages []*chat_rpc.ChatMessageInfo, keys []string) {
	t.Helper()

	// A page must retain every accepted message exactly once in the same order.
	if len(messages) != len(keys) {
		t.Fatalf("history has %d messages, want %d", len(messages), len(keys))
	}
	for i, message := range messages {
		if message.GetObjectKey() != keys[i] {
			t.Fatalf("message %d key = %q, want %q", i, message.GetObjectKey(), keys[i])
		}
		if message.GetIndex() != uint64(i) {
			t.Fatalf("message %d index = %d", i, message.GetIndex())
		}
	}
}
