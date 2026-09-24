package spacewave_chat

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

const (
	// maxThreadMigrationMessages bounds legacy backfill work per request.
	maxThreadMigrationMessages = 250
	// maxThreadScanLimit bounds filtering work independently of channel size.
	maxThreadScanLimit = 250
)

// ErrChatThreadIndexBuilding asks a caller to retry after bounded legacy backfill.
var ErrChatThreadIndexBuilding = errors.New("chat thread index is still building")

// ListThreads returns native thread summaries in descending activity order.
func (r *ChatResource) ListThreads(
	ctx context.Context,
	req *spacewave_chat_rpc.ListThreadsRequest,
) (*spacewave_chat_rpc.ListThreadsResponse, error) {
	if err := r.ensureThreadIndex(ctx); err != nil {
		return nil, err
	}
	channel, err := r.readChannel(ctx)
	if err != nil {
		return nil, err
	}

	limit := req.GetLimit()
	if limit == 0 || limit > maxMessageListLimit {
		limit = defaultMessageListLimit
	}
	threadKey := channel.GetThreadHeadKey()
	if req != nil && req.BeforeIndex != nil {
		beforeIndex := req.GetBeforeIndex()
		if beforeIndex >= channel.GetMessageCount() {
			return nil, errors.New("thread cursor exceeds channel history")
		}
		messageKey, err := r.readMessageKeyAt(ctx, r.ws, beforeIndex)
		if err != nil {
			return nil, err
		}
		message, err := world.LookupObjectBody[*ChatMessage](ctx, r.ws, messageKey, NewChatMessageBlock)
		if err != nil {
			return nil, err
		}
		relation := chatMessageRelation(message)
		if relation.GetType() != "m.thread" || relation.GetTargetKey() == "" {
			return nil, errors.New("thread cursor does not identify a thread reply")
		}
		cursorKey, err := r.chatThreadKey(relation.GetTargetKey())
		if err != nil {
			return nil, err
		}
		cursor, err := r.readThread(ctx, r.ws, cursorKey)
		if err != nil {
			return nil, err
		}
		threadKey = cursor.GetOlderThreadKey()
	}

	response := &spacewave_chat_rpc.ListThreadsResponse{}
	var lastScanned *ChatThread
	for scanned := 0; threadKey != "" && scanned < maxThreadScanLimit && len(response.Threads) < int(limit); scanned++ {
		thread, err := r.readThread(ctx, r.ws, threadKey)
		if err != nil {
			return nil, err
		}
		lastScanned = thread
		threadKey = thread.GetOlderThreadKey()

		participated, err := r.threadParticipated(ctx, thread)
		if err != nil {
			return nil, err
		}
		if req.GetParticipatedOnly() && !participated {
			continue
		}
		root, err := r.readMessage(ctx, thread.GetRootMessageKey())
		if err != nil {
			return nil, err
		}
		latest, err := r.readMessage(ctx, thread.GetLatestMessageKey())
		if err != nil {
			return nil, err
		}
		if root == nil || latest == nil {
			return nil, errors.New("thread index references a missing message")
		}
		response.Threads = append(response.Threads, &spacewave_chat_rpc.ChatThreadInfo{
			Root:                    root,
			LatestReply:             latest,
			ReplyCount:              thread.GetReplyCount(),
			CurrentUserParticipated: participated,
		})
	}
	if threadKey != "" && lastScanned != nil {
		next := lastScanned.GetLatestMessageIndex()
		response.NextBeforeIndex = &next
	}
	return response, nil
}

// ensureThreadIndex migrates a legacy channel once before serving indexed reads.
func (r *ChatResource) ensureThreadIndex(ctx context.Context) error {
	channel, err := r.readChannel(ctx)
	if err != nil {
		return err
	}
	if channel.ThreadIndexedMessageCount != nil &&
		channel.GetThreadIndexedMessageCount() == channel.GetMessageCount() {
		return nil
	}
	if r.engine == nil {
		return errors.New("legacy chat thread index requires a writable resource")
	}

	tx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	channel, err = world.LookupObjectBody[*ChatChannel](ctx, tx, r.objectKey, NewChatChannelBlock)
	if err != nil {
		return err
	}
	changed, complete, err := r.updateThreadIndex(ctx, tx, channel, maxThreadMigrationMessages)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := r.writeChannel(ctx, tx, channel); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if _, err := r.engine.Sync(ctx); err != nil {
		return err
	}
	if !complete {
		return ErrChatThreadIndexBuilding
	}
	return nil
}

