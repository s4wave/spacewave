package spacewave_chat_channel

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
)

// appendExternalMessage relays one bridged external event into the channel.
// This is the internal exactly-once entry point for a future bridge; no
// public RPC exposes an external origin.
func (r *ChatResource) appendExternalMessage(
	ctx context.Context,
	ref *spacewave_chat.ExternalMessageRef,
	text string,
	replyToKey string,
) (*appendedMessage, error) {
	if r.engine == nil {
		return nil, errors.New("chat resource is read-only")
	}
	if r.localPeerID == "" {
		return nil, ErrChatAuthorIdentityRequired
	}
	origin, err := newExternalOrigin(ref)
	if err != nil {
		return nil, err
	}
	return r.appendInTransaction(ctx, origin, text, replyToKey)
}

// checkExternalChannelBinding rejects events whose surface or channel differs
// from the binding stored on the chat channel.
func checkExternalChannelBinding(
	channel *spacewave_chat.ChatChannel,
	ref *spacewave_chat.ExternalMessageRef,
) error {
	bound := channel.GetExternalRef()
	if bound == nil {
		return nil
	}
	if bound.GetSystem() != ref.GetSystem() || bound.GetChannelId() != ref.GetChannelId() {
		return ErrChatExternalChannelMismatch
	}
	return nil
}

// writeExternalClaims stores the exactly-once receipts for one external
// append: the global two-segment channel claim and the global three-segment
// event claim carrying the full reference including author plus the claimed
// message key. Each claim lives at its deterministic global object key, so
// two distinct ChatChannels cannot bind one external channel and concurrent
// claims resolve through World transaction conflicts instead of partial
// writes.
func (r *ChatResource) writeExternalClaims(
	ctx context.Context,
	wtx world.Tx,
	ref *spacewave_chat.ExternalMessageRef,
	msgKey string,
) error {
	channelClaim := &spacewave_chat.ChatExternalChannelClaim{
		ExternalRef: &spacewave_chat.ExternalChannelRef{
			System:    ref.GetSystem(),
			ChannelId: ref.GetChannelId(),
		},
		ChannelObjectKey: r.objectKey,
	}
	claimKey := externalChannelClaimKey(channelClaim.GetExternalRef())
	obj, found, err := wtx.GetObject(ctx, claimKey)
	if err != nil {
		return err
	}
	if found {
		existing, err := readExternalChannelClaim(ctx, obj)
		if err != nil {
			return err
		}
		if existing.GetChannelObjectKey() != r.objectKey {
			return ErrChatExternalChannelConflict
		}
	} else {
		obj, err = wtx.CreateObject(ctx, claimKey, nil)
		if err != nil {
			return err
		}
		if _, _, err := world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
			bcs.SetBlock(channelClaim, true)
			return nil
		}); err != nil {
			return err
		}
		if err := world_types.SetObjectType(ctx, wtx, claimKey, spacewave_chat.ChatExternalChannelClaimTypeID); err != nil {
			return err
		}
	}

	eventClaim := &spacewave_chat.ChatExternalEventClaim{
		ExternalRef: ref,
		MessageKey:  msgKey,
	}
	eventKey := externalEventClaimKey(ref)
	obj, found, err = wtx.GetObject(ctx, eventKey)
	if err != nil {
		return err
	}
	if found {
		existing, err := readExternalEventClaim(ctx, obj)
		if err != nil {
			return err
		}
		if existing.GetMessageKey() != msgKey {
			return ErrChatExternalEventConflict
		}
		return nil
	}
	obj, err = wtx.CreateObject(ctx, eventKey, nil)
	if err != nil {
		return err
	}
	if _, _, err := world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(eventClaim, true)
		return nil
	}); err != nil {
		return err
	}
	return world_types.SetObjectType(ctx, wtx, eventKey, spacewave_chat.ChatExternalEventClaimTypeID)
}

// readExternalChannelClaim unmarshals one stored channel claim.
func readExternalChannelClaim(
	ctx context.Context,
	obj world.ObjectState,
) (*spacewave_chat.ChatExternalChannelClaim, error) {
	var claim *spacewave_chat.ChatExternalChannelClaim
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		claim, err = block.UnmarshalBlock[*spacewave_chat.ChatExternalChannelClaim](ctx, bcs, spacewave_chat.NewChatExternalChannelClaimBlock)
		if err != nil {
			return err
		}
		if claim == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

// readExternalEventClaim unmarshals one stored event claim.
func readExternalEventClaim(
	ctx context.Context,
	obj world.ObjectState,
) (*spacewave_chat.ChatExternalEventClaim, error) {
	var claim *spacewave_chat.ChatExternalEventClaim
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		claim, err = block.UnmarshalBlock[*spacewave_chat.ChatExternalEventClaim](ctx, bcs, spacewave_chat.NewChatExternalEventClaimBlock)
		if err != nil {
			return err
		}
		if claim == nil {
			return world.ErrObjectNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}
