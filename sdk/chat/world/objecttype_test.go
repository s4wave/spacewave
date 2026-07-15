package spacewave_chat_world

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

func TestChatChannelFactoryAllowsAnonymousReadResource(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	invoker, cleanup, err := ChatChannelFactory(
		ctx,
		nil,
		nil,
		nil,
		ws,
		"chat/general",
	)
	if err != nil {
		t.Fatalf("ChatChannelFactory: %v", err)
	}
	if invoker == nil {
		t.Fatal("ChatChannelFactory returned nil invoker")
	}
	if cleanup == nil {
		t.Fatal("ChatChannelFactory returned nil cleanup")
	}
	t.Cleanup(cleanup)
}
