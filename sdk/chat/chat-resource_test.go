package spacewave_chat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

type chatMessageStream struct {
	ctx  context.Context
	sent chan *spacewave_chat_rpc.WatchMessagesResponse
}

func newChatMessageStream(ctx context.Context) *chatMessageStream {
	return &chatMessageStream{
		ctx:  ctx,
		sent: make(chan *spacewave_chat_rpc.WatchMessagesResponse, 8),
	}
}

func (s *chatMessageStream) Context() context.Context {
	return s.ctx
}

func (s *chatMessageStream) MsgSend(srpc.Message) error {
	panic("MsgSend should not be called")
}

func (s *chatMessageStream) MsgRecv(srpc.Message) error {
	panic("MsgRecv should not be called")
}

func (s *chatMessageStream) CloseSend() error {
	return nil
}

func (s *chatMessageStream) Close() error {
	return nil
}

func (s *chatMessageStream) Send(resp *spacewave_chat_rpc.WatchMessagesResponse) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- resp.CloneVT():
		return nil
	}
}

func (s *chatMessageStream) SendAndClose(resp *spacewave_chat_rpc.WatchMessagesResponse) error {
	if resp != nil {
		if err := s.Send(resp); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

func TestChatResourceSendsListsAndWatchesMessages(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	info, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.GetName() != "General" {
		t.Fatalf("channel name = %q, want General", info.GetName())
	}

	sendResp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "hello goscript chat"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sendResp.GetMessageKey() == "" {
		t.Fatal("SendMessage returned empty message key")
	}

	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	requireChatMessages(t, listResp.GetMessages(), sendResp.GetMessageKey(), "hello goscript chat", "peer-local")

	if err := world_types.CheckObjectType(ctx, ws, sendResp.GetMessageKey(), ChatMessageTypeID); err != nil {
		t.Fatalf("message object type: %v", err)
	}
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(GeneralChannelKey, PredChannelMessage.String(), sendResp.GetMessageKey(), ""),
		1,
	)
	if err != nil {
		t.Fatalf("LookupGraphQuads(channel message): %v", err)
	}
	if len(quads) != 1 {
		t.Fatalf("channel message graph quads = %d, want 1", len(quads))
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newChatMessageStream(watchCtx)
	done := make(chan error, 1)
	go func() {
		done <- resource.WatchMessages(&spacewave_chat_rpc.WatchMessagesRequest{}, stream)
	}()

	watchResp := recvChatWatchValue(t, stream.sent)
	requireChatMessages(t, watchResp.GetMessages(), sendResp.GetMessageKey(), "hello goscript chat", "peer-local")

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("WatchMessages returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WatchMessages to stop")
	}
}

func TestChatResourceWatchMessagesSettlesEmptyChannel(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	watchCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	stream := newChatMessageStream(watchCtx)
	done := make(chan error, 1)
	go func() {
		done <- resource.WatchMessages(&spacewave_chat_rpc.WatchMessagesRequest{}, stream)
	}()

	watchResp := recvChatWatchValue(t, stream.sent)
	if len(watchResp.GetMessages()) != 0 {
		t.Fatalf("initial watch response has %d messages, want empty snapshot", len(watchResp.GetMessages()))
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("WatchMessages returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WatchMessages to stop")
	}
}

func TestChatResourceListMessagesBeforeKeyUsesSortedMessageSet(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	createChatMessage(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/0", "first", "peer-local")
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
	if !listResp.GetHasMore() {
		t.Fatal("ListMessages hasMore = false, want true")
	}
	requireChatMessages(t, listResp.GetMessages(), GeneralChannelKey+"/message/1", "second", "peer-local")
}

func TestChatResourceListMessagesClampsLimitAcrossPages(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	for idx := range 70 {
		createChatMessage(
			t,
			ctx,
			ws,
			GeneralChannelKey,
			fmt.Sprintf("%s/message/%d", GeneralChannelKey, idx),
			fmt.Sprintf("message-%02d", idx),
			"peer-local",
		)
	}

	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{Limit: 1000})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	messages := listResp.GetMessages()
	if len(messages) != maxMessageListLimit {
		t.Fatalf("ListMessages returned %d messages, want %d", len(messages), maxMessageListLimit)
	}
	if !listResp.GetHasMore() {
		t.Fatal("ListMessages hasMore = false, want true")
	}
	if first := messages[0]; first.GetObjectKey() != GeneralChannelKey+"/message/20" || first.GetText() != "message-20" {
		t.Fatalf("first clamped message = %q %q, want message 20", first.GetObjectKey(), first.GetText())
	}
	if last := messages[len(messages)-1]; last.GetObjectKey() != GeneralChannelKey+"/message/69" || last.GetText() != "message-69" {
		t.Fatalf("last clamped message = %q %q, want message 69", last.GetObjectKey(), last.GetText())
	}
}

func TestChatResourceAllowsAnonymousConstructionAndRead(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")

	resource := NewChatResource(ws, nil, GeneralChannelKey, "")
	if resource == nil {
		t.Fatal("NewChatResource returned nil")
	}
	info, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.GetName() != "General" {
		t.Fatalf("channel name = %q, want General", info.GetName())
	}
	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(listResp.GetMessages()) != 0 {
		t.Fatalf("ListMessages returned %d messages, want 0", len(listResp.GetMessages()))
	}
}

func TestChatResourceRejectsAnonymousSender(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "")
	_, err = resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "anonymous"})
	if err != ErrChatAuthorIdentityRequired {
		t.Fatalf("SendMessage error = %v, want %v", err, ErrChatAuthorIdentityRequired)
	}
}

