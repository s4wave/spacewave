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
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	createBadChatMessageObject(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/0")
	createChatMessage(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/1", "second", "peer-local")
	createChatMessage(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/2", "third", "peer-local")

	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{
		BeforeKey: GeneralChannelKey + "/message/2",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	requireChatMessages(t, listResp.GetMessages(), GeneralChannelKey+"/message/1", "second", "peer-local")
}

func createBadChatMessageObject(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey string) {
	t.Helper()

	_, _, err := world.CreateWorldObject(ctx, ws, msgKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatChannel{Name: "wrong block", CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", msgKey, err)
	}
	appendChatMessageKey(t, ctx, ws, channelKey, msgKey)
}
