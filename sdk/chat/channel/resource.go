// Package channel serves the ChatResourceService read and send boundary for
// one chat channel World object. Bridged external events enter through the
// internal claim path; no public RPC accepts an external origin.
package spacewave_chat_channel

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

const (
	defaultMessageListLimit = 50
	maxMessageListLimit     = 50
	chatMessagePageSize     = 64
)

// ErrChatAuthorIdentityRequired is returned when a message has no authenticated author.
var ErrChatAuthorIdentityRequired = errors.New("chat author identity required")

// ErrChatPeerNotMember is returned when a peer sends to or reads from a
// private channel whose member list does not include it.
var ErrChatPeerNotMember = errors.New("chat peer is not a channel member")

// ErrChatOriginConflict is returned when a send identity already exists but
// the retry carries a different author or payload. The original message wins;
// the conflicting attempt fails closed instead of mutating it.
var ErrChatOriginConflict = errors.New("chat send origin conflicts with an earlier message")

// ErrChatExternalChannelMismatch is returned when an external event names a
// surface or channel other than the one bound to this chat channel.
var ErrChatExternalChannelMismatch = errors.New("chat external event does not match the channel binding")

// ErrChatExternalChannelConflict is returned when another ChatChannel object
// already holds the global claim for one external channel.
var ErrChatExternalChannelConflict = errors.New("chat external channel is already bound to another chat channel")

// ErrChatExternalEventConflict is returned when another chat message already
// holds the global claim for one external event identity.
var ErrChatExternalEventConflict = errors.New("chat external event is already claimed by another message")

// ChatResource serves ChatResourceService for a single chat channel object.
type ChatResource struct {
	ws          world.WorldState
	engine      world.Engine
	objectKey   string
	localPeerID string
	mux         srpc.Mux
}

// NewChatResource constructs a ChatResource.
func NewChatResource(
	ws world.WorldState,
	engine world.Engine,
	objectKey string,
	localPeerID string,
) *ChatResource {
	r := &ChatResource{
		ws:          ws,
		engine:      engine,
		objectKey:   objectKey,
		localPeerID: localPeerID,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return spacewave_chat_rpc.SRPCRegisterChatResourceService(mux, r)
	})
	return r
}

