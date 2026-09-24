package spacewave_chat

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	chat_state "github.com/s4wave/spacewave/sdk/chat/state"
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

// ErrChatStateConflict indicates that current state no longer matches a write condition.
var ErrChatStateConflict = errors.New("chat state write condition conflicts with current state")

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
	// personPeerID is the verified person associated with the authenticated device.
	personPeerID string
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
	return NewChatResourceForPerson(ws, engine, objectKey, localPeerID, localPeerID)
}

// NewChatResourceForPerson binds a verified person and signing device to a channel.
// The embedding Session host verifies this association; RPC requests cannot select it.
// World access remains constrained by the supplied authorized state and engine.
func NewChatResourceForPerson(
	ws world.WorldState,
	engine world.Engine,
	objectKey string,
	localPeerID string,
	personPeerID string,
) *ChatResource {
	// Bind all operations to the mounted channel and authenticated author.
	r := &ChatResource{
		ws:           ws,
		engine:       engine,
		objectKey:    objectKey,
		localPeerID:  localPeerID,
		personPeerID: personPeerID,
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
		Name:                channel.GetName(),
		Topic:               channel.GetTopic(),
		MessageCount:        channel.GetMessageCount(),
		CreatedAt:           channel.GetCreatedAt().CloneVT(),
		CreatorPeerId:       channel.GetCreatorPeerId(),
		EncryptionAlgorithm: channel.GetEncryptionAlgorithm(),
	}, nil
}

// GetState returns the latest retained event for each channel state identity.
func (r *ChatResource) GetState(ctx context.Context, _ *spacewave_chat_rpc.GetStateRequest) (*spacewave_chat_rpc.GetStateResponse, error) {
	// Require an existing mounted channel even when it has no retained state.
	if _, err := r.readChannel(ctx); err != nil {
		return nil, err
	}

	// Resolve graph references through the same projection used by history reads.
	edges, err := r.ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(r.objectKey, PredChannelState.String(), "", ""), 0)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(edges))
	for _, edge := range edges {
		key, err := world.GraphValueToKey(edge.GetObj())
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	messages, err := r.readMessages(ctx, keys)
	if err != nil {
		return nil, err
	}

	// State order is independent of graph index ordering and history position.
	slices.SortFunc(messages, func(a, b *spacewave_chat_rpc.ChatMessageInfo) int {
		left, right := a.GetContent().GetStateChange(), b.GetContent().GetStateChange()
		if order := strings.Compare(left.GetType(), right.GetType()); order != 0 {
			return order
		}
		return strings.Compare(left.GetStateKey(), right.GetStateKey())
	})
	return &spacewave_chat_rpc.GetStateResponse{Messages: messages}, nil
}

