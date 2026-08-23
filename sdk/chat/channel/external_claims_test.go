package spacewave_chat_channel

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// matrixEvent builds one external event reference for tests.
func matrixEvent(eventID, authorID string) *spacewave_chat.ExternalMessageRef {
	return &spacewave_chat.ExternalMessageRef{
		System:    "matrix",
		ChannelId: "!room:example.org",
		EventId:   eventID,
		AuthorId:  authorID,
	}
}

func TestChatResourcePrivateChannelRejectsNonMemberSendsAndReads(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General", "peer-a")

	resourceA := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-a")
	resourceB := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-b")

	if _, err := resourceB.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "intrude", ClientMessageId: "x"}); !errors.Is(err, ErrChatPeerNotMember) {
		t.Fatalf("native send as non-member error = %v, want %v", err, ErrChatPeerNotMember)
	}
	if _, err := resourceB.appendExternalMessage(ctx, matrixEvent("$e1", "@ext"), "bridged", ""); !errors.Is(err, ErrChatPeerNotMember) {
		t.Fatalf("external send as non-member error = %v, want %v", err, ErrChatPeerNotMember)
	}
	if _, err := resourceB.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{}); !errors.Is(err, ErrChatPeerNotMember) {
		t.Fatalf("GetChannelInfo as non-member error = %v, want %v", err, ErrChatPeerNotMember)
	}
	if _, err := resourceB.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{}); !errors.Is(err, ErrChatPeerNotMember) {
		t.Fatalf("ListMessages as non-member error = %v, want %v", err, ErrChatPeerNotMember)
	}
	watchErr := resourceB.WatchMessages(
		&spacewave_chat_rpc.WatchMessagesRequest{},
		&watchStreamStub{ctx: ctx},
	)
	if !errors.Is(watchErr, ErrChatPeerNotMember) {
		t.Fatalf("WatchMessages as non-member error = %v, want %v", watchErr, ErrChatPeerNotMember)
	}
	if _, err := resourceA.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{}); err != nil {
		t.Fatalf("GetChannelInfo as member: %v", err)
	}
}

// watchStreamStub satisfies the watch stream interface without a live RPC
// connection; the non-member rejection happens before any send.
type watchStreamStub struct {
	srpc.Stream
	ctx context.Context
}

func (s *watchStreamStub) Context() context.Context { return s.ctx }

func (s *watchStreamStub) Send(*spacewave_chat_rpc.WatchMessagesResponse) error {
	return errors.New("watch stub must not send")
}

func (s *watchStreamStub) SendAndClose(*spacewave_chat_rpc.WatchMessagesResponse) error {
	return errors.New("watch stub must not close")
}

func TestChatResourceOpenChannelAllowsAnyPeer(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-anyone")
	if _, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{}); err != nil {
		t.Fatalf("GetChannelInfo on open channel: %v", err)
	}
	if _, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{}); err != nil {
		t.Fatalf("ListMessages on open channel: %v", err)
	}
	if resp, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "open", ClientMessageId: "open-1"}); err != nil || resp.GetMessageKey() == "" {
		t.Fatalf("SendMessage on open channel = %+v %v", resp, err)
	}
}

