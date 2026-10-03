package spacewave_chat

import (
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestChannelRelationsRetainReplayAndAuthority checks public metadata without weakening body encryption.
func TestChannelRelationsRetainReplayAndAuthority(t *testing.T) {
	// Create an encrypted channel through the production World operation.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the encrypted channel and attach its accepted author.
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/relations"
	const algorithm = "m.megolm.v1.aes-sha2"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Relations", Timestamp: timestamppb.Now(), EncryptionAlgorithm: algorithm}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	channel := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(channel.Close)

	// Send the encrypted root message for relationship targets.
	rootContent := &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: &ChatCiphertext{Algorithm: algorithm, Ciphertext: "opaque", SenderKey: "sender", SessionId: "session"}}}
	root, err := channel.SendMessage(ctx, &chat_rpc.SendMessageRequest{TransactionId: "root", Content: rootContent})
	if err != nil {
		t.Fatal(err)
	}

	// Both a public annotation and an encrypted thread retain their exact identity and metadata.
	thread := rootContent.CloneVT()
	thread.GetCiphertext().Relation = &ChatRelation{Type: "m.thread", TargetKey: root.GetMessageKey(), ReplyToKey: root.GetMessageKey(), IsFallingBack: true}
	annotation := &ChatMessageContent{Content: &ChatMessageContent_Annotation{Annotation: &ChatAnnotation{TargetKey: root.GetMessageKey(), Key: "👍"}}}
	for _, item := range []struct {
		name    string
		content *ChatMessageContent
	}{{"thread", thread}, {"annotation", annotation}} {
		t.Run(item.name, func(t *testing.T) {
			// Prepare and send the selected relationship content.
			request := &chat_rpc.SendMessageRequest{TransactionId: item.name, Content: item.content}
			sent, err := channel.SendMessage(ctx, request)
			if err != nil {
				t.Fatal(err)
			}

			// Verify a repeated relationship send preserves its identity.
			retry, err := channel.SendMessage(ctx, request)
			if err != nil || retry.GetMessageKey() != sent.GetMessageKey() {
				t.Fatalf("retry changed event: %v %v", retry, err)
			}

			// Verify stored relationship content and author attribution.
			read, err := channel.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
			if err != nil || !read.GetMessage().GetContent().EqualVT(item.content) || read.GetMessage().GetPersonId() != "alice" || read.GetMessage().GetText() != "" {
				t.Fatalf("retained content or attribution changed: %v %v", read, err)
			}

			// Require changed relationship metadata to conflict with its accepted transaction.
			conflict := request.CloneVT()
			if value := conflict.GetContent().GetAnnotation(); value != nil {
				value.Key = "👎"
			}
			if value := conflict.GetContent().GetCiphertext(); value != nil {
				value.Relation.IsFallingBack = false
			}
			if _, err := channel.SendMessage(ctx, conflict); err == nil {
				t.Fatal("changed metadata reused the accepted transaction")
			}
		})
	}

	// Cross-channel and absent targets fail before reserving a history position.
	for _, target := range []string{"other/message/root", channelKey + "/message/missing"} {
		invalid := annotation.CloneVT()
		invalid.GetAnnotation().TargetKey = target
		if _, err := channel.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: invalid}); err == nil {
			t.Fatal("accepted unavailable annotation target")
		}
		invalid = thread.CloneVT()
		invalid.GetCiphertext().Relation.TargetKey = target
		if _, err := channel.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: invalid}); err == nil {
			t.Fatal("accepted unavailable encrypted relation target")
		}
	}
	if _, err := channel.SendMessage(ctx, &chat_rpc.SendMessageRequest{Text: "private body"}); err == nil {
		t.Fatal("public reactions enabled plaintext message bodies")
	}
	info, err := channel.GetChannelInfo(ctx, &chat_rpc.GetChannelInfoRequest{})
	if err != nil || info.GetMessageCount() != 3 {
		t.Fatalf("rejected sends changed history: %v %v", info, err)
	}
}
