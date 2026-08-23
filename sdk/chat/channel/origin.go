package spacewave_chat_channel

import (
	"encoding/base64"

	"github.com/pkg/errors"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
)

// messageOrigin holds exactly one validated send origin for one message.
// Exactly one arm is set; the zero value and mixed arms are invalid.
type messageOrigin struct {
	clientMessageID string
	externalRef     *spacewave_chat.ExternalMessageRef
}

// newNativeOrigin validates and wraps one native send retry identity.
func newNativeOrigin(clientMessageID string) (messageOrigin, error) {
	if err := spacewave_chat.ValidateClientMessageID(clientMessageID); err != nil {
		return messageOrigin{}, err
	}
	return messageOrigin{clientMessageID: clientMessageID}, nil
}

// newExternalOrigin validates and wraps one bridged external event reference.
func newExternalOrigin(ref *spacewave_chat.ExternalMessageRef) (messageOrigin, error) {
	if err := spacewave_chat.ValidateExternalMessageRef(ref); err != nil {
		return messageOrigin{}, err
	}
	return messageOrigin{externalRef: ref}, nil
}

// validate rejects zero and mixed origins defensively.
func (o messageOrigin) validate() error {
	switch {
	case o.clientMessageID != "" && o.externalRef != nil:
		return errors.New("chat message origin is mixed")
	case o.clientMessageID == "" && o.externalRef == nil:
		return errors.New("chat message origin is missing")
	case o.externalRef != nil:
		return spacewave_chat.ValidateExternalMessageRef(o.externalRef)
	default:
		return spacewave_chat.ValidateClientMessageID(o.clientMessageID)
	}
}

// isExternal reports whether the origin came from a bridged external surface.
func (o messageOrigin) isExternal() bool {
	return o.externalRef != nil
}

// applyTo sets the validated origin variant on one stored message block.
func (o messageOrigin) applyTo(msg *spacewave_chat.ChatMessage) {
	if o.externalRef != nil {
		msg.Origin = &spacewave_chat.ChatMessage_ExternalRef{ExternalRef: o.externalRef}
		return
	}
	msg.Origin = &spacewave_chat.ChatMessage_ClientMessageId{ClientMessageId: o.clientMessageID}
}

// matches reports whether a stored message is the exact replay of this
// origin with the same payload. Any difference fails closed.
func (o messageOrigin) matches(stored *spacewave_chat.ChatMessage, localPeerID, text, replyToKey string) error {
	if o.externalRef != nil {
		ref := stored.GetExternalRef()
		if ref == nil ||
			ref.GetSystem() != o.externalRef.GetSystem() ||
			ref.GetChannelId() != o.externalRef.GetChannelId() ||
			ref.GetEventId() != o.externalRef.GetEventId() ||
			ref.GetAuthorId() != o.externalRef.GetAuthorId() {
			return ErrChatOriginConflict
		}
	} else {
		if stored.GetSenderPeerId() != localPeerID || stored.GetClientMessageId() != o.clientMessageID {
			return ErrChatOriginConflict
		}
	}
	if stored.GetContent().GetText() != text || stored.GetReplyToKey() != replyToKey {
		return ErrChatOriginConflict
	}
	return nil
}

// messageKey derives the deterministic message object key for this origin.
// The key is the exactly-once identity: native sends scope it by sender and
// client message id; external events use the three-field event identity.
// Segments are reversible RawURL base64, so untrusted bytes cannot alias
// distinct identities through separators.
func (o messageOrigin) messageKey(channelKey, senderPeerID string) string {
	prefix := channelKey + "/message/"
	if o.externalRef != nil {
		return prefix + "external/" +
			rawURLSegment(o.externalRef.GetSystem()) + "/" +
			rawURLSegment(o.externalRef.GetChannelId()) + "/" +
			rawURLSegment(o.externalRef.GetEventId())
	}
	return prefix + "native/" +
		rawURLSegment(senderPeerID) + "/" +
		rawURLSegment(o.clientMessageID)
}

// chatExternalClaimRoot namespaces the global external claim objects. Claim
// identity is the encoded reference segments alone; one bound external
// channel or delivered external event has exactly one claim in the World,
// whichever ChatChannel holds it.
const chatExternalClaimRoot = "chat/external"

// externalChannelClaimKey derives the global two-segment external channel
// claim key for one (system, channel_id) identity.
func externalChannelClaimKey(ref *spacewave_chat.ExternalChannelRef) string {
	return chatExternalClaimRoot + "-channel/" +
		rawURLSegment(ref.GetSystem()) + "/" +
		rawURLSegment(ref.GetChannelId())
}

// externalEventClaimKey derives the global three-segment external event
// claim key for one (system, channel_id, event_id) identity.
func externalEventClaimKey(ref *spacewave_chat.ExternalMessageRef) string {
	return chatExternalClaimRoot + "-event/" +
		rawURLSegment(ref.GetSystem()) + "/" +
		rawURLSegment(ref.GetChannelId()) + "/" +
		rawURLSegment(ref.GetEventId())
}

// rawURLSegment encodes one bounded key segment reversibly. RawURL base64
// excludes "/" and "+", so segments cannot alias across separators.
func rawURLSegment(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
