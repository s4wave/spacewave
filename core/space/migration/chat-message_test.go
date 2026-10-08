package space_migration_test

import (
	"context"
	"testing"

	space_migration "github.com/s4wave/spacewave/core/space/migration"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_chat "github.com/s4wave/spacewave/sdk/chat"
)

// TestChatMessageRewriteKeepsAuthor proves a rewritten chat message keeps its sender, person, and author.
func TestChatMessageRewriteKeepsAuthor(t *testing.T) {
	// Store a message sent by an author and replying to another message.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	message := &s4wave_chat.ChatMessage{SenderPeerId: "peer", PersonId: "alice", Author: "deadlock", ReplyToKey: "root"}
	setObjectBlock(t, ctx, tb.WorldState, "message", s4wave_chat.ChatMessageTypeID, message)

	// Rewrite it under a mapping that renames the reply target.
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mapping := space_migration.NewIdentityMap()
	mapping.ObjectKeys["root"] = "moved"
	object := &space_migration.ObjectDescriptor{ObjectKey: "message", ObjectType: s4wave_chat.ChatMessageTypeID, World: tb.WorldState}
	result, err := registry.Lookup(s4wave_chat.ChatMessageTypeID).Rewrite(ctx, object, mapping)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the rewrite moved only the reply key.
	rewritten := &s4wave_chat.ChatMessage{}
	if err := rewritten.UnmarshalBlock(result.Payload); err != nil {
		t.Fatal(err)
	}
	if rewritten.GetSenderPeerId() != "peer" || rewritten.GetPersonId() != "alice" || rewritten.GetAuthor() != "deadlock" || rewritten.GetReplyToKey() != "moved" {
		t.Fatalf("rewrite produced %v", rewritten)
	}
}