func TestChatResourceSendMessageClientMessageIdDeduplicates(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	firstResource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	req := &spacewave_chat_rpc.SendMessageRequest{
		Text:            "hello dedupe",
		ClientMessageId: "m1",
	}
	first, err := firstResource.SendMessage(ctx, req)
	if err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}

	// Simulate a sender restart: a fresh resource over the same world state
	// replays the same client message id and must not duplicate.
	secondResource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	second, err := secondResource.SendMessage(ctx, req)
	if err != nil {
		t.Fatalf("restarted SendMessage: %v", err)
	}
	if first.GetMessageKey() != second.GetMessageKey() {
		t.Fatalf(
			"same client_message_id returned keys %q and %q, want equal",
			first.GetMessageKey(),
			second.GetMessageKey(),
		)
	}

	listResp, err := firstResource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := len(listResp.GetMessages()); got != 1 {
		t.Fatalf("channel holds %d messages after duplicate-id sends, want 1", got)
	}
	requireChatMessages(t, listResp.GetMessages(), first.GetMessageKey(), "hello dedupe", "peer-local")
}

func TestChatResourceSendMessageClientMessageIdRawWireCompat(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	// Encode the client_message_id field as raw protobuf bytes: tag byte
	// 0x1a is field 3 << 3 | wire type 2, then varint length, then value.
	base, err := (&spacewave_chat_rpc.SendMessageRequest{Text: "wire compat"}).MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	const clientMessageID = "m1"
	future := append(append(append([]byte{}, base...), 0x1a), byte(len(clientMessageID)))
	future = append(future, clientMessageID...)

	var req spacewave_chat_rpc.SendMessageRequest
	if err := req.UnmarshalVT(future); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if req.GetClientMessageId() != clientMessageID {
		t.Fatalf("decoded client_message_id = %q, want %q", req.GetClientMessageId(), clientMessageID)
	}

	first, err := resource.SendMessage(ctx, &req)
	if err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	second, err := resource.SendMessage(ctx, &req)
	if err != nil {
		t.Fatalf("second SendMessage: %v", err)
	}
	if first.GetMessageKey() != second.GetMessageKey() {
		t.Fatalf("raw-encoded retry returned keys %q and %q, want equal", first.GetMessageKey(), second.GetMessageKey())
	}
	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := len(listResp.GetMessages()); got != 1 {
		t.Fatalf("channel holds %d messages after raw-encoded duplicate sends, want 1", got)
	}
}

