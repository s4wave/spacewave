package spacewave_chat

import (
	"errors"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestRedactionStripsOwnBodies checks that a person removes only their own bodies and keeps routing.
func TestRedactionStripsOwnBodies(t *testing.T) {
	// Create an encrypted channel so redactions must bypass the body policy.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/redaction"
	const algorithm = "m.megolm.v1.aes-sha2"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Redaction", Timestamp: timestamppb.Now(), EncryptionAlgorithm: algorithm}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}

	// Alice and Bob write through their own devices.
	alice := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(alice.Close)
	bob := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "bob-device", "bob")
	t.Cleanup(bob.Close)

	// Alice sends a thread root and an encrypted reply in it.
	body := &ChatCiphertext{Algorithm: algorithm, Ciphertext: "opaque", SenderKey: "sender", SessionId: "session"}
	root, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{TransactionId: "root", Content: &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: body}}})
	if err != nil {
		t.Fatal(err)
	}
	reply := body.CloneVT()
	reply.Relation = &ChatRelation{Type: "m.thread", TargetKey: root.GetMessageKey(), ReplyToKey: root.GetMessageKey(), IsFallingBack: true}
	replyRequest := &chat_rpc.SendMessageRequest{TransactionId: "reply", Content: &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: reply}}}
	sent, err := alice.SendMessage(ctx, replyRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Each redaction selects one target with a public reason.
	redaction := func(target string) *chat_rpc.SendMessageRequest {
		return &chat_rpc.SendMessageRequest{Content: &ChatMessageContent{Content: &ChatMessageContent_Redaction{Redaction: &ChatRedaction{TargetKey: target, Reason: "typo"}}}}
	}

	// Another person cannot remove the body.
	if _, err := bob.SendMessage(ctx, redaction(sent.GetMessageKey())); !errors.Is(err, ErrChatRedactionForbidden) {
		t.Fatalf("redacted another person's message: %v", err)
	}

	// The author's redaction keeps the history position and the thread routing.
	redacted, err := alice.SendMessage(ctx, redaction(sent.GetMessageKey()))
	if err != nil {
		t.Fatal(err)
	}
	read, err := alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	message := read.GetMessage()
	want := &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: &ChatCiphertext{Relation: &ChatRelation{Type: "m.thread", TargetKey: root.GetMessageKey()}}}}
	if message.GetRedactedByKey() != redacted.GetMessageKey() || !message.GetContent().EqualVT(want) || message.GetText() != "Message deleted" {
		t.Fatalf("redacted message kept its body: %v", message)
	}

	// The accepted send identity still resolves after its body is gone.
	retry, err := alice.SendMessage(ctx, replyRequest)
	if err != nil || retry.GetMessageKey() != sent.GetMessageKey() {
		t.Fatalf("retry after redaction: %v %v", retry, err)
	}

	// Redactions themselves are permanent history.
	if _, err := alice.SendMessage(ctx, redaction(redacted.GetMessageKey())); err == nil {
		t.Fatal("redacted a redaction")
	}
}