// updateThreadIndex extends or initializes the index through current history.
func (r *ChatResource) updateThreadIndex(
	ctx context.Context,
	ws world.WorldState,
	channel *ChatChannel,
	messageLimit uint64,
) (bool, bool, error) {
	if messageLimit == 0 {
		return false, false, errors.New("chat thread migration limit must be positive")
	}
	initialized := channel.ThreadIndexedMessageCount != nil
	if !initialized {
		if channel.GetThreadHeadKey() != "" {
			return false, false, errors.New("legacy chat thread index contains current-format state")
		}
		channel.ThreadIndexedMessageCount = new(uint64)
	}
	if channel.GetThreadIndexedMessageCount() > channel.GetMessageCount() {
		return false, false, errors.New("chat thread index exceeds channel history")
	}

	changed := !initialized
	startIndex := channel.GetThreadIndexedMessageCount()
	endIndex := startIndex + min(messageLimit, channel.GetMessageCount()-startIndex)
	for index := startIndex; index < endIndex; index++ {
		messageKey, err := r.readMessageKeyAt(ctx, ws, index)
		if err != nil {
			return false, false, err
		}
		message, err := world.LookupObjectBody[*ChatMessage](ctx, ws, messageKey, NewChatMessageBlock)
		if err != nil {
			return false, false, err
		}
		if err := r.indexThreadReply(ctx, ws, channel, messageKey, message); err != nil {
			return false, false, err
		}
		indexedMessageCount := index + 1
		channel.ThreadIndexedMessageCount = &indexedMessageCount
		changed = true
	}
	return changed, channel.GetThreadIndexedMessageCount() == channel.GetMessageCount(), nil
}

// indexAppendedMessage advances the current index with one newly retained message.
func (r *ChatResource) indexAppendedMessage(ctx context.Context, ws world.WorldState, messageKey string, message *ChatMessage) error {
	channel, err := world.LookupObjectBody[*ChatChannel](ctx, ws, r.objectKey, NewChatChannelBlock)
	if err != nil {
		return err
	}
	if channel.ThreadIndexedMessageCount == nil ||
		channel.GetThreadIndexedMessageCount() != message.GetIndex() ||
		channel.GetMessageCount() != message.GetIndex()+1 {
		return errors.New("chat thread index does not match appended history")
	}
	if err := r.indexThreadReply(ctx, ws, channel, messageKey, message); err != nil {
		return err
	}
	indexedMessageCount := channel.GetMessageCount()
	channel.ThreadIndexedMessageCount = &indexedMessageCount
	return r.writeChannel(ctx, ws, channel)
}

// indexThreadReply updates the activity list and participation set for one reply.
func (r *ChatResource) indexThreadReply(
	ctx context.Context,
	ws world.WorldState,
	channel *ChatChannel,
	messageKey string,
	message *ChatMessage,
) error {
	relation := chatMessageRelation(message)
	if relation.GetType() != "m.thread" || relation.GetTargetKey() == "" {
		return nil
	}
	threadKey, err := r.chatThreadKey(relation.GetTargetKey())
	if err != nil {
		return err
	}
	thread, err := r.readThread(ctx, ws, threadKey)
	if err != nil && !errors.Is(err, world.ErrObjectNotFound) {
		return err
	}
	if thread == nil {
		thread = &ChatThread{
			RootMessageKey:     relation.GetTargetKey(),
			LatestMessageKey:   messageKey,
			LatestMessageIndex: message.GetIndex(),
			ReplyCount:         1,
			OlderThreadKey:     channel.GetThreadHeadKey(),
		}
		if headKey := channel.GetThreadHeadKey(); headKey != "" {
			head, err := r.readThread(ctx, ws, headKey)
			if err != nil {
				return err
			}
			if head.GetNewerThreadKey() != "" {
				return errors.New("chat thread head has a newer link")
			}
			head.NewerThreadKey = threadKey
			if err := r.writeThread(ctx, ws, headKey, head); err != nil {
				return err
			}
		}
		channel.ThreadHeadKey = threadKey
	} else {
		if thread.GetRootMessageKey() != relation.GetTargetKey() || thread.GetLatestMessageIndex() >= message.GetIndex() {
			return errors.New("chat thread index order is inconsistent")
		}
		thread.LatestMessageKey = messageKey
		thread.LatestMessageIndex = message.GetIndex()
		thread.ReplyCount++
		if channel.GetThreadHeadKey() != threadKey {
			newerKey := thread.GetNewerThreadKey()
			if newerKey == "" {
				return errors.New("non-head chat thread has no newer link")
			}
			newer, err := r.readThread(ctx, ws, newerKey)
			if err != nil {
				return err
			}
			if newer.GetOlderThreadKey() != threadKey {
				return errors.New("chat thread newer link is inconsistent")
			}
			newer.OlderThreadKey = thread.GetOlderThreadKey()
			if err := r.writeThread(ctx, ws, newerKey, newer); err != nil {
				return err
			}
			if olderKey := thread.GetOlderThreadKey(); olderKey != "" {
				older, err := r.readThread(ctx, ws, olderKey)
				if err != nil {
					return err
				}
				if older.GetNewerThreadKey() != threadKey {
					return errors.New("chat thread older link is inconsistent")
				}
				older.NewerThreadKey = thread.GetNewerThreadKey()
				if err := r.writeThread(ctx, ws, olderKey, older); err != nil {
					return err
				}
			}
			headKey := channel.GetThreadHeadKey()
			head, err := r.readThread(ctx, ws, headKey)
			if err != nil {
				return err
			}
			if head.GetNewerThreadKey() != "" {
				return errors.New("chat thread head has a newer link")
			}
			head.NewerThreadKey = threadKey
			if err := r.writeThread(ctx, ws, headKey, head); err != nil {
				return err
			}
			thread.NewerThreadKey = ""
			thread.OlderThreadKey = headKey
			channel.ThreadHeadKey = threadKey
		}
	}
	if err := r.writeThread(ctx, ws, threadKey, thread); err != nil {
		return err
	}

	personPeerID := message.GetPersonPeerId()
	if personPeerID == "" {
		personPeerID = message.GetSenderPeerId()
	}
	if personPeerID == "" {
		return errors.New("thread reply has no attributed person")
	}
	return ws.SetGraphQuad(ctx, NewChatThreadParticipantQuad(threadKey, personPeerID))
}