func TestChatResourceSendMessageClientMessageIdConflict(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	req := &spacewave_chat_rpc.SendMessageRequest{Text: "original", ClientMessageId: "m1"}
	first, err := resource.SendMessage(ctx, req)
	if err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}

	conflictReq := &spacewave_chat_rpc.SendMessageRequest{Text: "different text", ClientMessageId: "m1"}
	if _, err := resource.SendMessage(ctx, conflictReq); !errors.Is(err, ErrChatClientMessageConflict) {
		t.Fatalf("conflicting SendMessage error = %v, want %v", err, ErrChatClientMessageConflict)
	}
	replyConflict := &spacewave_chat_rpc.SendMessageRequest{
		Text:            "original",
		ClientMessageId: "m1",
		ReplyToKey:      GeneralChannelKey + "/message/0",
	}
	if _, err := resource.SendMessage(ctx, replyConflict); !errors.Is(err, ErrChatClientMessageConflict) {
		t.Fatalf("reply-target change SendMessage error = %v, want %v", err, ErrChatClientMessageConflict)
	}

	// Same id and payload still returns the original key after conflicts.
	retry, err := resource.SendMessage(ctx, req)
	if err != nil {
		t.Fatalf("retry SendMessage: %v", err)
	}
	if retry.GetMessageKey() != first.GetMessageKey() {
		t.Fatalf("retry key %q != original key %q", retry.GetMessageKey(), first.GetMessageKey())
	}

	// Legacy path: no client message id keeps plain append behavior.
	anon1, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "legacy one"})
	if err != nil {
		t.Fatalf("legacy SendMessage: %v", err)
	}
	anon2, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "legacy one"})
	if err != nil {
		t.Fatalf("legacy SendMessage: %v", err)
	}
	if anon1.GetMessageKey() == anon2.GetMessageKey() {
		t.Fatal("legacy sends without client_message_id returned the same key")
	}

	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := len(listResp.GetMessages()); got != 3 {
		t.Fatalf("channel holds %d messages, want 3", got)
	}
}

func TestChatResourceConcurrentSameClientMessageIdSends(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	const senders = 8
	keys := make([]string, senders)
	errs := make([]error, senders)
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
				Text:            "concurrent",
				ClientMessageId: "shared",
			})
			keys[i], errs[i] = resp.GetMessageKey(), err
		}(i)
	}
	wg.Wait()

	unique := make(map[string]bool)
	for i := 0; i < senders; i++ {
		if errs[i] != nil {
			t.Fatalf("send %d: %v", i, errs[i])
		}
		unique[keys[i]] = true
	}
	if len(unique) != 1 {
		t.Fatalf("concurrent same-id sends produced %d distinct keys, want 1", len(unique))
	}

	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := len(listResp.GetMessages()); got != 1 {
		t.Fatalf("channel holds %d messages after concurrent duplicate sends, want 1", got)
	}
}

func TestChatResourceSendMessageReceiptIdentityCollision(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	// Seed a receipt addressed by this channel's hash for the request id but
	// owned by another sender identity.
	receiptKey := ReceiptObjectKey(GeneralChannelKey, "peer-other", "m1")
	receiptObj, _, err := world.CreateWorldObject(ctx, ws, receiptKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatMessageReceipt{
			SenderPeerId:    "peer-other",
			ClientMessageId: "m1",
			MessageKey:      GeneralChannelKey + "/message/9",
		}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(receipt): %v", err)
	}
	_ = receiptObj
	if err := world_types.SetObjectType(ctx, ws, receiptKey, ChatMessageReceiptTypeID); err != nil {
		t.Fatalf("SetObjectType(receipt): %v", err)
	}

	if _, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:            "hello",
		ClientMessageId: "m1",
	}); !errors.Is(err, ErrChatReceiptIdentityCollision) {
		t.Fatalf("SendMessage error = %v, want %v", err, ErrChatReceiptIdentityCollision)
	}
}

func TestChatResourceListMessagesIgnoresNonMessageBeforeKey(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	createChatMessage(t, ctx, ws, GeneralChannelKey, GeneralChannelKey+"/message/0", "only", "peer-local")

	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	beforeKeys := []string{
		// Receipt keys are numeric-suffixed hashes outside the message prefix.
		GeneralChannelKey + "/receipt/" + strings.Repeat("ab", 32) + "12",
		// A well-formed index under a different channel never parses here.
		"chat/channel/other/message/5",
	}
	for _, beforeKey := range beforeKeys {
		listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{
			BeforeKey: beforeKey,
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("ListMessages(before_key=%q): %v", beforeKey, err)
		}
		if got := len(listResp.GetMessages()); got != 1 {
			t.Fatalf(
				"ListMessages(before_key=%q) returned %d messages, want 1",
				beforeKey,
				got,
			)
		}
		if listResp.GetHasMore() {
			t.Fatalf("ListMessages(before_key=%q) hasMore = true, want false", beforeKey)
		}
	}
}

