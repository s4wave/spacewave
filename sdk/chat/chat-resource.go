package spacewave_chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

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
	// defaultMessageListLimit bounds each history response.
	defaultMessageListLimit = 50
	// maxMessageListLimit caps caller-selected page sizes.
	maxMessageListLimit = 50
	// chatMessagePageSize bounds each persisted history page.
	chatMessagePageSize = 64
)

// ErrChatAuthorIdentityRequired is returned when a message has no authenticated author.
var ErrChatAuthorIdentityRequired = errors.New("chat author identity required")

// ChatResource serves ChatResourceService for a single chat channel object.
type ChatResource struct {
	// ws serves reads within the mounted Space.
	ws world.WorldState
	// engine serializes channel mutations.
	engine world.Engine
	// objectKey identifies the channel.
	objectKey string
	// localPeerID is the authenticated author bound at Resource construction.
	localPeerID string
	// mux exposes channel operations for this attachment.
	mux srpc.Mux
}

// NewChatResource constructs a ChatResource.
func NewChatResource(
	ws world.WorldState,
	engine world.Engine,
	objectKey string,
	localPeerID string,
) *ChatResource {
	// Bind all operations to the mounted channel and authenticated author.
	r := &ChatResource{
		ws:          ws,
		engine:      engine,
		objectKey:   objectKey,
		localPeerID: localPeerID,
	}

	// Expose the generated service through the Resource lifecycle.
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
	// Read the channel metadata from its shared state.
	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}

	// Project the metadata for the client.
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
	// Bound each response independently of the retained history size.
	limit := req.GetLimit()
	if limit == 0 || limit > maxMessageListLimit {
		limit = defaultMessageListLimit
	}

	// Resolve the cursor's stored position within this channel.
	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}
	beforeIndex := channel.GetMessageCount()
	if beforeKey := req.GetBeforeKey(); beforeKey != "" {
		suffix, matches := strings.CutPrefix(beforeKey, r.objectKey+"/message/")
		if !matches || suffix == "" || strings.Contains(suffix, "/") {
			return nil, errors.New("message cursor belongs to another channel")
		}
		message, err := r.readMessage(ctx, beforeKey)
		if err != nil {
			return nil, err
		}
		if message == nil {
			return nil, world.ErrObjectNotFound
		}
		if message.GetIndex() < beforeIndex {
			beforeIndex = message.GetIndex()
		}
	}

	// Load only the selected interval and report earlier retained history.
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
	// Track the next unsent channel position for this attachment.
	ctx := strm.Context()
	nextIndex := uint64(0)
	initialized := false

	// Snapshot the World sequence before reading to avoid missing a concurrent append.
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

		// Emit complete bounded history batches in acceptance order.
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

		// Resume only when the shared World changes or the client releases its stream.
		if _, err := r.ws.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

