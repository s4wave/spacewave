package spacewave_chat_channel

import (
	"context"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// appendedMessage is the private result of one canonical transaction append.
type appendedMessage struct {
	messageKey string
}

// SendMessage sends a native message as the authenticated caller and wraps
// the native result into the public response. The request maps only
// ClientMessageId; bridged origins never enter through this RPC.
func (r *ChatResource) SendMessage(
	ctx context.Context,
	req *spacewave_chat_rpc.SendMessageRequest,
) (*spacewave_chat_rpc.SendMessageResponse, error) {
	if r.engine == nil {
		return nil, errors.New("chat resource is read-only")
	}
	if r.localPeerID == "" {
		return nil, ErrChatAuthorIdentityRequired
	}
	origin, err := newNativeOrigin(req.GetClientMessageId())
	if err != nil {
		return nil, err
	}
	result, err := r.appendInTransaction(ctx, origin, req.GetText(), req.GetReplyToKey())
	if err != nil {
		return nil, err
	}
	return &spacewave_chat_rpc.SendMessageResponse{MessageKey: result.messageKey}, nil
}

// appendInTransaction runs one canonical append attempt on a single write
// transaction. Every mutation of the channel, page, message, and claims
// shares that transaction, so a conflict rolls back the whole attempt and a
// retry observes the committed winner.
func (r *ChatResource) appendInTransaction(
	ctx context.Context,
	origin messageOrigin,
	text string,
	replyToKey string,
) (*appendedMessage, error) {
	if err := origin.validate(); err != nil {
		return nil, err
	}
	var result *appendedMessage
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (world.Tx, error) {
			return r.engine.NewTransaction(ctx, true)
		},
		func(ctx context.Context, wtx world.Tx) error {
			appended, err := r.attemptAppend(ctx, wtx, origin, text, replyToKey)
			if err != nil {
				return err
			}
			result = appended
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// attemptAppend performs one replayable append. An exact replay returns the
// original message key before allocating an index, advancing the count, or
// touching a page; a conflicting retry fails closed.
func (r *ChatResource) attemptAppend(
	ctx context.Context,
	wtx world.Tx,
	origin messageOrigin,
	text string,
	replyToKey string,
) (*appendedMessage, error) {
	channel, err := spacewave_chat.ReadChatChannel(ctx, wtx, r.objectKey)
	if err != nil {
		return nil, err
	}
	if !spacewave_chat.ChannelAllowsMember(channel, r.localPeerID) {
		return nil, ErrChatPeerNotMember
	}

	msgKey := origin.messageKey(r.objectKey, r.localPeerID)
	existing, err := readStoredMessage(ctx, wtx, msgKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if err := origin.matches(existing, r.localPeerID, text, replyToKey); err != nil {
			return nil, err
		}
		return &appendedMessage{messageKey: msgKey}, nil
	}

	if origin.isExternal() {
		if err := checkExternalChannelBinding(channel, origin.externalRef); err != nil {
			return nil, err
		}
	}

	index := channel.GetMessageCount()
	now := time.Now().UTC()
	msg := &spacewave_chat.ChatMessage{
		SenderPeerId: r.localPeerID,
		Content:      &spacewave_chat.ChatMessageContent{Content: &spacewave_chat.ChatMessageContent_Text{Text: text}},
		CreatedAt:    timestamppb.New(now),
		ReplyToKey:   replyToKey,
		Index:        index,
	}
	origin.applyTo(msg)
	obj, err := wtx.CreateObject(ctx, msgKey, nil)
	if err != nil {
		return nil, err
	}
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(msg, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, wtx, msgKey, spacewave_chat.ChatMessageTypeID); err != nil {
		return nil, err
	}
	if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(r.objectKey, spacewave_chat.PredChannelMessage.String(), msgKey, "")); err != nil {
		return nil, err
	}
	peerKey := "peer/" + r.localPeerID
	_, found, err := wtx.GetObject(ctx, peerKey)
	if err != nil {
		return nil, err
	}
	if found {
		if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(msgKey, spacewave_chat.PredMessageSender.String(), peerKey, "")); err != nil {
			return nil, err
		}
	}
	if err := appendMessagePageKey(ctx, wtx, r.objectKey, index, msgKey); err != nil {
		return nil, err
	}

	if err := rewriteChannel(ctx, wtx, r.objectKey, func(channel *spacewave_chat.ChatChannel) {
		channel.MessageCount = index + 1
		if origin.isExternal() && channel.GetExternalRef() == nil {
			channel.ExternalRef = &spacewave_chat.ExternalChannelRef{
				System:    origin.externalRef.GetSystem(),
				ChannelId: origin.externalRef.GetChannelId(),
			}
		}
	}); err != nil {
		return nil, err
	}

	if origin.isExternal() {
		if err := r.writeExternalClaims(ctx, wtx, origin.externalRef, msgKey); err != nil {
			return nil, err
		}
	}

	return &appendedMessage{messageKey: msgKey}, nil
}

// rewriteChannel loads the channel object, applies one mutation inside its
// block, and writes it back within the caller's transaction.
func rewriteChannel(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	mutate func(*spacewave_chat.ChatChannel),
) error {
	obj, found, err := ws.GetObject(ctx, objectKey)
	if err != nil || !found {
		if err != nil {
			return err
		}
		return world.ErrObjectNotFound
	}
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		channel, err := block.UnmarshalBlock[*spacewave_chat.ChatChannel](ctx, bcs, spacewave_chat.NewChatChannelBlock)
		if err != nil {
			return err
		}
		if channel == nil {
			return world.ErrObjectNotFound
		}
		mutate(channel)
		bcs.SetBlock(channel, true)
		return nil
	})
	return err
}

// readStoredMessage returns the message stored at key, or nil when absent.
func readStoredMessage(ctx context.Context, ws world.WorldState, key string) (*spacewave_chat.ChatMessage, error) {
	obj, found, err := ws.GetObject(ctx, key)
	if err != nil || !found {
		return nil, err
	}
	var msg *spacewave_chat.ChatMessage
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		msg, err = block.UnmarshalBlock[*spacewave_chat.ChatMessage](ctx, bcs, spacewave_chat.NewChatMessageBlock)
		if err != nil {
			return err
		}
		if msg == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// appendMessagePageKey appends one message key to the page holding index.
func appendMessagePageKey(ctx context.Context, ws world.WorldState, channelKey string, index uint64, msgKey string) error {
	pageKey := messagePageKey(channelKey, index/chatMessagePageSize)
	obj, found, err := ws.GetObject(ctx, pageKey)
	if err != nil {
		return err
	}
	if !found {
		obj, err = ws.CreateObject(ctx, pageKey, nil)
		if err != nil {
			return err
		}
	}

	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
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
	return err
}
