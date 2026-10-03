package spacewave_chat

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// interruptedFenceEngine injects one failed durability acknowledgment over a real World.
type interruptedFenceEngine struct {
	// Engine executes real transactions and reads.
	world.Engine
	// failNext selects one interrupted fence in this sequential test.
	failNext bool
}

// Sync preserves committed state while simulating an interrupted acknowledgment.
func (e *interruptedFenceEngine) Sync(ctx context.Context) (bool, error) {
	if e.failNext {
		e.failNext = false
		return false, errors.New("interrupted storage fence")
	}
	return e.Engine.Sync(ctx)
}

// TestChannelRetriesAfterInterruptedFence verifies uncertain writes remain retryable without duplication.
func TestChannelRetriesAfterInterruptedFence(t *testing.T) {
	// Start an isolated World for interrupted durability acknowledgments.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Attach a channel Resource with an interrupted storage fence.
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	engine := &interruptedFenceEngine{Engine: tb.Engine, failNext: true}
	resource := newChatResource(t, ws, engine, GeneralChannelKey, "alice")

	// Require the first send acknowledgment to fail at the storage fence.
	request := &chat_rpc.SendMessageRequest{Text: "Retain this send", TransactionId: "one-send"}
	if _, err := resource.SendMessage(ctx, request); err == nil {
		t.Fatal("acknowledged a send before its durability fence succeeded")
	}

	// Retry the uncertain send after the fence interruption.
	retry, err := resource.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	// Read and verify that retry retained a single accepted message.
	page, err := resource.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetMessages()) != 1 || page.GetMessages()[0].GetObjectKey() != retry.GetMessageKey() {
		t.Fatal("retry duplicated or replaced the uncertain send")
	}

	// Interrupt a read-receipt acknowledgment and verify its retry retains the position.
	engine.failNext = true
	if _, err := resource.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1}); err == nil {
		t.Fatal("acknowledged a receipt before its durability fence succeeded")
	}
	position, err := resource.UpdateReadPosition(ctx, &chat_rpc.UpdateReadPositionRequest{NextIndex: 1})
	if err != nil || position.GetPosition().GetNextIndex() != 1 {
		t.Fatalf("retry lost the uncertain read position: %v", err)
	}
}
