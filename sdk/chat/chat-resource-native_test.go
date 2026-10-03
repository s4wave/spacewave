//go:build !goscript

package spacewave_chat

import (
	"context"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

func TestChatResourceListMessagesReadsOnlyRequestedPage(t *testing.T) {
	// Start an isolated World for bounded history reads.
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Create history with a malformed message outside the requested page.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	createBadChatMessageObject(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/0")
	createChatMessage(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/1", "second", "peer-local")
	createChatMessage(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/2", "third", "peer-local")

	// Read the single valid message before the final key.
	resource := newChatResource(t, ws, wtb.Engine, GeneralChannelKey, "peer-local")
	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{
		BeforeKey: GeneralChannelKey + "/message/2",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}

	// Verify the requested page contains the valid second message.
	requireChatMessages(t, listResp.GetMessages(), GeneralChannelKey+"/message/1", "second", "peer-local")
}

func createBadChatMessageObject(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey string) {
	// Attribute malformed fixture failures to the calling test.
	t.Helper()

	// Create a malformed message block and add its key to channel history.
	createdObject, _, err := world.CreateWorldObject(ctx, ws, msgKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatChannel{Name: "wrong block", CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", msgKey, err)
	}
	appendChatMessageKey(t, ctx, ws, channelKey, msgKey)
}