// GetMux returns the SRPC mux for this resource.
func (r *ChatResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the chat resource lifecycle.
func (r *ChatResource) Close() {}

// GetChannelInfo reads the channel block and returns metadata.
func (r *ChatResource) GetChannelInfo(
	ctx context.Context,
	_ *spacewave_chat_rpc.GetChannelInfoRequest,
) (*spacewave_chat_rpc.GetChannelInfoResponse, error) {
	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.authorizeChannelRead(channel); err != nil {
		return nil, err
	}
	return &spacewave_chat_rpc.GetChannelInfoResponse{
		Name:  channel.GetName(),
		Topic: channel.GetTopic(),
	}, nil
}

// authorizeChannelRead rejects peers outside a private channel's member list.
// An empty member list keeps the channel open to all peers.
func (r *ChatResource) authorizeChannelRead(channel *spacewave_chat.ChatChannel) error {
	if !spacewave_chat.ChannelAllowsMember(channel, r.localPeerID) {
		return ErrChatPeerNotMember
	}
	return nil
}

// ListMessages returns a page of channel messages.
func (r *ChatResource) ListMessages(
	ctx context.Context,
	req *spacewave_chat_rpc.ListMessagesRequest,
) (*spacewave_chat_rpc.ListMessagesResponse, error) {
	limit := req.GetLimit()
	if limit == 0 || limit > maxMessageListLimit {
		limit = defaultMessageListLimit
	}

	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.authorizeChannelRead(channel); err != nil {
		return nil, err
	}
	beforeIndex := channel.GetMessageCount()
	if beforeKey := req.GetBeforeKey(); beforeKey != "" {
		idx, ok := r.messageIndexByKey(ctx, beforeKey)
		if ok && idx < beforeIndex {
			beforeIndex = idx
		}
	}

	count := min(uint64(limit), beforeIndex)
	startIndex := beforeIndex - count
	keys, err := r.readMessageKeys(ctx, startIndex, beforeIndex)
	if err != nil {
		return nil, err
	}
	messages, err := r.readMessages(ctx, keys)
	if err != nil {
		return nil, err
	}
	hasMore := startIndex != 0
	return &spacewave_chat_rpc.ListMessagesResponse{Messages: messages, HasMore: hasMore}, nil
}

// WatchMessages streams new message batches after each world change.
func (r *ChatResource) WatchMessages(
	_ *spacewave_chat_rpc.WatchMessagesRequest,
	strm spacewave_chat_rpc.SRPCChatResourceService_WatchMessagesStream,
) error {
	ctx := strm.Context()
	nextIndex := uint64(0)
	initialized := false

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		seqno, err := r.ws.GetSeqno(ctx)
		if err != nil {
			return err
		}
		channel, err := r.readChannel(ctx)
		if err != nil {
			return err
		}
		if err := r.authorizeChannelRead(channel); err != nil {
			return err
		}
		messageCount := channel.GetMessageCount()
		if nextIndex > messageCount {
			nextIndex = messageCount
		}
		if !initialized && messageCount == 0 {
			if err := strm.Send(&spacewave_chat_rpc.WatchMessagesResponse{}); err != nil {
				return err
			}
		}
		initialized = true
		for nextIndex < messageCount {
			endIndex := min(nextIndex+defaultMessageListLimit, messageCount)
			keys, err := r.readMessageKeys(ctx, nextIndex, endIndex)
			if err != nil {
				return err
			}
			messages, err := r.readMessages(ctx, keys)
			if err != nil {
				return err
			}
			if len(messages) != 0 {
				if err := strm.Send(&spacewave_chat_rpc.WatchMessagesResponse{Messages: messages}); err != nil {
					return err
				}
			}
			nextIndex = endIndex
			if err := ctx.Err(); err != nil {
				return err
			}
		}

		if _, err := r.ws.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

// readChannel reads the channel block.
func (r *ChatResource) readChannel(ctx context.Context) (*spacewave_chat.ChatChannel, error) {
	if r.ws == nil {
		return nil, errors.New("chat resource requires world state")
	}
	return spacewave_chat.ReadChatChannel(ctx, r.ws, r.objectKey)
}

// messageIndexByKey resolves one pagination cursor to the message index
// stored on the referenced message object.
func (r *ChatResource) messageIndexByKey(ctx context.Context, key string) (uint64, bool) {
	msg, err := r.readMessageBlock(ctx, key)
	if err != nil {
		return 0, false
	}
	return msg.GetIndex(), true
}

func (r *ChatResource) readMessages(ctx context.Context, keys []string) ([]*spacewave_chat_rpc.ChatMessageInfo, error) {
	if r.ws == nil {
		return nil, errors.New("chat resource requires world state")
	}

	messages := make([]*spacewave_chat_rpc.ChatMessageInfo, 0, len(keys))
	for _, key := range keys {
		info, err := r.readMessage(ctx, key)
		if err != nil {
			return nil, err
		}
		if info != nil {
			messages = append(messages, info)
		}
	}
	return messages, nil
}

func (r *ChatResource) readMessageKeys(ctx context.Context, startIndex, endIndex uint64) ([]string, error) {
	if startIndex >= endIndex {
		return nil, nil
	}
	keys := make([]string, 0, int(endIndex-startIndex))
	for pageIndex := startIndex / chatMessagePageSize; pageIndex <= (endIndex-1)/chatMessagePageSize; pageIndex++ {
		page, err := r.readMessagePage(ctx, pageIndex)
		if err != nil {
			return nil, err
		}
		pageStart := pageIndex * chatMessagePageSize
		from := uint64(0)
		if startIndex > pageStart {
			from = startIndex - pageStart
		}
		to := uint64(len(page.GetMessageKeys()))
		if pageEnd := endIndex - pageStart; pageEnd < to {
			to = pageEnd
		}
		if from < to {
			keys = append(keys, page.GetMessageKeys()[from:to]...)
		}
	}
	return keys, nil
}

func (r *ChatResource) readMessagePage(ctx context.Context, pageIndex uint64) (*spacewave_chat.ChatMessagePage, error) {
	obj, found, err := r.ws.GetObject(ctx, messagePageKey(r.objectKey, pageIndex))
	if err != nil {
		return nil, err
	}
	if !found {
		return &spacewave_chat.ChatMessagePage{}, nil
	}

	var page *spacewave_chat.ChatMessagePage
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		page, err = block.UnmarshalBlock[*spacewave_chat.ChatMessagePage](ctx, bcs, spacewave_chat.NewChatMessagePageBlock)
		if err != nil {
			return err
		}
		if page == nil {
			page = &spacewave_chat.ChatMessagePage{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

// readMessageBlock loads the raw ChatMessage stored at one key.
func (r *ChatResource) readMessageBlock(
	ctx context.Context,
	key string,
) (*spacewave_chat.ChatMessage, error) {
	obj, found, err := r.ws.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
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

func (r *ChatResource) readMessage(
	ctx context.Context,
	key string,
) (*spacewave_chat_rpc.ChatMessageInfo, error) {
	msg, err := r.readMessageBlock(ctx, key)
	if err != nil {
		if errors.Is(err, world.ErrObjectNotFound) {
			return nil, nil
		}
		return nil, err
	}

	info := &spacewave_chat_rpc.ChatMessageInfo{
		ObjectKey:    key,
		SenderPeerId: msg.GetSenderPeerId(),
		Text:         msg.GetContent().GetText(),
		CreatedAt:    msg.GetCreatedAt(),
		ReplyToKey:   msg.GetReplyToKey(),
	}
	switch origin := msg.GetOrigin().(type) {
	case *spacewave_chat.ChatMessage_ClientMessageId:
		info.Origin = &spacewave_chat_rpc.ChatMessageInfo_ClientMessageId{
			ClientMessageId: origin.ClientMessageId,
		}
	case *spacewave_chat.ChatMessage_ExternalRef:
		info.Origin = &spacewave_chat_rpc.ChatMessageInfo_ExternalRef{
			ExternalRef: origin.ExternalRef,
		}
	}
	return info, nil
}

// messagePageKey derives the message page object key for one page index.
func messagePageKey(channelKey string, pageIndex uint64) string {
	return channelKey + "/message-page/" + strconv.FormatUint(pageIndex, 10)
}

var _ spacewave_chat_rpc.SRPCChatResourceServiceServer = (*ChatResource)(nil)