// TestChatResourceExternalReplayReturnsOriginalWithoutAdvancing proves an
// exact external replay returns the original message key and leaves count,
// page, and claim state untouched.
func TestChatResourceExternalReplayReturnsOriginalWithoutAdvancing(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay")

	first, err := resource.appendExternalMessage(ctx, matrixEvent("$e1", "@author"), "bridged body", "")
	if err != nil {
		t.Fatalf("appendExternalMessage(first): %v", err)
	}

	replay, err := resource.appendExternalMessage(ctx, matrixEvent("$e1", "@author"), "bridged body", "")
	if err != nil {
		t.Fatalf("appendExternalMessage(replay): %v", err)
	}
	if replay.messageKey != first.messageKey {
		t.Fatalf("replay key = %q, want original %q", replay.messageKey, first.messageKey)
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel after replay: %v", err)
	}
	if channel.GetMessageCount() != 1 {
		t.Fatalf("message count after replay = %d, want 1", channel.GetMessageCount())
	}
	page, err := resource.readMessagePage(ctx, 0)
	if err != nil {
		t.Fatalf("readMessagePage after replay: %v", err)
	}
	if len(page.GetMessageKeys()) != 1 {
		t.Fatalf("page keys after replay = %d, want 1", len(page.GetMessageKeys()))
	}

	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages after replay: %v", err)
	}
	messages := listResp.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("messages after replay = %d, want 1", len(messages))
	}
	ref := messages[0].GetExternalRef()
	if ref == nil || ref.GetSystem() != "matrix" || ref.GetEventId() != "$e1" || ref.GetAuthorId() != "@author" {
		t.Fatalf("projected origin = %+v, want the matrix $e1 ref with author", messages[0].GetOrigin())
	}

	claimKey := externalEventClaimKey(matrixEvent("$e1", "@author"))
	obj, found, err := ws.GetObject(ctx, claimKey)
	if err != nil || !found {
		t.Fatalf("event claim %s found = %v err = %v, want present", claimKey, found, err)
	}
	var claim *spacewave_chat.ChatExternalEventClaim
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		claim, err = block.UnmarshalBlock[*spacewave_chat.ChatExternalEventClaim](ctx, bcs, spacewave_chat.NewChatExternalEventClaimBlock)
		return err
	})
	if err != nil {
		t.Fatalf("unmarshal claim: %v", err)
	}
	if claim.GetMessageKey() != first.messageKey || claim.GetExternalRef().GetAuthorId() != "@author" {
		t.Fatalf("claim = %+v, want full ref with author claiming %q", claim, first.messageKey)
	}
	if err := world_types.CheckObjectType(ctx, ws, claimKey, spacewave_chat.ChatExternalEventClaimTypeID); err != nil {
		t.Fatalf("claim object type: %v", err)
	}
}

// TestChatResourceExternalReplayConflictFailsClosed proves the same event
// identity replayed with a different author or payload fails closed instead
// of returning the original.
func TestChatResourceExternalReplayConflictFailsClosed(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay")

	first, err := resource.appendExternalMessage(ctx, matrixEvent("$e1", "@author"), "body", "")
	if err != nil {
		t.Fatalf("appendExternalMessage(first): %v", err)
	}

	for name, mutate := range map[string]func(*spacewave_chat.ExternalMessageRef, *string){
		"different author": func(ref *spacewave_chat.ExternalMessageRef, _ *string) { ref.AuthorId = "@someone-else" },
		"different text":   func(_ *spacewave_chat.ExternalMessageRef, text *string) { *text = "changed body" },
	} {
		ref := matrixEvent("$e1", "@author")
		text := "body"
		mutate(ref, &text)
		resp, err := resource.appendExternalMessage(ctx, ref, text, "")
		if !errors.Is(err, ErrChatOriginConflict) {
			t.Fatalf("%s: appendExternalMessage error = %v, want %v", name, err, ErrChatOriginConflict)
		}
		if resp != nil && resp.messageKey == first.messageKey {
			t.Fatalf("%s: conflicting replay returned the original key", name)
		}
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel after conflicts: %v", err)
	}
	if channel.GetMessageCount() != 1 {
		t.Fatalf("message count after conflicts = %d, want 1", channel.GetMessageCount())
	}
}

