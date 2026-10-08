package spacewave_chat

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	chat_state "github.com/s4wave/spacewave/sdk/chat/state"
	"golang.org/x/sync/errgroup"
)

// TestReadPositionFollowsPersonAcrossDevices verifies shared attribution and monotonic receipts.
func TestReadPositionFollowsPersonAcrossDevices(t *testing.T) {
	// Start an isolated World for shared-person read receipts.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create a shared channel with two devices for Alice and one for Bob.
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	alice := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device-a", "alice")
	otherAlice := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device-b", "alice")
	bob := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "bob-device", "bob")

	// Send one message from each signing device.
	for _, device := range []*ChatResource{alice, otherAlice, bob} {
		if _, err := device.SendMessage(ctx, &chat_rpc.SendMessageRequest{Text: "A shared message"}); err != nil {
			t.Fatal(err)
		}
	}

	// Read and verify shared-person attribution with distinct device identities.
	page, err := alice.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetMessages()) != 3 || page.GetMessages()[0].GetPersonId() != "alice" || page.GetMessages()[1].GetPersonId() != "alice" || page.GetMessages()[2].GetPersonId() != "bob" {
		t.Fatal("message attribution did not preserve shared person identity")
	}
	if page.GetMessages()[0].GetSenderPeerId() == page.GetMessages()[1].GetSenderPeerId() {
		t.Fatal("person attribution erased distinct device signing identities")
	}

	// Both device updates serialize through the canonical channel transaction.
	var updates errgroup.Group
	updates.Go(func() error {
		_, err := alice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 2})
		return err
	})
	updates.Go(func() error {
		_, err := otherAlice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 3})
		return err
	})
	if err := updates.Wait(); err != nil {
		t.Fatal(err)
	}

	// Read Alice's retained receipt from a replacement device.
	resumed := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device-c", "alice")
	positions, err := resumed.GetReadPositions(ctx, &chat_rpc.GetReadPositionsRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Require the replacement device to recover the furthest shared receipt.
	if len(positions.GetPositions()) != 1 || positions.GetPositions()["alice"].GetNextIndex() != 3 {
		t.Fatal("replacement device did not recover the furthest shared read position")
	}

	// Verify an older receipt preserves both position and World revision.
	seqno, err := tb.Engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := resumed.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1})
	if err != nil || retry.GetPosition().GetNextIndex() != 3 {
		t.Fatalf("older receipt moved shared position backwards: %v", err)
	}
	after, err := tb.Engine.GetSeqno(ctx)
	if err != nil || after != seqno {
		t.Fatalf("unchanged receipt rewrote the World: %v", err)
	}

	// Advance Bob's receipt and verify each person's position stays independent.
	if _, err := bob.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1}); err != nil {
		t.Fatal(err)
	}
	positions, err = resumed.GetReadPositions(ctx, &chat_rpc.GetReadPositionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if positions.GetPositions()["alice"].GetNextIndex() != 3 || positions.GetPositions()["bob"].GetNextIndex() != 1 {
		t.Fatal("one person's receipt changed another person's position")
	}

	// Require receipt advancement to stay within retained channel history.
	if _, err := resumed.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 4}); err == nil {
		t.Fatal("receipt advanced beyond the retained history")
	}
}

// TestThreadReadPositions advances timeline positions independently of the
// channel position, which drops the timeline positions it passes.
func TestThreadReadPositions(t *testing.T) {
	// Build a channel with a thread root, one reply, and a main message.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	alice := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device", "alice")
	root := sendThreadTestEvent(t, ctx, alice, "root", nil)
	sendThreadTestEvent(t, ctx, alice, "reply", &ChatRelation{Type: "m.thread", TargetKey: root})
	sendThreadTestEvent(t, ctx, alice, "main", nil)

	// Each update returns the person's retained position.
	update := func(nextIndex uint64, threadRootKey *string) *chat_state.ChatReadPosition {
		t.Helper()
		response, err := alice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: nextIndex, ThreadRootKey: threadRootKey})
		if err != nil {
			t.Fatal(err)
		}
		return response.GetPosition()
	}

	// Main and thread positions advance without moving each other or the channel position.
	main := ""
	update(1, &main)
	position := update(2, &root)
	if position.GetNextIndex() != 0 || position.GetThreadPositions()[""].GetNextIndex() != 1 || position.GetThreadPositions()[root].GetNextIndex() != 2 {
		t.Fatalf("timeline positions did not advance independently: %v", position)
	}
	if position := update(1, &root); position.GetThreadPositions()[root].GetNextIndex() != 2 {
		t.Fatal("older thread receipt moved its position backwards")
	}
	other := GeneralChannelKey + "/message/missing"
	if _, err := alice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1, ThreadRootKey: &other}); err == nil {
		t.Fatal("receipt accepted a missing thread root")
	}

	// The channel position subsumes the main position it passes and keeps the thread ahead of it.
	position = update(1, nil)
	if position.GetNextIndex() != 1 || len(position.GetThreadPositions()) != 1 || position.GetThreadPositions()[root].GetNextIndex() != 2 {
		t.Fatalf("channel receipt kept a subsumed timeline position: %v", position)
	}
	if position := update(1, &main); position.GetThreadPositions()[""] != nil {
		t.Fatal("timeline receipt behind the channel position was retained")
	}
}

// TestAuthorReadPositions gives each author its own position beside its person's.
func TestAuthorReadPositions(t *testing.T) {
	// Build a channel with two messages and one participant.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	alice := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device", "alice")
	for _, text := range []string{"first", "second"} {
		if _, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	// Read as an author and then as the person, which keeps its own position.
	if _, err := alice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 2, Author: "deadlock"}); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1}); err != nil {
		t.Fatal(err)
	}
	response, err := alice.GetReadPositions(ctx, &chat_rpc.GetReadPositionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	positions := response.GetPositions()
	person, author := positions["alice"], positions["alice.deadlock"]
	if len(positions) != 2 || person.GetNextIndex() != 1 || person.GetAuthor() != "" {
		t.Fatalf("author moved the person's position: %v", positions)
	}
	if author.GetNextIndex() != 2 || author.GetAuthor() != "deadlock" {
		t.Fatalf("author lost its own position: %v", positions)
	}

	// Reject an author that is not a DNS label.
	if _, err := alice.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1, Author: "Dead.Lock"}); err == nil {
		t.Fatal("accepted an author that is not a DNS label")
	}
}
