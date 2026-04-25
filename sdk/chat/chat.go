package spacewave_chat

import (
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
)

// ChatChannelTypeID is the type identifier for chat channel objects.
const ChatChannelTypeID = "spacewave-chat/channel"

// ChatMessageTypeID is the type identifier for chat message objects.
const ChatMessageTypeID = "spacewave-chat/message"

// PredChannelMessage is the graph predicate linking a channel to its messages.
var PredChannelMessage = quad.IRI("spacewave-chat/channel-message")

// PredMessageSender is the graph predicate linking a message to its sender.
var PredMessageSender = quad.IRI("spacewave-chat/message-sender")

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

// Validate performs cursory checks on the ChatMessage.
func (m *ChatMessage) Validate() error {
	return nil
}

// _ is a type assertion
var _ block.Block = (*ChatChannel)(nil)

// _ is a type assertion
var _ block.Block = (*ChatMessage)(nil)