// TestChatResourceRejectsWrongExternalChannel proves an event naming another
// surface or channel fails closed against the stored binding.
func TestChatResourceRejectsWrongExternalChannel(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay")

	if _, err := resource.appendExternalMessage(ctx, matrixEvent("$e1", "@author"), "bind", ""); err != nil {
		t.Fatalf("appendExternalMessage(bind): %v", err)
	}

	wrong := &spacewave_chat.ExternalMessageRef{
		System:    "slack",
		ChannelId: "!room:example.org",
		EventId:   "$e2",
		AuthorId:  "@author",
	}
	if _, err := resource.appendExternalMessage(ctx, wrong, "wrong surface", ""); !errors.Is(err, ErrChatExternalChannelMismatch) {
		t.Fatalf("wrong surface error = %v, want %v", err, ErrChatExternalChannelMismatch)
	}
	wrongRoom := matrixEvent("$e3", "@author")
	wrongRoom.ChannelId = "!other:example.org"
	if _, err := resource.appendExternalMessage(ctx, wrongRoom, "wrong room", ""); !errors.Is(err, ErrChatExternalChannelMismatch) {
		t.Fatalf("wrong room error = %v, want %v", err, ErrChatExternalChannelMismatch)
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel after wrong channels: %v", err)
	}
	if channel.GetMessageCount() != 1 ||
		channel.GetExternalRef().GetSystem() != "matrix" ||
		channel.GetExternalRef().GetChannelId() != "!room:example.org" {
		t.Fatalf("channel after wrong channels = %+v, want untouched matrix binding", channel)
	}
}

// TestChatResourceConcurrentExternalClaimsCreateExactlyOneClaim proves two
// concurrent resources racing to claim the same external channel resolve to
// one claim object through World transaction conflicts.
func TestChatResourceConcurrentExternalClaimsCreateExactlyOneClaim(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	type sendResult struct {
		key string
		err error
	}
	results := make(chan sendResult, 2)
	for i := range 2 {
		resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay")
		go func(i int) {
			resp, err := resource.appendExternalMessage(
				ctx,
				matrixEvent(fmt.Sprintf("$race-%d", i), "@author"),
				"concurrent bind",
				"",
			)
			key := ""
			if resp != nil {
				key = resp.messageKey
			}
			results <- sendResult{key: key, err: err}
		}(i)
	}
	outcomes := []sendResult{<-results, <-results}
	for _, outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("appendExternalMessage concurrent: %v", outcome.err)
		}
		if outcome.key == "" {
			t.Fatal("appendExternalMessage concurrent returned empty key")
		}
	}

	claimKey := externalChannelClaimKey(&spacewave_chat.ExternalChannelRef{
		System:    "matrix",
		ChannelId: "!room:example.org",
	})
	_, found, err := ws.GetObject(ctx, claimKey)
	if err != nil || !found {
		t.Fatalf("channel claim %s found = %v err = %v, want present", claimKey, found, err)
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel: %v", err)
	}
	if channel.GetMessageCount() != 2 {
		t.Fatalf("message count = %d, want 2", channel.GetMessageCount())
	}
	page, err := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay").readMessagePage(ctx, 0)
	if err != nil {
		t.Fatalf("readMessagePage: %v", err)
	}
	if got := page.GetMessageKeys(); len(got) != 2 {
		t.Fatalf("page keys = %d, want exactly two appends", len(got))
	}
}

// TestChatResourceConcurrentExternalReplaysCreateExactlyOneMessage proves
// concurrent replays of one external event produce one message and one
// claim; the losing transaction retries onto the committed result.
func TestChatResourceConcurrentExternalReplaysCreateExactlyOneMessage(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	type sendResult struct {
		key string
		err error
	}
	results := make(chan sendResult, 2)
	for range 2 {
		resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay")
		go func() {
			resp, err := resource.appendExternalMessage(ctx, matrixEvent("$e-race", "@author"), "same body", "")
			key := ""
			if resp != nil {
				key = resp.messageKey
			}
			results <- sendResult{key: key, err: err}
		}()
	}
	keys := map[string]bool{}
	for range 2 {
		outcome := <-results
		if outcome.err != nil {
			t.Fatalf("appendExternalMessage concurrent replay: %v", outcome.err)
		}
		keys[outcome.key] = true
	}
	if len(keys) != 1 {
		t.Fatalf("concurrent replays produced %d distinct keys, want 1", len(keys))
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel: %v", err)
	}
	if channel.GetMessageCount() != 1 {
		t.Fatalf("message count after concurrent replay = %d, want 1", channel.GetMessageCount())
	}
	page, err := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-relay").readMessagePage(ctx, 0)
	if err != nil {
		t.Fatalf("readMessagePage: %v", err)
	}
	if got := page.GetMessageKeys(); len(got) != 1 {
		t.Fatalf("page keys after concurrent replay = %d, want 1", len(got))
	}
	eventKey := externalEventClaimKey(matrixEvent("$e-race", "@author"))
	if _, found, err := ws.GetObject(ctx, eventKey); err != nil || !found {
		t.Fatalf("event claim %s found = %v err = %v, want present", eventKey, found, err)
	}
}

