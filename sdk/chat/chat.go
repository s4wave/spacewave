package spacewave_chat

import (
	"context"
	"slices"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// ChatChannelTypeID is the type identifier for chat channel objects.
const ChatChannelTypeID = "spacewave-chat/channel"

// ChatMessageTypeID is the type identifier for chat message objects.
const ChatMessageTypeID = "spacewave-chat/message"

// ChatExternalChannelClaimTypeID is the type identifier for external channel
// claim objects.
const ChatExternalChannelClaimTypeID = "spacewave-chat/external-channel-claim"

// ChatExternalEventClaimTypeID is the type identifier for external event
// claim objects.
const ChatExternalEventClaimTypeID = "spacewave-chat/external-event-claim"

// PredChannelMessage is the graph predicate linking a channel to its messages.
var PredChannelMessage = quad.IRI("spacewave-chat/channel-message")

// PredMessageSender is the graph predicate linking a message to its sender.
var PredMessageSender = quad.IRI("spacewave-chat/message-sender")

// Send origin bounds. These are the smallest defensible limits for one
// message send identity; every claim-key segment is bounded by them and
// encoded reversibly, so no object key can exceed a few hundred bytes.
const (
	// MaxChatClientMessageIDLen bounds one native send retry identity in
	// bytes. It matches the wire contract on SendMessageRequest.
	MaxChatClientMessageIDLen = 256
	// MaxChatExternalSystemLen bounds one external surface name in bytes.
	MaxChatExternalSystemLen = 64
	// MaxChatExternalIDLen bounds one external channel, event, or author id
	// in bytes.
	MaxChatExternalIDLen = 256
)

// NewChatChannelBlock constructs a new ChatChannel block.
func NewChatChannelBlock() block.Block {
	return &ChatChannel{}
}

// NewChatMessageBlock constructs a new ChatMessage block.
func NewChatMessageBlock() block.Block {
	return &ChatMessage{}
}

// NewChatMessagePageBlock constructs a new ChatMessagePage block.
func NewChatMessagePageBlock() block.Block {
	return &ChatMessagePage{}
}

// NewChatExternalChannelClaimBlock constructs a new ChatExternalChannelClaim
// block.
func NewChatExternalChannelClaimBlock() block.Block {
	return &ChatExternalChannelClaim{}
}

// NewChatExternalEventClaimBlock constructs a new ChatExternalEventClaim
// block.
func NewChatExternalEventClaimBlock() block.Block {
	return &ChatExternalEventClaim{}
}

// MarshalBlock marshals the ChatChannel to bytes.
func (c *ChatChannel) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatChannel from bytes.
func (c *ChatChannel) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatChannel.
func (c *ChatChannel) Validate() error {
	if c.GetMemberPeerIds() != nil {
		seen := make(map[string]struct{}, len(c.GetMemberPeerIds()))
		for _, memberID := range c.GetMemberPeerIds() {
			if memberID == "" {
				return errors.New("chat channel member peer id is empty")
			}
			if _, dup := seen[memberID]; dup {
				return errors.Errorf("chat channel member %s is listed more than once", memberID)
			}
			seen[memberID] = struct{}{}
		}
	}
	if ref := c.GetExternalRef(); ref != nil {
		if err := ValidateExternalChannelRef(ref); err != nil {
			return err
		}
	}
	return nil
}

// MarshalBlock marshals the ChatMessage to bytes.
func (m *ChatMessage) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatMessage from bytes.
func (m *ChatMessage) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatMessage. Exactly one send
// origin must be set and bounded.
func (m *ChatMessage) Validate() error {
	switch origin := m.GetOrigin().(type) {
	case nil:
		return errors.New("chat message origin is missing")
	case *ChatMessage_ClientMessageId:
		return ValidateClientMessageID(origin.ClientMessageId)
	case *ChatMessage_ExternalRef:
		return ValidateExternalMessageRef(origin.ExternalRef)
	default:
		return errors.New("chat message origin is invalid")
	}
}

// MarshalBlock marshals the ChatMessagePage to bytes.
func (m *ChatMessagePage) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatMessagePage from bytes.
func (m *ChatMessagePage) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatMessagePage.
func (m *ChatMessagePage) Validate() error {
	if slices.Contains(m.GetMessageKeys(), "") {
		return errors.New("chat message page contains an empty key")
	}
	return nil
}

// MarshalBlock marshals the ChatExternalChannelClaim to bytes.
func (c *ChatExternalChannelClaim) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatExternalChannelClaim from bytes.
func (c *ChatExternalChannelClaim) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatExternalChannelClaim.
func (c *ChatExternalChannelClaim) Validate() error {
	if err := ValidateExternalChannelRef(c.GetExternalRef()); err != nil {
		return err
	}
	if c.GetChannelObjectKey() == "" {
		return errors.New("chat external channel claim has no channel object key")
	}
	return nil
}

// MarshalBlock marshals the ChatExternalEventClaim to bytes.
func (c *ChatExternalEventClaim) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatExternalEventClaim from bytes.
func (c *ChatExternalEventClaim) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatExternalEventClaim.
func (c *ChatExternalEventClaim) Validate() error {
	if err := ValidateExternalMessageRef(c.GetExternalRef()); err != nil {
		return err
	}
	if c.GetMessageKey() == "" {
		return errors.New("chat external event claim has no message key")
	}
	return nil
}

// ValidateClientMessageID checks one native send retry identity. The identity
// must be nonempty printable ASCII within MaxChatClientMessageIDLen bytes.
func ValidateClientMessageID(clientMessageID string) error {
	if clientMessageID == "" {
		return errors.New("chat client message id is empty")
	}
	if len(clientMessageID) > MaxChatClientMessageIDLen {
		return errors.Errorf(
			"chat client message id exceeds %d bytes",
			MaxChatClientMessageIDLen,
		)
	}
	for i := 0; i < len(clientMessageID); i++ {
		if c := clientMessageID[i]; c < 0x20 || c > 0x7e {
			return errors.New("chat client message id must be printable ASCII")
		}
	}
	return nil
}

// ValidateExternalChannelRef checks one bounded external channel identity.
func ValidateExternalChannelRef(ref *ExternalChannelRef) error {
	if ref == nil || ref.GetSystem() == "" || ref.GetChannelId() == "" {
		return errors.New("chat external channel ref is incomplete")
	}
	if len(ref.GetSystem()) > MaxChatExternalSystemLen {
		return errors.Errorf(
			"chat external system exceeds %d bytes",
			MaxChatExternalSystemLen,
		)
	}
	if len(ref.GetChannelId()) > MaxChatExternalIDLen {
		return errors.Errorf(
			"chat external channel id exceeds %d bytes",
			MaxChatExternalIDLen,
		)
	}
	return nil
}

// ValidateExternalMessageRef checks one bounded external event identity
// including its author.
func ValidateExternalMessageRef(ref *ExternalMessageRef) error {
	if err := ValidateExternalChannelRef(&ExternalChannelRef{
		System:    ref.GetSystem(),
		ChannelId: ref.GetChannelId(),
	}); err != nil {
		return err
	}
	if ref.GetEventId() == "" || ref.GetAuthorId() == "" {
		return errors.New("chat external event ref is incomplete")
	}
	if len(ref.GetEventId()) > MaxChatExternalIDLen {
		return errors.Errorf(
			"chat external event id exceeds %d bytes",
			MaxChatExternalIDLen,
		)
	}
	if len(ref.GetAuthorId()) > MaxChatExternalIDLen {
		return errors.Errorf(
			"chat external author id exceeds %d bytes",
			MaxChatExternalIDLen,
		)
	}
	return nil
}

// ReadChatChannel reads the chat channel block from the given world state.
func ReadChatChannel(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) (*ChatChannel, error) {
	obj, found, err := ws.GetObject(ctx, objectKey)
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

// ChannelAllowsMember reports whether peerID holds read and send authority on
// the channel. An empty member list keeps the channel open to all peers.
func ChannelAllowsMember(channel *ChatChannel, peerID string) bool {
	members := channel.GetMemberPeerIds()
	if len(members) == 0 {
		return true
	}
	return slices.Contains(members, peerID)
}

var _ block.Block = (*ChatChannel)(nil)

var _ block.Block = (*ChatMessage)(nil)

var _ block.Block = (*ChatMessagePage)(nil)

var _ block.Block = (*ChatExternalChannelClaim)(nil)

var _ block.Block = (*ChatExternalEventClaim)(nil)