// SendMessage atomically appends a message or resolves an identical sender-scoped retry.
func (r *ChatResource) SendMessage(
	ctx context.Context,
	req *spacewave_chat_rpc.SendMessageRequest,
) (*spacewave_chat_rpc.SendMessageResponse, error) {
	// Require authenticated write authority before examining a send identity.
	if r.engine == nil {
		return nil, errors.New("chat resource is read-only")
	}
	if r.localPeerID == "" {
		return nil, ErrChatAuthorIdentityRequired
	}

	// Serialize retry resolution and message creation through the World transaction.
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()
	if _, err := world.LookupObjectBody[*ChatChannel](ctx, wtx, r.objectKey, NewChatChannelBlock); err != nil {
		return nil, err
	}

	// Normalize the request content before resolving a send identity.
	content, err := normalizeSendMessageContent(req)
	if err != nil {
		return nil, err
	}

	// Resolve the sender-scoped retry before reserving a history position.
	msgKey := ""
	if transactionID := req.GetTransactionId(); transactionID != "" {
		digest := sha256.Sum256([]byte(strconv.Itoa(len(r.localPeerID)) + ":" + r.localPeerID + transactionID))
		msgKey = r.objectKey + "/message/tx-" + hex.EncodeToString(digest[:])
		prior, err := world.LookupObjectBody[*ChatMessage](ctx, wtx, msgKey, NewChatMessageBlock)
		if err != nil && !errors.Is(err, world.ErrObjectNotFound) {
			return nil, err
		}
		if prior != nil {
			if prior.GetSenderPeerId() != r.localPeerID || !prior.GetContent().EqualVT(content) || prior.GetReplyToKey() != req.GetReplyToKey() {
				return nil, errors.New("chat send transaction conflicts with its accepted message")
			}
			return &spacewave_chat_rpc.SendMessageResponse{MessageKey: msgKey}, nil
		}
	}

	// Append the accepted message and its page entry in one World transaction.
	index, msgKey, err := r.appendChannelMessageKey(ctx, wtx, msgKey)
	if err != nil {
		return nil, err
	}
	msg := &ChatMessage{
		SenderPeerId: r.localPeerID,
		Content:      content,
		CreatedAt:    timestamppb.Now(),
		ReplyToKey:   req.GetReplyToKey(),
		Index:        index,
	}
	obj, err := wtx.CreateObject(ctx, msgKey, nil)
	if err != nil {
		return nil, err
	}
	defer world.ReleaseObjectState(obj)
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(msg, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, wtx, msgKey, ChatMessageTypeID); err != nil {
		return nil, err
	}
	if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(r.objectKey, PredChannelMessage.String(), msgKey, "")); err != nil {
		return nil, err
	}

	// Link an existing peer object without manufacturing identity records.
	peerKey := "peer/" + r.localPeerID
	peerObject, found, err := wtx.GetObject(ctx, peerKey)
	world.ReleaseObjectState(peerObject)
	if err != nil {
		return nil, err
	}
	if found {
		if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(msgKey, PredMessageSender.String(), peerKey, "")); err != nil {
			return nil, err
		}
	}

	// Publish the message and its history position together.
	if err := wtx.Commit(ctx); err != nil {
		return nil, err
	}
	return &spacewave_chat_rpc.SendMessageResponse{MessageKey: msgKey}, nil
}

// appendChannelMessageKey reserves one channel position and its stable message key.
func (r *ChatResource) appendChannelMessageKey(ctx context.Context, ws world.WorldState, msgKey string) (uint64, string, error) {
	// Acquire the mutable channel inside the caller's transaction.
	obj, found, err := ws.GetObject(ctx, r.objectKey)
	if err != nil {
		return 0, "", err
	}
	if !found {
		return 0, "", world.ErrObjectNotFound
	}
	defer world.ReleaseObjectState(obj)

	// Allocate the next position, retaining a caller's stable send identity.
	var index uint64
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		channel, err := block.UnmarshalBlock[*ChatChannel](ctx, bcs, NewChatChannelBlock)
		if err != nil {
			return err
		}
		if channel == nil {
			return world.ErrObjectNotFound
		}
		index = channel.GetMessageCount()
		if msgKey == "" {
			msgKey = r.messageKey(index)
		}
		channel.MessageCount = index + 1
		bcs.SetBlock(channel, true)
		return nil
	})
	if err != nil {
		return 0, "", err
	}

	// Add the accepted key to its bounded page in the same transaction.
	if err := r.appendMessagePageKey(ctx, ws, index, msgKey); err != nil {
		return 0, "", err
	}
	return index, msgKey, nil
}

