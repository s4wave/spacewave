package spacewave_chat_channel

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
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
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-local")

	info, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.GetName() != "General" {
		t.Fatalf("channel name = %q, want General", info.GetName())
	}

	sendResp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:            "hello goscript chat",
		ClientMessageId: "cmid-1",
	})
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

	if err := world_types.CheckObjectType(ctx, ws, sendResp.GetMessageKey(), spacewave_chat.ChatMessageTypeID); err != nil {
		t.Fatalf("message object type: %v", err)
	}
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(spacewave_chat.GeneralChannelKey, spacewave_chat.PredChannelMessage.String(), sendResp.GetMessageKey(), ""),
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
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-local")

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
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	createChatMessage(t, ctx, ws, spacewave_chat.GeneralChannelKey, spacewave_chat.GeneralChannelKey+"/message/0", "first", "peer-local")
	createChatMessage(t, ctx, ws, spacewave_chat.GeneralChannelKey, spacewave_chat.GeneralChannelKey+"/message/1", "second", "peer-local")
	createChatMessage(t, ctx, ws, spacewave_chat.GeneralChannelKey, spacewave_chat.GeneralChannelKey+"/message/2", "third", "peer-local")

	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-local")
	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{
		BeforeKey: spacewave_chat.GeneralChannelKey + "/message/2",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if !listResp.GetHasMore() {
		t.Fatal("ListMessages hasMore = false, want true")
	}
	requireChatMessages(t, listResp.GetMessages(), spacewave_chat.GeneralChannelKey+"/message/1", "second", "peer-local")
}

func TestChatResourceListMessagesClampsLimitAcrossPages(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-local")
	for idx := range 70 {
		createChatMessage(
			t,
			ctx,
			ws,
			spacewave_chat.GeneralChannelKey,
			fmt.Sprintf("%s/message/%d", spacewave_chat.GeneralChannelKey, idx),
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
	if first := messages[0]; first.GetObjectKey() != spacewave_chat.GeneralChannelKey+"/message/20" || first.GetText() != "message-20" {
		t.Fatalf("first clamped message = %q %q, want message 20", first.GetObjectKey(), first.GetText())
	}
	if last := messages[len(messages)-1]; last.GetObjectKey() != spacewave_chat.GeneralChannelKey+"/message/69" || last.GetText() != "message-69" {
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
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	resource := NewChatResource(ws, nil, spacewave_chat.GeneralChannelKey, "")
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
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "")
	_, err = resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "anonymous", ClientMessageId: "cmid-anon"})
	if err != ErrChatAuthorIdentityRequired {
		t.Fatalf("SendMessage error = %v, want %v", err, ErrChatAuthorIdentityRequired)
	}
}

func createChatChannel(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	name string,
	memberPeerIds ...string,
) {
	t.Helper()

	_, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&spacewave_chat.ChatChannel{Name: name, CreatedAt: timestamppb.Now(), MemberPeerIds: memberPeerIds}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, spacewave_chat.ChatChannelTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

// createChatMessage stores one message block at an arbitrary key and appends
// it to the channel pages, mirroring the canonical append shape.
func createChatMessage(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey, text, sender string) {
	t.Helper()

	msgIndex, ok := parseTestMessageIndex(msgKey)
	if !ok {
		t.Fatalf("message key %q does not end with an index", msgKey)
	}
	_, _, err := world.CreateWorldObject(ctx, ws, msgKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&spacewave_chat.ChatMessage{
			SenderPeerId: sender,
			Content:      &spacewave_chat.ChatMessageContent{Content: &spacewave_chat.ChatMessageContent_Text{Text: text}},
			CreatedAt:    timestamppb.Now(),
			Index:        msgIndex,
		}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", msgKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, msgKey, spacewave_chat.ChatMessageTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", msgKey, err)
	}
	appendChatMessageKey(t, ctx, ws, channelKey, msgKey)
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(channelKey, spacewave_chat.PredChannelMessage.String(), msgKey, "")); err != nil {
		t.Fatalf("SetGraphQuad(%s): %v", msgKey, err)
	}
}

func appendChatMessageKey(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey string) {
	t.Helper()

	msgIndex, ok := parseTestMessageIndex(msgKey)
	if !ok {
		t.Fatalf("message key %q does not end with an index", msgKey)
	}
	channelObj, found, err := ws.GetObject(ctx, channelKey)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", channelKey, err)
	}
	if !found {
		t.Fatalf("GetObject(%s): not found", channelKey)
	}
	_, _, err = world.AccessObjectState(ctx, channelObj, true, func(bcs *block.Cursor) error {
		channel, err := block.UnmarshalBlock[*spacewave_chat.ChatChannel](ctx, bcs, spacewave_chat.NewChatChannelBlock)
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
		page, err := block.UnmarshalBlock[*spacewave_chat.ChatMessagePage](ctx, bcs, spacewave_chat.NewChatMessagePageBlock)
		if err != nil {
			return err
		}
		if page == nil {
			page = &spacewave_chat.ChatMessagePage{}
		}
		page.MessageKeys = append(page.MessageKeys, msgKey)
		bcs.SetBlock(page, true)
		return nil
	})
	if err != nil {
		t.Fatalf("append page message key(%s): %v", msgKey, err)
	}
}

func parseTestMessageIndex(messageKey string) (uint64, bool) {
	idx := len(messageKey) - 1
	for idx >= 0 && messageKey[idx] != '/' {
		idx--
	}
	if idx < 0 || idx == len(messageKey)-1 {
		return 0, false
	}
	messageIndex, err := strconv.ParseUint(messageKey[idx+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return messageIndex, true
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