// TestModeratedRedaction checks that an owner or an enveloped redaction removes
// another person's body, and that a redaction of a message not held yet takes
// effect when the message arrives.
func TestModeratedRedaction(t *testing.T) {
	// Create a channel written through authenticated replay.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/moderated"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Moderated", Timestamp: timestamppb.Now()}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}

	// Each person signs with their own device.
	// owner reports the signer's role.
	apply := func(person string, owner bool, op world.Operation) error {
		authorCtx := world.WithOperationOwner(world.WithOperationPerson(ctx, person), owner)
		_, _, err := ws.ApplyWorldOp(authorCtx, op, peer.ID(person+"-device"))
		return err
	}
	send := func(person string, owner bool, req *chat_rpc.SendMessageRequest) (string, error) {
		if err := apply(person, owner, &SendChatMessageOp{ObjectKey: channelKey, Request: req, Timestamp: timestamppb.Now()}); err != nil {
			return "", err
		}
		return sendMessageKey(channelKey, peer.ID(person+"-device").String(), req)
	}
	text := func(transactionID string) *chat_rpc.SendMessageRequest {
		return &chat_rpc.SendMessageRequest{TransactionId: transactionID, Text: "hello"}
	}
	redaction := func(transactionID string, target *ChatRedaction) *chat_rpc.SendMessageRequest {
		return &chat_rpc.SendMessageRequest{TransactionId: transactionID, Content: &ChatMessageContent{Content: &ChatMessageContent_Redaction{Redaction: target}}}
	}
	envelopeKey := func(id string) string {
		key, err := EnvelopeMessageKey(channelKey, id)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	requireRedacted := func(messageKey, redactionKey string) {
		t.Helper()
		message, err := world.LookupObjectBody[ChatMessage](ctx, ws, messageKey)
		if err != nil {
			t.Fatal(err)
		}
		if message.GetRedactedByKey() != redactionKey || message.GetContent().GetText() != "" {
			t.Fatalf("message %s not redacted by %s: %v", messageKey, redactionKey, message)
		}
	}

	// Bob's message resists Carol, a writer, and yields to Dave, an owner.
	bobKey, err := send("bob", false, text("bob-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := send("carol", false, redaction("carol-1", &ChatRedaction{TargetKey: bobKey})); !errors.Is(err, ErrChatRedactionForbidden) {
		t.Fatalf("writer redacted another person's message: %v", err)
	}
	daveKey, err := send("dave", true, redaction("dave-1", &ChatRedaction{TargetKey: bobKey}))
	if err != nil {
		t.Fatal(err)
	}
	requireRedacted(bobKey, daveKey)

	// A redaction carrying its protocol's envelope has the protocol's authority.
	bobKey, err = send("bob", false, text("bob-2"))
	if err != nil {
		t.Fatal(err)
	}
	enveloped := redaction("", &ChatRedaction{TargetKey: bobKey})
	enveloped.Envelope = &ChatEnvelope{Id: "$redact", Data: []byte(`{}`)}
	envelopedKey, err := send("carol", false, enveloped)
	if err != nil {
		t.Fatal(err)
	}
	requireRedacted(bobKey, envelopedKey)

	// Only a moderator may target a message not held yet.
	early := &ChatRedaction{TargetKey: envelopeKey("$later"), TargetEnvelopeId: "$later"}
	if _, err := send("carol", false, redaction("carol-2", early)); !errors.Is(err, ErrChatRedactionForbidden) {
		t.Fatalf("writer redacted a message not held: %v", err)
	}
	if _, err := send("dave", true, redaction("dave-2", &ChatRedaction{TargetKey: bobKey, TargetEnvelopeId: "$later"})); err == nil {
		t.Fatal("accepted a target key that does not match its envelope ID")
	}

	// The early redaction strips the message sent later under its envelope.
	earlyKey, err := send("dave", true, redaction("dave-3", early))
	if err != nil {
		t.Fatal(err)
	}
	later := text("")
	later.Envelope = &ChatEnvelope{Id: "$later", Data: []byte(`{"body":"hello"}`)}
	laterKey, err := send("bob", false, later)
	if err != nil {
		t.Fatal(err)
	}
	requireRedacted(laterKey, earlyKey)

	// The early redaction also strips a message whose envelope is attached later.
	attachedKey, err := send("bob", false, text("bob-3"))
	if err != nil {
		t.Fatal(err)
	}
	attachRedactionKey, err := send("dave", true, redaction("dave-4", &ChatRedaction{TargetKey: envelopeKey("$attached"), TargetEnvelopeId: "$attached"}))
	if err != nil {
		t.Fatal(err)
	}
	attach := &AttachChatEnvelopeOp{ObjectKey: channelKey, MessageKey: attachedKey, Envelope: &ChatEnvelope{Id: "$attached", Data: []byte(`{}`)}}
	if err := apply("bob", false, attach); err != nil {
		t.Fatal(err)
	}
	requireRedacted(attachedKey, attachRedactionKey)
}