func createUntypedWorldObject(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) {
	t.Helper()
	_, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatChannel{Name: "untyped"}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
}

func TestChatResourceSendMessageRejectsMissingAndUntypedLinks(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	untypedKey := GeneralChannelKey + "/untyped"
	createUntypedWorldObject(t, ctx, ws, untypedKey)
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	missing := &spacewave_chat_rpc.SendMessageRequest{
		Text:             "links missing target",
		ClientMessageId:  "missing",
		LinkedObjectKeys: []string{GeneralChannelKey + "/does-not-exist"},
	}
	if _, err := resource.SendMessage(ctx, missing); !errors.Is(err, ErrChatLinkedObjectNotFound) {
		t.Fatalf("missing-link SendMessage error = %v, want %v", err, ErrChatLinkedObjectNotFound)
	}

	untyped := &spacewave_chat_rpc.SendMessageRequest{
		Text:             "links untyped target",
		ClientMessageId:  "untyped",
		LinkedObjectKeys: []string{untypedKey},
	}
	if _, err := resource.SendMessage(ctx, untyped); !errors.Is(err, ErrChatLinkedObjectTypeRequired) {
		t.Fatalf("untyped-link SendMessage error = %v, want %v", err, ErrChatLinkedObjectTypeRequired)
	}

	// Rejections leave no state behind: no message, no receipt.
	obj := mustObjectState(t, ctx, ws, GeneralChannelKey)
	var count uint64
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		channel, err := block.UnmarshalBlock[*ChatChannel](ctx, bcs, NewChatChannelBlock)
		if err != nil {
			return err
		}
		count = channel.GetMessageCount()
		return nil
	})
	if err != nil {
		t.Fatalf("read channel: %v", err)
	}
	if count != 0 {
		t.Fatalf("message count = %d after rejected sends, want 0", count)
	}
	receiptKey := ReceiptObjectKey(GeneralChannelKey, "peer-local", "missing")
	if _, found, err := ws.GetObject(ctx, receiptKey); err != nil || found {
		t.Fatalf("rejected send created receipt at %q (found=%v err=%v)", receiptKey, found, err)
	}
}

func mustObjectState(t *testing.T, ctx context.Context, ws world.WorldState, objectKey string) world.ObjectState {
	t.Helper()
	obj, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", objectKey, err)
	}
	if !found {
		t.Fatalf("GetObject(%s): not found", objectKey)
	}
	return obj
}

func TestChatResourceSendMessageLinkedObjectRoundTrip(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	targetKey := GeneralChannelKey + "/target"
	createChatChannel(t, ctx, ws, targetKey, "target")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")

	// Duplicated keys canonicalize to one stored link.
	resp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:             "linked",
		LinkedObjectKeys: []string{targetKey, targetKey},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := len(listResp.GetMessages()); got != 1 {
		t.Fatalf("ListMessages returned %d messages, want 1", got)
	}
	gotLinks := listResp.GetMessages()[0].GetLinkedObjectKeys()
	if len(gotLinks) != 1 || gotLinks[0] != targetKey {
		t.Fatalf("readback links = %v, want [%s]", gotLinks, targetKey)
	}

	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(resp.GetMessageKey(), PredMessageLink.String(), targetKey, ""),
		8,
	)
	if err != nil {
		t.Fatalf("LookupGraphQuads(message link): %v", err)
	}
	if len(quads) != 1 {
		t.Fatalf("message link graph quads = %d, want 1", len(quads))
	}

	// More than maxLinkedObjectKeys unique links rejects the send.
	tooMany := make([]string, 0, maxLinkedObjectKeys+1)
	for i := 0; i <= maxLinkedObjectKeys; i++ {
		extra := GeneralChannelKey + "/target-" + strconv.Itoa(i)
		createChatChannel(t, ctx, ws, extra, "target")
		tooMany = append(tooMany, extra)
	}
	if _, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:             "too many links",
		LinkedObjectKeys: tooMany,
	}); err == nil {
		t.Fatal("oversized link list accepted, want rejection")
	}
}