// TestChatResourceConcurrentDistinctChannelsCannotShareBinding proves the
// global external channel claim is exclusive: two distinct ChatChannel
// objects racing to bind one external channel resolve to exactly one winner;
// the loser fails closed with no message, count, or page change.
func TestChatResourceConcurrentDistinctChannelsCannotShareBinding(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	const channelOne = "chat/channel/one"
	const channelTwo = "chat/channel/two"
	createChatChannel(t, ctx, ws, channelOne, "One", "peer-relay")
	createChatChannel(t, ctx, ws, channelTwo, "Two", "peer-relay")

	type sendResult struct {
		channel string
		key     string
		err     error
	}
	results := make(chan sendResult, 2)
	for _, channelKey := range []string{channelOne, channelTwo} {
		resource := NewChatResource(ws, wtb.Engine, channelKey, "peer-relay")
		go func(channelKey string) {
			resp, err := resource.appendExternalMessage(
				ctx,
				matrixEvent(fmt.Sprintf("$bind-%s", channelKey), "@author"),
				"binding race",
				"",
			)
			key := ""
			if resp != nil {
				key = resp.messageKey
			}
			results <- sendResult{channel: channelKey, key: key, err: err}
		}(channelKey)
	}
	outcomes := []sendResult{<-results, <-results}

	var winner *sendResult
	for i := range outcomes {
		outcome := outcomes[i]
		if outcome.err == nil {
			if winner != nil {
				t.Fatal("both channels bound the same external channel, want one winner")
			}
			outcome := outcome
			winner = &outcome
			continue
		}
		if !errors.Is(outcome.err, ErrChatExternalChannelConflict) {
			t.Fatalf("loser %s error = %v, want %v", outcome.channel, outcome.err, ErrChatExternalChannelConflict)
		}
	}
	if winner == nil {
		t.Fatal("no channel won the binding race")
	}

	// The global claim exists once and names the winning channel only.
	ref := &spacewave_chat.ExternalChannelRef{System: "matrix", ChannelId: "!room:example.org"}
	claimObj, found, err := ws.GetObject(ctx, externalChannelClaimKey(ref))
	if err != nil || !found {
		t.Fatalf("global channel claim found = %v err = %v, want present", found, err)
	}
	var claim *spacewave_chat.ChatExternalChannelClaim
	_, _, err = world.AccessObjectState(ctx, claimObj, false, func(bcs *block.Cursor) error {
		var err error
		claim, err = block.UnmarshalBlock[*spacewave_chat.ChatExternalChannelClaim](ctx, bcs, spacewave_chat.NewChatExternalChannelClaimBlock)
		return err
	})
	if err != nil {
		t.Fatalf("unmarshal channel claim: %v", err)
	}
	if claim.GetChannelObjectKey() != winner.channel {
		t.Fatalf("channel claim bound to %q, want winner %q", claim.GetChannelObjectKey(), winner.channel)
	}

	// The winner holds its message; the loser wrote nothing.
	for _, channelKey := range []string{channelOne, channelTwo} {
		channel, err := spacewave_chat.ReadChatChannel(ctx, ws, channelKey)
		if err != nil {
			t.Fatalf("ReadChatChannel(%s): %v", channelKey, err)
		}
		wantCount := uint64(1)
		if channelKey != winner.channel {
			wantCount = 0
		}
		if channel.GetMessageCount() != wantCount {
			t.Fatalf("channel %s count = %d, want %d", channelKey, channel.GetMessageCount(), wantCount)
		}
		if channel.GetExternalRef() != nil && channelKey != winner.channel {
			t.Fatalf("loser channel %s carries an external binding", channelKey)
		}
	}
	for _, channelKey := range []string{channelOne, channelTwo} {
		eventRef := &spacewave_chat.ExternalMessageRef{
			System:    "matrix",
			ChannelId: "!room:example.org",
			EventId:   fmt.Sprintf("$bind-%s", channelKey),
			AuthorId:  "@author",
		}
		_, found, err := ws.GetObject(ctx, externalEventClaimKey(eventRef))
		if err != nil {
			t.Fatalf("event claim lookup for %s: %v", channelKey, err)
		}
		if channelKey == winner.channel && !found {
			t.Fatalf("winner event claim missing for %s", channelKey)
		}
		if channelKey != winner.channel && found {
			t.Fatalf("loser channel %s left an event claim behind", channelKey)
		}
	}
}