// GetMessage reads one channel message by its channel-scoped object key.
func (r *ChatResource) GetMessage(
	ctx context.Context,
	req *spacewave_chat_rpc.GetMessageRequest,
) (*spacewave_chat_rpc.GetMessageResponse, error) {
	// Validate the requested key against the mounted channel.
	key := req.GetMessageKey()
	suffix, matches := strings.CutPrefix(key, r.objectKey+"/message/")
	if !matches || suffix == "" || strings.Contains(suffix, "/") {
		return nil, errors.New("message key belongs to another channel")
	}

	// Read the validated message through the owner projection.
	message, err := r.readMessage(ctx, key)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, world.ErrObjectNotFound
	}

	// Return the selected message projection.
	return &spacewave_chat_rpc.GetMessageResponse{Message: message}, nil
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

	// Resolve the requested pagination direction and its channel boundary.
	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}
	messageCount := channel.GetMessageCount()
	beforeKey := req.GetBeforeKey()
	hasBeforeIndex := req != nil && req.BeforeIndex != nil
	hasFromIndex := req != nil && req.FromIndex != nil
	if (hasBeforeIndex && hasFromIndex) ||
		(hasBeforeIndex && beforeKey != "") ||
		(hasFromIndex && beforeKey != "") {
		return nil, errors.New("message cursors are mutually exclusive")
	}

	// Select a half-open interval while preserving ascending message order.
	startIndex := uint64(0)
	endIndex := messageCount
	hasMore := false
	switch {
	case hasFromIndex:
		startIndex = req.GetFromIndex()
		if startIndex > messageCount {
			return nil, errors.New("from index exceeds channel message count")
		}
		count := min(uint64(limit), messageCount-startIndex)
		endIndex = startIndex + count
		hasMore = endIndex < messageCount
	case hasBeforeIndex:
		endIndex = req.GetBeforeIndex()
		if endIndex > messageCount {
			return nil, errors.New("before index exceeds channel message count")
		}
		count := min(uint64(limit), endIndex)
		startIndex = endIndex - count
		hasMore = startIndex != 0
	case beforeKey != "":
		messageResponse, err := r.GetMessage(ctx, &spacewave_chat_rpc.GetMessageRequest{MessageKey: beforeKey})
		if err != nil {
			return nil, err
		}
		endIndex = min(messageResponse.GetMessage().GetIndex(), messageCount)
		count := min(uint64(limit), endIndex)
		startIndex = endIndex - count
		hasMore = startIndex != 0
	default:
		count := min(uint64(limit), messageCount)
		startIndex = messageCount - count
		hasMore = startIndex != 0
	}

	// Load only the selected interval and report directional history.
	keys, err := r.readMessageKeys(ctx, startIndex, endIndex)
	if err != nil {
		return nil, err
	}
	messages, err := r.readMessages(ctx, keys)
	if err != nil {
		return nil, err
	}
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

// SendMessage durably appends a message or resolves an identical sender-scoped retry.
func (r *ChatResource) SendMessage(
	ctx context.Context,
	req *spacewave_chat_rpc.SendMessageRequest,
) (*spacewave_chat_rpc.SendMessageResponse, error) {
	// Advance legacy indexing in bounded work before entering the append transaction.
	if r.engine != nil && r.localPeerID != "" && r.personPeerID != "" {
		if err := r.ensureThreadIndex(ctx); err != nil {
			return nil, err
		}
	}
	response, err := r.commitMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	if _, err := r.engine.Sync(ctx); err != nil {
		return nil, err
	}
	return response, nil
}

// commitMessage releases its transaction before the caller fences storage.
func (r *ChatResource) commitMessage(ctx context.Context, req *spacewave_chat_rpc.SendMessageRequest) (*spacewave_chat_rpc.SendMessageResponse, error) {
	// Require authenticated write authority before examining a send identity.
	if r.engine == nil {
		return nil, errors.New("chat resource is read-only")
	}
	if r.localPeerID == "" || r.personPeerID == "" {
		return nil, ErrChatAuthorIdentityRequired
	}

	// Serialize retry resolution and message creation through the World transaction.
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()
	response, err := r.appendMessage(ctx, wtx, req, timestamppb.Now())
	if err != nil {
		return nil, err
	}
	if err := wtx.Commit(ctx); err != nil {
		return nil, err
	}
	return response, nil
}