func createChatChannel(t *testing.T, ctx context.Context, ws world.WorldState, objectKey, name string) {
	t.Helper()

	_, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatChannel{Name: name, CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, ChatChannelTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func createChatMessage(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey, text, sender string) {
	t.Helper()

	msgIndex, ok := parseMessageIndex(channelKey, msgKey)
	if !ok {
		t.Fatalf("message key %q does not parse as a %q message index", msgKey, channelKey)
	}
	_, _, err := world.CreateWorldObject(ctx, ws, msgKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatMessage{
			SenderPeerId: sender,
			Content:      &ChatMessageContent{Content: &ChatMessageContent_Text{Text: text}},
			CreatedAt:    timestamppb.Now(),
			Index:        msgIndex,
		}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", msgKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, msgKey, ChatMessageTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", msgKey, err)
	}
	appendChatMessageKey(t, ctx, ws, channelKey, msgKey)
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(channelKey, PredChannelMessage.String(), msgKey, "")); err != nil {
		t.Fatalf("SetGraphQuad(%s): %v", msgKey, err)
	}
}

func appendChatMessageKey(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey string) {
	t.Helper()

	msgIndex, ok := parseMessageIndex(channelKey, msgKey)
	if !ok {
		t.Fatalf("message key %q does not parse as a %q message index", msgKey, channelKey)
	}
	channelObj, found, err := ws.GetObject(ctx, channelKey)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", channelKey, err)
	}
	if !found {
		t.Fatalf("GetObject(%s): not found", channelKey)
	}
	_, _, err = world.AccessObjectState(ctx, channelObj, true, func(bcs *block.Cursor) error {
		channel, err := block.UnmarshalBlock[*ChatChannel](ctx, bcs, NewChatChannelBlock)
		if err != nil {
			return err
		}
		if channel.GetMessageCount() <= msgIndex {
			channel.MessageCount = msgIndex + 1
		}
		bcs.SetBlock(channel, true)
		return nil
	})
	if err != nil {
		t.Fatalf("increment channel message count(%s): %v", msgKey, err)
	}
	pageKey := fmt.Sprintf("%s/message-page/%d", channelKey, msgIndex/chatMessagePageSize)
	pageObj, found, err := ws.GetObject(ctx, pageKey)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", pageKey, err)
	}
	if !found {
		pageObj, err = ws.CreateObject(ctx, pageKey, nil)
		if err != nil {
			t.Fatalf("CreateObject(%s): %v", pageKey, err)
		}
	}
	_, _, err = world.AccessObjectState(ctx, pageObj, true, func(bcs *block.Cursor) error {
		page, err := block.UnmarshalBlock[*ChatMessagePage](ctx, bcs, NewChatMessagePageBlock)
		if err != nil {
			return err
		}
		if page == nil {
			page = &ChatMessagePage{}
		}
		page.MessageKeys = append(page.MessageKeys, msgKey)
		bcs.SetBlock(page, true)
		return nil
	})
	if err != nil {
		t.Fatalf("append page message key(%s): %v", msgKey, err)
	}
}

func recvChatWatchValue(t *testing.T, ch <-chan *spacewave_chat_rpc.WatchMessagesResponse) *spacewave_chat_rpc.WatchMessagesResponse {
	t.Helper()

	select {
	case val, ok := <-ch:
		if !ok {
			t.Fatal("watch stream channel closed")
		}
		return val
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chat watch response")
	}
	return nil
}

func requireChatMessages(
	t *testing.T,
	messages []*spacewave_chat_rpc.ChatMessageInfo,
	wantKey string,
	wantText string,
	wantSender string,
) {
	t.Helper()

	for _, msg := range messages {
		if msg.GetObjectKey() == wantKey && msg.GetText() == wantText {
			if msg.GetSenderPeerId() != wantSender {
				t.Fatalf("sender = %q, want %q", msg.GetSenderPeerId(), wantSender)
			}
			return
		}
	}
	t.Fatalf("missing message key %q text %q in %#v", wantKey, wantText, messages)
}