// TestChatAppendTransactionRollbackLeavesNoPartialWrite proves the World
// transaction boundary rolls back every mutation of a failed append attempt:
// channel count, page, message object, and claims stay unwritten.
func TestChatAppendTransactionRollbackLeavesNoPartialWrite(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	engine := wtb.Engine

	injected := errors.New("injected append failure")
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (world.Tx, error) {
			return engine.NewTransaction(ctx, true)
		},
		func(ctx context.Context, wtx world.Tx) error {
			if err := rewriteChannel(ctx, wtx, spacewave_chat.GeneralChannelKey, func(channel *spacewave_chat.ChatChannel) {
				channel.MessageCount = 7
			}); err != nil {
				return err
			}
			msgKey := spacewave_chat.GeneralChannelKey + "/message/partial"
			obj, err := wtx.CreateObject(ctx, msgKey, nil)
			if err != nil {
				return err
			}
			_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
				bcs.SetBlock(&spacewave_chat.ChatMessage{SenderPeerId: "peer-x"}, true)
				return nil
			})
			if err != nil {
				return err
			}
			if err := appendMessagePageKey(ctx, wtx, spacewave_chat.GeneralChannelKey, 7, msgKey); err != nil {
				return err
			}
			if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
				spacewave_chat.GeneralChannelKey,
				spacewave_chat.PredChannelMessage.String(),
				msgKey,
				"",
			)); err != nil {
				return err
			}
			claimKey := externalEventClaimKey(matrixEvent("$rollback", "@author"))
			claimObj, err := wtx.CreateObject(ctx, claimKey, nil)
			if err != nil {
				return err
			}
			if _, _, err := world.AccessObjectState(ctx, claimObj, true, func(bcs *block.Cursor) error {
				bcs.SetBlock(&spacewave_chat.ChatExternalEventClaim{
					ExternalRef: matrixEvent("$rollback", "@author"),
					MessageKey:  msgKey,
				}, true)
				return nil
			}); err != nil {
				return err
			}
			return injected
		},
	)
	if !errors.Is(err, injected) {
		t.Fatalf("RunTransaction error = %v, want %v", err, injected)
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel after rollback: %v", err)
	}
	if channel.GetMessageCount() != 0 {
		t.Fatalf("message count after rollback = %d, want 0", channel.GetMessageCount())
	}
	if _, found, err := ws.GetObject(ctx, spacewave_chat.GeneralChannelKey+"/message/partial"); err != nil || found {
		t.Fatalf("partial message found = %v err = %v, want absent", found, err)
	}
	if _, found, err := ws.GetObject(ctx, spacewave_chat.GeneralChannelKey+"/message-page/0"); err != nil || found {
		t.Fatalf("rolled-back page object found = %v err = %v, want absent", found, err)
	}
	if _, found, err := ws.GetObject(ctx, externalEventClaimKey(matrixEvent("$rollback", "@author"))); err != nil || found {
		t.Fatalf("rolled-back event claim found = %v err = %v, want absent", found, err)
	}
}
