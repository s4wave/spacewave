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

var _ block.Block = (*ChatChannel)(nil)

var _ block.Block = (*ChatMessage)(nil)

var _ block.Block = (*ChatMessagePage)(nil)

var _ block.Block = (*ChatExternalChannelClaim)(nil)

var _ block.Block = (*ChatExternalEventClaim)(nil)