// appendMessagePageKey appends one key to its bounded channel page.
func (r *ChatResource) appendMessagePageKey(ctx context.Context, ws world.WorldState, index uint64, msgKey string) error {
	// Open or create the page containing this accepted position.
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
	defer world.ReleaseObjectState(obj)

	// Append within the fixed-size page selected by the channel position.
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

// readChannel reads the current channel without retaining an object handle.
func (r *ChatResource) readChannel(ctx context.Context) (*ChatChannel, error) {
	if r.ws == nil {
		return nil, errors.New("chat resource requires world state")
	}
	return world.LookupObjectBody[*ChatChannel](ctx, r.ws, r.objectKey, NewChatChannelBlock)
}

// readMessages resolves only the selected history page.
func (r *ChatResource) readMessages(ctx context.Context, keys []string) ([]*spacewave_chat_rpc.ChatMessageInfo, error) {
	// Require the mounted state before resolving message references.
	if r.ws == nil {
		return nil, errors.New("chat resource requires world state")
	}

	// Preserve the page's order while projecting retained messages.
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

// readMessageKeys resolves a half-open interval from bounded channel pages.
func (r *ChatResource) readMessageKeys(ctx context.Context, startIndex, endIndex uint64) ([]string, error) {
	// Avoid touching storage for an empty interval.
	if startIndex >= endIndex {
		return nil, nil
	}

	// Visit only pages intersecting the requested history interval.
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

// readMessagePage reads one bounded history page.
func (r *ChatResource) readMessagePage(ctx context.Context, pageIndex uint64) (*ChatMessagePage, error) {
	page, err := world.LookupObjectBody[*ChatMessagePage](ctx, r.ws, r.messagePageKey(pageIndex), NewChatMessagePageBlock)
	if errors.Is(err, world.ErrObjectNotFound) {
		return &ChatMessagePage{}, nil
	}
	return page, err
}

// readMessage projects an accepted message without retaining an object handle.
func (r *ChatResource) readMessage(ctx context.Context, key string) (*spacewave_chat_rpc.ChatMessageInfo, error) {
	// Read the stored message, including its stable history position.
	msg, err := world.LookupObjectBody[*ChatMessage](ctx, r.ws, key, NewChatMessageBlock)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Return the shared client projection.
	return &spacewave_chat_rpc.ChatMessageInfo{
		ObjectKey:    key,
		SenderPeerId: msg.GetSenderPeerId(),
		Text:         msg.GetContent().GetText(),
		Content:      msg.GetContent().CloneVT(),
		CreatedAt:    msg.GetCreatedAt(),
		ReplyToKey:   msg.GetReplyToKey(),
		Index:        msg.GetIndex(),
	}, nil
}

// normalizeSendMessageContent validates one shared content value without decrypting it.
func normalizeSendMessageContent(req *spacewave_chat_rpc.SendMessageRequest) (*ChatMessageContent, error) {
	// Keep the existing text request wire shape while accepting typed content.
	content := req.GetContent()
	if content == nil {
		return &ChatMessageContent{Content: &ChatMessageContent_Text{Text: req.GetText()}}, nil
	}
	if req.GetText() != "" {
		return nil, errors.New("chat send cannot include both text and content")
	}

	// Require an explicit variant and a complete encrypted envelope.
	switch value := content.GetContent().(type) {
	case *ChatMessageContent_Text:
		if value == nil {
			return nil, errors.New("chat text content is missing")
		}
	case *ChatMessageContent_Ciphertext:
		if value == nil || value.Ciphertext.GetAlgorithm() == "" || value.Ciphertext.GetCiphertext() == "" || value.Ciphertext.GetSenderKey() == "" || value.Ciphertext.GetSessionId() == "" {
			return nil, errors.New("chat encrypted content is incomplete")
		}
	default:
		return nil, errors.New("chat message content is missing")
	}
	return content.CloneVT(), nil
}

// messageKey identifies a message sent without a transaction identity.
func (r *ChatResource) messageKey(index uint64) string {
	return r.objectKey + "/message/" + strconv.FormatUint(index, 10)
}

// messagePageKey identifies a bounded page owned by this channel.
func (r *ChatResource) messagePageKey(pageIndex uint64) string {
	return r.objectKey + "/message-page/" + strconv.FormatUint(pageIndex, 10)
}

// _ is a type assertion
var _ spacewave_chat_rpc.SRPCChatResourceServiceServer = (*ChatResource)(nil)