// appendMessage records a message inside the caller's World transaction.
// Channel creation uses the same append path and supplies its operation timestamp.
func (r *ChatResource) appendMessage(ctx context.Context, wtx world.WorldState, req *spacewave_chat_rpc.SendMessageRequest, timestamp *timestamppb.Timestamp) (*spacewave_chat_rpc.SendMessageResponse, error) {
	// Read the policy and history extent inside the caller's transaction.
	channel, err := world.LookupObjectBody[*ChatChannel](ctx, wtx, r.objectKey, NewChatChannelBlock)
	if err != nil {
		return nil, err
	}

	// Normalize the request content before resolving a send identity.
	content, err := normalizeSendMessageContent(req)
	if err != nil {
		return nil, err
	}
	if req.ExpectedStateMessageKey != nil && content.GetStateChange() == nil {
		return nil, errors.New("chat state write condition requires a state change")
	}

	// Resolve the sender-scoped retry before reserving a history position.
	msgKey := ""
	if transactionID := req.GetTransactionId(); transactionID != "" {
		msgKey, err = TransactionMessageKey(r.objectKey, r.localPeerID, transactionID)
		if err != nil {
			return nil, err
		}
		prior, err := world.LookupObjectBody[*ChatMessage](ctx, wtx, msgKey, NewChatMessageBlock)
		if err != nil && !errors.Is(err, world.ErrObjectNotFound) {
			return nil, err
		}
		if prior != nil {
			personPeerID := prior.GetPersonPeerId()
			if personPeerID == "" {
				personPeerID = prior.GetSenderPeerId()
			}
			if prior.GetSenderPeerId() != r.localPeerID || personPeerID != r.personPeerID || !req.GetReuseAcceptedTransaction() && (!prior.GetContent().EqualVT(content) || prior.GetReplyToKey() != req.GetReplyToKey()) {
				return nil, errors.New("chat send transaction conflicts with its accepted message")
			}
			return &spacewave_chat_rpc.SendMessageResponse{MessageKey: msgKey}, nil
		}
	}

	// Resolve relationships only for new writes; accepted retries retain their original targets.
	relation := content.GetCiphertext().GetRelation()
	eventRelation := content.GetEvent().GetRelation()
	for _, key := range []string{relation.GetTargetKey(), relation.GetReplyToKey(), eventRelation.GetTargetKey(), eventRelation.GetReplyToKey(), content.GetAnnotation().GetTargetKey()} {
		if key == "" {
			continue
		}
		suffix, found := strings.CutPrefix(key, r.objectKey+"/message/")
		if !found || suffix == "" || strings.ContainsAny(suffix, "/\x00") {
			return nil, errors.New("chat relation target belongs to another channel")
		}
		if _, err := world.LookupObjectBody[*ChatMessage](ctx, wtx, key, NewChatMessageBlock); err != nil {
			return nil, errors.Wrap(err, "resolve chat relation target")
		}
	}

	// New bodies follow current encryption policy; accepted retries retain their original result.
	if algorithm := channel.GetEncryptionAlgorithm(); algorithm != "" && content.GetAnnotation() == nil && content.GetStateChange() == nil && content.GetCiphertext().GetAlgorithm() != algorithm {
		return nil, errors.New("chat message ciphertext algorithm does not match channel")
	}

	// Resolve current state after retry lookup so old retries cannot replace newer state.
	var priorState []world.GraphQuad
	state := content.GetStateChange()
	if state != nil {
		priorState, err = wtx.LookupGraphQuads(ctx, NewChatStateQuad(r.objectKey, "", state.GetType(), state.GetStateKey()), 0)
		if err != nil {
			return nil, err
		}
		if expected := req.ExpectedStateMessageKey; expected != nil {
			currentKey := ""
			if len(priorState) > 1 {
				return nil, errors.New("chat state has multiple current events")
			}
			if len(priorState) == 1 {
				currentKey, err = world.GraphValueToKey(priorState[0].GetObj())
				if err != nil {
					return nil, err
				}
			}
			if currentKey != *expected {
				return nil, ErrChatStateConflict
			}
		}
		for _, edge := range priorState {
			key, err := world.GraphValueToKey(edge.GetObj())
			if err != nil {
				return nil, err
			}
			prior, err := world.LookupObjectBody[*ChatMessage](ctx, wtx, key, NewChatMessageBlock)
			if err != nil {
				return nil, err
			}
			if req.GetTransactionId() == "" && prior.GetContent().EqualVT(content) {
				return &spacewave_chat_rpc.SendMessageResponse{MessageKey: key}, nil
			}
			if state.GetType() == "m.room.create" && state.GetStateKey() == "" {
				return nil, errors.New("chat creation state cannot be replaced")
			}
		}
	}

	// Every new append advances an already initialized history prefix atomically.
	if channel.ThreadIndexedMessageCount == nil ||
		channel.GetThreadIndexedMessageCount() != channel.GetMessageCount() {
		return nil, ErrChatThreadIndexBuilding
	}

	// Append the accepted message and its page entry in one World transaction.
	index, msgKey, err := r.appendChannelMessageKey(ctx, wtx, msgKey, state)
	if err != nil {
		return nil, err
	}
	msg := &ChatMessage{
		SenderPeerId: r.localPeerID,
		PersonPeerId: r.personPeerID,
		Content:      content,
		CreatedAt:    timestamp.CloneVT(),
		ReplyToKey:   req.GetReplyToKey(),
		Index:        index,
	}
	obj, err := wtx.CreateObject(ctx, msgKey, nil)
	defer world.ReleaseObjectState(obj)
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

	// Replace only the current-state edge; earlier messages remain in history.
	if state != nil {
		for _, edge := range priorState {
			if err := wtx.DeleteGraphQuad(ctx, edge); err != nil {
				return nil, err
			}
		}
		if err := wtx.SetGraphQuad(ctx, NewChatStateQuad(r.objectKey, msgKey, state.GetType(), state.GetStateKey())); err != nil {
			return nil, err
		}
	}
	if err := r.indexAppendedMessage(ctx, wtx, msgKey, msg); err != nil {
		return nil, err
	}
	return &spacewave_chat_rpc.SendMessageResponse{MessageKey: msgKey}, nil
}

