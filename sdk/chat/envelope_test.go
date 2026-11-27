package spacewave_chat

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestEnvelopeStoredOnce checks that an envelope is stored once under its ID, whoever
// delivers it, and that its redaction swaps in the stored redacted form.
func TestEnvelopeStoredOnce(t *testing.T) {
	// Start a World testbed.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the channel and its two writers.
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/envelope"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Envelope", Timestamp: timestamppb.Now()}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	alice := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(alice.Close)
	bob := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "bob-device", "bob")
	t.Cleanup(bob.Close)

	// Alice stores an envelope with the content read from it.
	envelope := &ChatEnvelope{Id: "$event", Data: []byte(`{"type":"m.room.message"}`), RedactedData: []byte(`{}`)}
	request := &chat_rpc.SendMessageRequest{
		Content:  &ChatMessageContent{Content: &ChatMessageContent_Event{Event: &ChatEvent{Type: "m.room.message", ContentJson: `{"body":"hi"}`}}},
		Envelope: envelope,
	}
	sent, err := alice.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	wantKey, err := EnvelopeMessageKey(channelKey, envelope.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if sent.GetMessageKey() != wantKey {
		t.Fatalf("envelope key %q, want %q", sent.GetMessageKey(), wantKey)
	}

	// Bob delivers the same envelope and reaches the accepted message.
	again, err := bob.SendMessage(ctx, request)
	if err != nil || again.GetMessageKey() != sent.GetMessageKey() {
		t.Fatalf("repeated envelope: %v %v", again, err)
	}
	info, err := alice.GetChannelInfo(ctx, &chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetMessageCount() != 1 {
		t.Fatalf("channel holds %d messages, want 1", info.GetMessageCount())
	}
	read, err := alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	if !read.GetMessage().GetEnvelope().EqualVT(envelope) {
		t.Fatalf("envelope read back as %v", read.GetMessage().GetEnvelope())
	}

	// A redaction swaps in the redacted form the envelope stored.
	redact := &chat_rpc.SendMessageRequest{Content: &ChatMessageContent{Content: &ChatMessageContent_Redaction{Redaction: &ChatRedaction{TargetKey: sent.GetMessageKey()}}}}
	if _, err := alice.SendMessage(ctx, redact); err != nil {
		t.Fatal(err)
	}
	read, err = alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	if got := read.GetMessage().GetEnvelope(); !bytes.Equal(got.GetData(), envelope.GetRedactedData()) || len(got.GetRedactedData()) != 0 {
		t.Fatalf("redacted envelope holds %q and %q", got.GetData(), got.GetRedactedData())
	}
}

// TestAttachEnvelope checks that attaching sets an absent envelope once, refuses
// an ID identifying another message, and stores the redacted form on a redacted
// message with its prelude intact.
func TestAttachEnvelope(t *testing.T) {
	// Start a World testbed.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the channel and its two writers.
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/attach"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Attach", Timestamp: timestamppb.Now()}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	alice := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(alice.Close)
	bob := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "bob-device", "bob")
	t.Cleanup(bob.Close)

	// Alice sends a message without an envelope.
	text := &ChatMessageContent{Content: &ChatMessageContent_Text{Text: "hi"}}
	plain, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: text})
	if err != nil {
		t.Fatal(err)
	}

	// The first attachment stands. A second returns it.
	first := &ChatEnvelope{Id: "$first", Data: []byte(`{"n":1}`), RedactedData: []byte(`{}`), Prelude: []*ChatEnvelope{{Id: "$join", Data: []byte(`{}`)}}}
	attached, err := alice.AttachEnvelope(ctx, &chat_rpc.AttachEnvelopeRequest{MessageKey: plain.GetMessageKey(), Envelope: first})
	if err != nil || !attached.GetEnvelope().EqualVT(first) {
		t.Fatalf("first attachment: %v %v", attached, err)
	}
	second := &ChatEnvelope{Id: "$second", Data: []byte(`{"n":2}`)}
	attached, err = bob.AttachEnvelope(ctx, &chat_rpc.AttachEnvelopeRequest{MessageKey: plain.GetMessageKey(), Envelope: second})
	if err != nil || !attached.GetEnvelope().EqualVT(first) {
		t.Fatalf("second attachment: %v %v", attached, err)
	}

	// An envelope ID identifying a message stored under its envelope key is
	// refused.
	sent, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: text, Envelope: &ChatEnvelope{Id: "$sent", Data: []byte(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: text})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alice.AttachEnvelope(ctx, &chat_rpc.AttachEnvelopeRequest{MessageKey: other.GetMessageKey(), Envelope: &ChatEnvelope{Id: "$sent", Data: []byte(`{}`)}}); err == nil {
		t.Fatalf("attached an ID naming %s", sent.GetMessageKey())
	}

	// Attaching to a redacted message stores the redacted form.
	redact := &chat_rpc.SendMessageRequest{Content: &ChatMessageContent{Content: &ChatMessageContent_Redaction{Redaction: &ChatRedaction{TargetKey: other.GetMessageKey()}}}}
	if _, err := alice.SendMessage(ctx, redact); err != nil {
		t.Fatal(err)
	}
	late := &ChatEnvelope{Id: "$late", Data: []byte(`{"body":"hi"}`), RedactedData: []byte(`{}`), Prelude: []*ChatEnvelope{{Id: "$admit", Data: []byte(`{}`)}}}
	attached, err = alice.AttachEnvelope(ctx, &chat_rpc.AttachEnvelopeRequest{MessageKey: other.GetMessageKey(), Envelope: late})
	if err != nil {
		t.Fatal(err)
	}
	if got := attached.GetEnvelope(); got.GetId() != late.GetId() || !bytes.Equal(got.GetData(), late.GetRedactedData()) || len(got.GetRedactedData()) != 0 || len(got.GetPrelude()) != 1 {
		t.Fatalf("redacted attachment: %v", got)
	}
}

