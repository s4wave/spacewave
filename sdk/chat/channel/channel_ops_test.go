package spacewave_chat_channel

import (
	"errors"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	"github.com/sirupsen/logrus"
)

func TestCreateChatChannelOpCreatesPrivateMembership(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	op := &spacewave_chat.CreateChatChannelOp{
		ObjectKey:     "chat/channel/dm-test",
		Name:          "Direct Messages",
		Timestamp:     timestamppb.Now(),
		MemberPeerIds: []string{"peer-b", "peer-a"},
	}
	if err := op.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := op.ApplyWorldOp(ctx, logrus.NewEntry(logrus.New()), ws, "peer-a"); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, "chat/channel/dm-test")
	if err != nil {
		t.Fatalf("ReadChatChannel: %v", err)
	}
	if len(channel.GetMemberPeerIds()) != 2 {
		t.Fatalf("member peer ids = %v, want both peers", channel.GetMemberPeerIds())
	}

	member := NewChatResource(ws, wtb.Engine, "chat/channel/dm-test", "peer-b")
	resp, err := member.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "hello", ClientMessageId: "m1"})
	if err != nil {
		t.Fatalf("SendMessage as member: %v", err)
	}
	if resp.GetMessageKey() == "" {
		t.Fatalf("SendMessage as member response = %+v, want a message key", resp)
	}

	outsider := NewChatResource(ws, wtb.Engine, "chat/channel/dm-test", "peer-c")
	if _, err := outsider.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "intrude", ClientMessageId: "m2"}); !errors.Is(err, ErrChatPeerNotMember) {
		t.Fatalf("SendMessage as non-member error = %v, want %v", err, ErrChatPeerNotMember)
	}
	if _, err := outsider.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{}); !errors.Is(err, ErrChatPeerNotMember) {
		t.Fatalf("GetChannelInfo as non-member error = %v, want %v", err, ErrChatPeerNotMember)
	}
}

func TestInitChatDemoOpKeepsGeneralChannelOpen(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	op := &spacewave_chat.InitChatDemoOp{
		ChannelObjectKey: spacewave_chat.GeneralChannelKey,
		Timestamp:        timestamppb.Now(),
	}
	if err := op.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := op.ApplyWorldOp(ctx, logrus.NewEntry(logrus.New()), ws, "peer-anyone"); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel: %v", err)
	}
	if len(channel.GetMemberPeerIds()) != 0 {
		t.Fatalf("general channel members = %v, want open membership", channel.GetMemberPeerIds())
	}

	anyone := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-unknown")
	if _, err := anyone.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "open", ClientMessageId: "o1"}); err != nil {
		t.Fatalf("SendMessage on open general channel: %v", err)
	}
	if _, err := anyone.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{}); err != nil {
		t.Fatalf("GetChannelInfo on open general channel: %v", err)
	}
}