// GetReadPositions reads shared receipt state without transferring mutable channel state.
func (r *ChatResource) GetReadPositions(ctx context.Context, _ *spacewave_chat_rpc.GetReadPositionsRequest) (*spacewave_chat_rpc.GetReadPositionsResponse, error) {
	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}
	positions := make(map[string]*chat_state.ChatReadPosition, len(channel.GetReadPositions()))
	for person, position := range channel.GetReadPositions() {
		positions[person] = position.CloneVT()
	}
	return &spacewave_chat_rpc.GetReadPositionsResponse{Positions: positions}, nil
}

// UpdateReadPosition advances the authenticated person's position durably across devices.
// Repeated or older positions return the current value without a World mutation.
func (r *ChatResource) UpdateReadPosition(ctx context.Context, req *spacewave_chat_rpc.UpdateReadPositionRequest) (*spacewave_chat_rpc.UpdateReadPositionResponse, error) {
	response, err := r.commitReadPosition(ctx, req)
	if err != nil {
		return nil, err
	}
	if _, err := r.engine.Sync(ctx); err != nil {
		return nil, err
	}
	return response, nil
}

// commitReadPosition serializes receipt advancement and releases the transaction before Sync.
func (r *ChatResource) commitReadPosition(ctx context.Context, req *spacewave_chat_rpc.UpdateReadPositionRequest) (*spacewave_chat_rpc.UpdateReadPositionResponse, error) {
	if r.engine == nil || r.localPeerID == "" || r.personPeerID == "" {
		return nil, ErrChatAuthorIdentityRequired
	}
	tx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	channel, err := world.LookupObjectBody[*ChatChannel](ctx, tx, r.objectKey, NewChatChannelBlock)
	if err != nil {
		return nil, err
	}
	if req.GetNextIndex() > channel.GetMessageCount() {
		return nil, errors.New("read position exceeds channel message count")
	}
	prior := channel.GetReadPositions()[r.personPeerID]
	if req.GetNextIndex() <= prior.GetNextIndex() {
		position := prior.CloneVT()
		if position == nil {
			position = &chat_state.ChatReadPosition{}
		}
		return &spacewave_chat_rpc.UpdateReadPositionResponse{Position: position}, nil
	}
	position := &chat_state.ChatReadPosition{NextIndex: req.GetNextIndex(), UpdatedAt: timestamppb.Now()}
	if channel.ReadPositions == nil {
		channel.ReadPositions = make(map[string]*chat_state.ChatReadPosition)
	}
	channel.ReadPositions[r.personPeerID] = position
	object, found, err := tx.GetObject(ctx, r.objectKey)
	defer world.ReleaseObjectState(object)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}
	if _, _, err := world.AccessObjectState(ctx, object, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(channel, true)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &spacewave_chat_rpc.UpdateReadPositionResponse{Position: position.CloneVT()}, nil
}

