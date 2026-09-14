package spacewave_chat

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
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
	if info.GetMessageCount() != 0 {
		t.Fatalf("channel message count = %d, want 0", info.GetMessageCount())
	}
	if info.GetCreatedAt() == nil {
		t.Fatal("channel creation timestamp is nil")
	}
	createdAt := info.GetCreatedAt().CloneVT()

	sendResp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "hello goscript chat"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sendResp.GetMessageKey() == "" {
		t.Fatal("SendMessage returned empty message key")
	}
	updatedInfo, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo after send: %v", err)
	}
	if updatedInfo.GetMessageCount() != 1 {
		t.Fatalf("channel message count = %d, want 1", updatedInfo.GetMessageCount())
	}
	if !updatedInfo.GetCreatedAt().EqualVT(createdAt) {
		t.Fatal("channel creation timestamp changed after sending a message")
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

func TestChatResourceGetMessageValidatesChannelKey(t *testing.T) {
	// Create one accepted event in the mounted channel.
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	sendResp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "accepted"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Read the accepted event through the channel-scoped lookup.
	messageResp, err := resource.GetMessage(ctx, &spacewave_chat_rpc.GetMessageRequest{MessageKey: sendResp.GetMessageKey()})
	if err != nil {
		t.Fatalf("GetMessage accepted: %v", err)
	}
	if messageResp.GetMessage().GetObjectKey() != sendResp.GetMessageKey() {
		t.Fatalf("GetMessage key = %q, want %q", messageResp.GetMessage().GetObjectKey(), sendResp.GetMessageKey())
	}

	// Preserve the missing-object result and reject a foreign-channel key.
	_, err = resource.GetMessage(ctx, &spacewave_chat_rpc.GetMessageRequest{MessageKey: GeneralChannelKey + "/message/missing"})
	if !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("GetMessage missing error = %v, want %v", err, world.ErrObjectNotFound)
	}
	_, err = resource.GetMessage(ctx, &spacewave_chat_rpc.GetMessageRequest{MessageKey: "other/message/0"})
	if err == nil {
		t.Fatal("GetMessage accepted a foreign-channel key")
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
			GeneralChannelKey+"/message/"+strconv.Itoa(idx),
			"message-"+strconv.Itoa(idx),
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

func TestChatResourceListMessagesSupportsIndexCursors(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	for idx := range 70 {
		createChatMessage(
			t,
			ctx,
			ws,
			GeneralChannelKey,
			GeneralChannelKey+"/message/"+strconv.Itoa(idx),
			"message-"+strconv.Itoa(idx),
			"peer-local",
		)
	}

	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	checkPage := func(name string, req *spacewave_chat_rpc.ListMessagesRequest, first, last uint64, wantHasMore bool) {
		t.Helper()
		resp, err := resource.ListMessages(ctx, req)
		if err != nil {
			t.Fatalf("%s: ListMessages: %v", name, err)
		}
		messages := resp.GetMessages()
		if len(messages) != int(last-first+1) {
			t.Fatalf("%s: returned %d messages, want %d", name, len(messages), last-first+1)
		}
		for idx, message := range messages {
			if want := first + uint64(idx); message.GetIndex() != want {
				t.Fatalf("%s: message %d has index %d, want %d", name, idx, message.GetIndex(), want)
			}
		}
		if resp.GetHasMore() != wantHasMore {
			t.Fatalf("%s: hasMore = %t, want %t", name, resp.GetHasMore(), wantHasMore)
		}
	}
	checkEmpty := func(name string, req *spacewave_chat_rpc.ListMessagesRequest) {
		t.Helper()
		resp, err := resource.ListMessages(ctx, req)
		if err != nil {
			t.Fatalf("%s: ListMessages: %v", name, err)
		}
		if len(resp.GetMessages()) != 0 || resp.GetHasMore() {
			t.Fatalf("%s: returned %d messages with hasMore=%t, want empty terminal page", name, len(resp.GetMessages()), resp.GetHasMore())
		}
	}
	checkPage("from 0", &spacewave_chat_rpc.ListMessagesRequest{FromIndex: new(uint64(0)), Limit: 20}, 0, 19, true)
	checkPage("from 20", &spacewave_chat_rpc.ListMessagesRequest{FromIndex: new(uint64(20)), Limit: 20}, 20, 39, true)
	checkPage("from 60", &spacewave_chat_rpc.ListMessagesRequest{FromIndex: new(uint64(60)), Limit: 20}, 60, 69, false)
	checkEmpty("from terminal", &spacewave_chat_rpc.ListMessagesRequest{FromIndex: new(uint64(70)), Limit: 20})
	checkPage("before 70", &spacewave_chat_rpc.ListMessagesRequest{BeforeIndex: new(uint64(70)), Limit: 20}, 50, 69, true)
	checkPage("before 50", &spacewave_chat_rpc.ListMessagesRequest{BeforeIndex: new(uint64(50)), Limit: 20}, 30, 49, true)
	checkPage("before 20", &spacewave_chat_rpc.ListMessagesRequest{BeforeIndex: new(uint64(20)), Limit: 20}, 0, 19, false)
	checkEmpty("before terminal", &spacewave_chat_rpc.ListMessagesRequest{BeforeIndex: new(uint64(0)), Limit: 20})

	conflictRequests := map[string]*spacewave_chat_rpc.ListMessagesRequest{
		"index directions": {
			BeforeIndex: new(uint64(20)),
			FromIndex:   new(uint64(20)),
		},
		"before index and key": {
			BeforeKey:   GeneralChannelKey + "/message/20",
			BeforeIndex: new(uint64(20)),
		},
		"from index and key": {
			BeforeKey: GeneralChannelKey + "/message/20",
			FromIndex: new(uint64(20)),
		},
		"before index above count": {BeforeIndex: new(uint64(71))},
		"from index above count":   {FromIndex: new(uint64(71))},
	}
	for name, req := range conflictRequests {
		if _, err := resource.ListMessages(ctx, req); err == nil {
			t.Errorf("%s: ListMessages succeeded, want error", name)
		}
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

func TestChatResourceReportsChannelCreator(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Initialize a real World so the operation writes through engine-backed state.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()

	// Apply channel creation with the testbed peer as the authenticated sender.
	_, _, err = ws.ApplyWorldOp(ctx, &CreateChatChannelOp{
		ObjectKey: GeneralChannelKey,
		Name:      "General",
		Timestamp: timestamppb.Now(),
	}, sender)
	if err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	// Verify the resource projects the persisted creator identity.
	resource := NewChatResource(ws, wtb.Engine, GeneralChannelKey, "peer-local")
	info, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.GetCreatorPeerId() != sender.String() {
		t.Fatalf("channel creator peer ID = %q, want %q", info.GetCreatorPeerId(), sender.String())
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

func createChatChannel(t *testing.T, ctx context.Context, ws world.WorldState, objectKey, name string) {
	t.Helper()

	createdObject, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatChannel{Name: name, CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, ChatChannelTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", objectKey, err)
	}
}

func createChatMessage(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey, text, sender string) {
	t.Helper()

	msgIndex, ok := parseMessageIndex(msgKey)
	if !ok {
		t.Fatalf("message key %q does not end with an index", msgKey)
	}
	createdObject, _, err := world.CreateWorldObject(ctx, ws, msgKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&ChatMessage{
			SenderPeerId: sender,
			Content:      &ChatMessageContent{Content: &ChatMessageContent_Text{Text: text}},
			CreatedAt:    timestamppb.Now(),
			Index:        msgIndex,
		}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
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

	msgIndex, ok := parseMessageIndex(msgKey)
	if !ok {
		t.Fatalf("message key %q does not end with an index", msgKey)
	}
	channelObj, found, err := ws.GetObject(ctx, channelKey)
	defer world.ReleaseObjectState(channelObj)
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
	pageKey := channelKey + "/message-page/" + strconv.FormatUint(msgIndex/chatMessagePageSize, 10)
	pageObj, found, err := ws.GetObject(ctx, pageKey)
	defer world.ReleaseObjectState(pageObj)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", pageKey, err)
	}
	if !found {
		pageObj, err = ws.CreateObject(ctx, pageKey, nil)
		defer world.ReleaseObjectState(pageObj)
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
