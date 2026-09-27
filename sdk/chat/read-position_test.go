package spacewave_chat

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	"golang.org/x/sync/errgroup"
)

// TestReadPositionFollowsPersonAcrossDevices verifies shared attribution and monotonic receipts.
func TestReadPositionFollowsPersonAcrossDevices(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	alice := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device-a", "alice")
	otherAlice := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device-b", "alice")
	bob := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "bob-device", "bob")
	for _, device := range []*ChatResource{alice, otherAlice, bob} {
		if _, err := device.SendMessage(ctx, &chat_rpc.SendMessageRequest{Text: "A shared message"}); err != nil {
			t.Fatal(err)
		}
	}
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
	resumed := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice-device-c", "alice")
	positions, err := resumed.GetReadPositions(ctx, &chat_rpc.GetReadPositionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(positions.GetPositions()) != 1 || positions.GetPositions()["alice"].GetNextIndex() != 3 {
		t.Fatal("replacement device did not recover the furthest shared read position")
	}
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
	if _, err := resumed.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 4}); err == nil {
		t.Fatal("receipt advanced beyond the retained history")
	}
}