// appendChannelMessageKey reserves one channel position and its stable message key.
func (r *ChatResource) appendChannelMessageKey(ctx context.Context, ws world.WorldState, msgKey string, state *ChatStateChange) (uint64, string, error) {
	// Acquire the mutable channel inside the caller's transaction.
	obj, found, err := ws.GetObject(ctx, r.objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return 0, "", err
	}
	if !found {
		return 0, "", world.ErrObjectNotFound
	}

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
		if err := applyChannelState(channel, state); err != nil {
			return err
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
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		obj, err = ws.CreateObject(ctx, pageKey, nil)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if err := world_types.SetObjectType(ctx, ws, pageKey, ChatMessagePageTypeID); err != nil {
			return err
		}
	}

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
	keys := make([]string, 0, int(endIndex-startIndex)) //nolint:gosec // the requested history interval is bounded by the page query contract.
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
	personPeerID := msg.GetPersonPeerId()
	if personPeerID == "" {
		personPeerID = msg.GetSenderPeerId()
	}
	text := msg.GetContent().GetText()
	if msg.GetContent().GetEvent() != nil {
		text = "Unsupported message"
	}
	if state := msg.GetContent().GetStateChange(); state != nil {
		text = "Channel settings updated"
		if state.GetStateKey() == "" {
			switch state.GetType() {
			case "m.room.create":
				text = "Channel created"
			case "m.room.name":
				text = "Channel name updated"
			case "m.room.topic":
				text = "Channel topic updated"
			case "m.room.encryption":
				text = "Channel encryption enabled"
			}
		}
	}
	return &spacewave_chat_rpc.ChatMessageInfo{
		ObjectKey:    key,
		SenderPeerId: msg.GetSenderPeerId(),
		PersonPeerId: personPeerID,
		Text:         text,
		Content:      msg.GetContent().CloneVT(),
		CreatedAt:    msg.GetCreatedAt().CloneVT(),
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
		if err := value.Ciphertext.GetRelation().Validate(); err != nil {
			return nil, err
		}
	case *ChatMessageContent_Annotation:
		if value == nil || value.Annotation.GetTargetKey() == "" || len(value.Annotation.GetTargetKey()) > 4096 || value.Annotation.GetKey() == "" || len(value.Annotation.GetKey()) > 4096 {
			return nil, errors.New("chat annotation requires a bounded target and key")
		}
	case *ChatMessageContent_StateChange:
		if value == nil {
			return nil, errors.New("chat state content is missing")
		}
		if err := value.StateChange.Validate(); err != nil {
			return nil, err
		}
	case *ChatMessageContent_Event:
		if value == nil {
			return nil, errors.New("chat event content is missing")
		}
		if err := value.Event.Validate(); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("chat message content is missing")
	}
	return content.CloneVT(), nil
}

// applyChannelState keeps native channel metadata consistent with retained state.
func applyChannelState(channel *ChatChannel, state *ChatStateChange) error {
	// Named state keys and extension types do not replace channel-wide metadata.
	if state == nil || state.GetStateKey() != "" {
		return nil
	}
	var field string
	switch state.GetType() {
	case "m.room.name":
		field = "name"
	case "m.room.topic":
		field = "topic"
	case "m.room.encryption":
		field = "algorithm"
	default:
		return nil
	}

	// These known values have native meaning and require their schema's string field.
	value, err := fastjson.Parse(state.GetContentJson())
	if err != nil {
		return err
	}
	member := value.Get(field)
	if member == nil || member.Type() != fastjson.TypeString {
		return errors.New("chat state requires a string " + field)
	}
	text := string(member.GetStringBytes())
	switch field {
	case "name":
		channel.Name = text
	case "topic":
		channel.Topic = text
	case "algorithm":
		if text == "" || channel.GetEncryptionAlgorithm() != "" && channel.GetEncryptionAlgorithm() != text {
			return errors.New("chat encryption cannot be disabled or changed")
		}
		channel.EncryptionAlgorithm = text
	}
	return nil
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