// threadParticipated checks the authenticated person without enumerating participants.
func (r *ChatResource) threadParticipated(ctx context.Context, thread *ChatThread) (bool, error) {
	if r.personPeerID == "" {
		return false, nil
	}
	threadKey, err := r.chatThreadKey(thread.GetRootMessageKey())
	if err != nil {
		return false, err
	}
	quads, err := r.ws.LookupGraphQuads(
		ctx,
		NewChatThreadParticipantQuad(threadKey, r.personPeerID),
		1,
	)
	return len(quads) != 0, err
}

// chatThreadKey derives a bounded channel-owned key from a validated root key.
func (r *ChatResource) chatThreadKey(rootMessageKey string) (string, error) {
	suffix, found := strings.CutPrefix(rootMessageKey, r.objectKey+"/message/")
	if !found || suffix == "" || strings.ContainsAny(suffix, "/\x00") {
		return "", errors.New("thread root belongs to another channel")
	}
	return r.objectKey + "/thread/" + suffix, nil
}

// chatMessageRelation returns public relationship metadata from supported bodies.
func chatMessageRelation(message *ChatMessage) *ChatRelation {
	content := message.GetContent()
	if content.GetEvent() != nil {
		return content.GetEvent().GetRelation()
	}
	if content.GetCiphertext() != nil {
		return content.GetCiphertext().GetRelation()
	}
	return nil
}

// readMessageKeyAt resolves one stable history position through its bounded page.
func (r *ChatResource) readMessageKeyAt(ctx context.Context, ws world.WorldState, index uint64) (string, error) {
	page, err := world.LookupObjectBody[*ChatMessagePage](ctx, ws, r.messagePageKey(index/chatMessagePageSize), NewChatMessagePageBlock)
	if err != nil {
		return "", err
	}
	offset := index % chatMessagePageSize
	if offset >= uint64(len(page.GetMessageKeys())) || page.GetMessageKeys()[offset] == "" {
		return "", errors.New("chat history page is missing an indexed message")
	}
	return page.GetMessageKeys()[offset], nil
}

// readThread loads one bounded thread summary.
func (r *ChatResource) readThread(ctx context.Context, ws world.WorldState, key string) (*ChatThread, error) {
	return world.LookupObjectBody[*ChatThread](ctx, ws, key, NewChatThreadBlock)
}

// writeThread creates or replaces one thread summary inside the caller's transaction.
func (r *ChatResource) writeThread(ctx context.Context, ws world.WorldState, key string, thread *ChatThread) error {
	object, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(object)
	if err != nil {
		return err
	}
	if !found {
		object, err = ws.CreateObject(ctx, key, nil)
		defer world.ReleaseObjectState(object)
		if err != nil {
			return err
		}
		if err := world_types.SetObjectType(ctx, ws, key, ChatThreadTypeID); err != nil {
			return err
		}
	}
	_, _, err = world.AccessObjectState(ctx, object, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(thread, true)
		return nil
	})
	return err
}

// writeChannel replaces channel metadata inside the caller's transaction.
func (r *ChatResource) writeChannel(ctx context.Context, ws world.WorldState, channel *ChatChannel) error {
	object, found, err := ws.GetObject(ctx, r.objectKey)
	defer world.ReleaseObjectState(object)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}
	_, _, err = world.AccessObjectState(ctx, object, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(channel, true)
		return nil
	})
	return err
}