// TestDetachedEnvelope checks that a detached envelope, alone or with earlier
// history, is stored without content, even in an encrypted channel, and that one
// carrying content is refused.
func TestDetachedEnvelope(t *testing.T) {
	// Start a World testbed.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create an encrypted channel and its writer.
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/detached"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Detached", Timestamp: timestamppb.Now(), EncryptionAlgorithm: "m.megolm.v1.aes-sha2"}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	alice := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(alice.Close)

	// A detached envelope with content is refused.
	envelope := &ChatEnvelope{Id: "$auth", Data: []byte(`{"type":"m.room.power_levels"}`), Detached: true}
	text := &ChatMessageContent{Content: &ChatMessageContent_Text{Text: "hi"}}
	if _, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: text, Envelope: envelope}); err == nil {
		t.Fatal("detached envelope with content accepted")
	}

	// One without content is stored without content.
	sent, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Envelope: envelope})
	if err != nil {
		t.Fatal(err)
	}
	read, err := alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	if read.GetMessage().GetContent() != nil || !read.GetMessage().GetEnvelope().EqualVT(envelope) {
		t.Fatalf("detached message read back as %v", read.GetMessage())
	}

	// A detached batch of earlier history keeps its prelude and state.
	batch := &ChatEnvelope{
		Id:       "$topic",
		Data:     []byte(`{"type":"m.room.topic"}`),
		Detached: true,
		Prelude:  []*ChatEnvelope{{Id: "$rules", Data: []byte(`{"type":"m.room.join_rules"}`)}},
		StateIds: []string{"$auth"},
	}
	sent, err = alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Envelope: batch})
	if err != nil {
		t.Fatal(err)
	}
	read, err = alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	if read.GetMessage().GetContent() != nil || !read.GetMessage().GetEnvelope().EqualVT(batch) {
		t.Fatalf("detached batch read back as %v", read.GetMessage())
	}
}

// TestEnvelopeEncryptionPolicy checks that an encrypted channel refuses a local
// plaintext body but keeps one that arrives in an envelope.
func TestEnvelopeEncryptionPolicy(t *testing.T) {
	// Start a World testbed.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create an encrypted channel and its writer.
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/envelope-policy"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Policy", Timestamp: timestamppb.Now(), EncryptionAlgorithm: "m.megolm.v1.aes-sha2"}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	alice := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(alice.Close)

	// A local plaintext event is refused.
	event := &ChatMessageContent{Content: &ChatMessageContent_Event{Event: &ChatEvent{Type: "m.room.message", ContentJson: `{"body":"hi","msgtype":"m.text"}`}}}
	if _, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{TransactionId: "local", Content: event}); err == nil {
		t.Fatal("local plaintext accepted in an encrypted channel")
	}

	// The same body in an envelope is kept.
	envelope := &ChatEnvelope{Id: "$plain", Data: []byte(`{"type":"m.room.message"}`)}
	sent, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{Content: event, Envelope: envelope})
	if err != nil {
		t.Fatal(err)
	}
	read, err := alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	if !read.GetMessage().GetContent().EqualVT(event) {
		t.Fatalf("enveloped message read back as %v", read.GetMessage())
	}
}
