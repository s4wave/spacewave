package spacewave_chat

import (
	"context"
	"strconv"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

const (
	defaultMessageListLimit = 50
	maxMessageListLimit     = 50
	chatMessagePageSize     = 64
)

// ErrChatAuthorIdentityRequired is returned when a message has no authenticated author.
var ErrChatAuthorIdentityRequired = errors.New("chat author identity required")

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
	return &spacewave_chat_rpc.GetChannelInfoResponse{
		Name:  channel.GetName(),
		Topic: channel.GetTopic(),
	}, nil
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
	beforeIndex := channel.GetMessageCount()
	if beforeKey := req.GetBeforeKey(); beforeKey != "" {
		idx, ok := parseMessageIndex(beforeKey)
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

// SendMessage creates a message object and links it to the channel.
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

	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	index, msgKey, err := r.appendChannelMessageKey(ctx, wtx)
	if err != nil {
		wtx.Discard()
		return nil, err
	}
	now := time.Now().UTC()
	msg := &ChatMessage{
		SenderPeerId: r.localPeerID,
		Content:      &ChatMessageContent{Content: &ChatMessageContent_Text{Text: req.GetText()}},
		CreatedAt:    timestamppb.New(now),
		ReplyToKey:   req.GetReplyToKey(),
		Index:        index,
	}
	obj, err := wtx.CreateObject(ctx, msgKey, nil)
	if err != nil {
		wtx.Discard()
		return nil, err
	}
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(msg, true)
		return nil
	})
	if err != nil {
		wtx.Discard()
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, wtx, msgKey, ChatMessageTypeID); err != nil {
		wtx.Discard()
		return nil, err
	}
	if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(r.objectKey, PredChannelMessage.String(), msgKey, "")); err != nil {
		wtx.Discard()
		return nil, err
	}
	peerKey := "peer/" + r.localPeerID
	_, found, err := wtx.GetObject(ctx, peerKey)
	if err != nil {
		wtx.Discard()
		return nil, err
	}
	if found {
		if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(msgKey, PredMessageSender.String(), peerKey, "")); err != nil {
			wtx.Discard()
			return nil, err
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		wtx.Discard()
		return nil, err
	}

	return &spacewave_chat_rpc.SendMessageResponse{MessageKey: msgKey}, nil
}

func (r *ChatResource) appendChannelMessageKey(ctx context.Context, ws world.WorldState) (uint64, string, error) {
	obj, found, err := ws.GetObject(ctx, r.objectKey)
	if err != nil {
		return 0, "", err
	}
	if !found {
		return 0, "", world.ErrObjectNotFound
	}

	var index uint64
	var msgKey string
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		channel, err := block.UnmarshalBlock[*ChatChannel](ctx, bcs, NewChatChannelBlock)
		if err != nil {
			return err
		}
		if channel == nil {
			return world.ErrObjectNotFound
		}
		index = channel.GetMessageCount()
		msgKey = r.messageKey(index)
		channel.MessageCount = index + 1
		bcs.SetBlock(channel, true)
		return nil
	})
	if err != nil {
		return 0, "", err
	}
	if err := r.appendMessagePageKey(ctx, ws, index, msgKey); err != nil {
		return 0, "", err
	}
	return index, msgKey, nil
}

func (r *ChatResource) appendMessagePageKey(ctx context.Context, ws world.WorldState, index uint64, msgKey string) error {
	pageKey := r.messagePageKey(index / chatMessagePageSize)
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
	return err
}

func (r *ChatResource) readChannel(ctx context.Context) (*ChatChannel, error) {
	if r.ws == nil {
		return nil, errors.New("chat resource requires world state")
	}
	obj, found, err := r.ws.GetObject(ctx, r.objectKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	var channel *ChatChannel
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		channel, err = block.UnmarshalBlock[*ChatChannel](ctx, bcs, NewChatChannelBlock)
		if err != nil {
			return err
		}
		if channel == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return channel, nil
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

func (r *ChatResource) readMessagePage(ctx context.Context, pageIndex uint64) (*ChatMessagePage, error) {
	obj, found, err := r.ws.GetObject(ctx, r.messagePageKey(pageIndex))
	if err != nil {
		return nil, err
	}
	if !found {
		return &ChatMessagePage{}, nil
	}

	var page *ChatMessagePage
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		page, err = block.UnmarshalBlock[*ChatMessagePage](ctx, bcs, NewChatMessagePageBlock)
		if err != nil {
			return err
		}
		if page == nil {
			page = &ChatMessagePage{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (r *ChatResource) readMessage(
	ctx context.Context,
	key string,
) (*spacewave_chat_rpc.ChatMessageInfo, error) {
	obj, found, err := r.ws.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	var msg *ChatMessage
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		msg, err = block.UnmarshalBlock[*ChatMessage](ctx, bcs, NewChatMessageBlock)
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

	return &spacewave_chat_rpc.ChatMessageInfo{
		ObjectKey:    key,
		SenderPeerId: msg.GetSenderPeerId(),
		Text:         msg.GetContent().GetText(),
		CreatedAt:    msg.GetCreatedAt(),
		ReplyToKey:   msg.GetReplyToKey(),
	}, nil
}

func (r *ChatResource) messageKey(index uint64) string {
	return r.objectKey + "/message/" + strconv.FormatUint(index, 10)
}

func (r *ChatResource) messagePageKey(pageIndex uint64) string {
	return r.objectKey + "/message-page/" + strconv.FormatUint(pageIndex, 10)
}

func parseMessageIndex(messageKey string) (uint64, bool) {
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

var _ spacewave_chat_rpc.SRPCChatResourceServiceServer = (*ChatResource)(nil)
