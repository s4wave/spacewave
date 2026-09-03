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

// PredMessageLink is the graph predicate linking a message to a referenced
// object.
var PredMessageLink = quad.IRI("spacewave-chat/message-link")

// ChatMessageReceiptTypeID is the type identifier for chat message receipt
// objects.
const ChatMessageReceiptTypeID = "spacewave-chat/message-receipt"

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

// NewChatMessageReceiptBlock constructs a new ChatMessageReceipt block.
func NewChatMessageReceiptBlock() block.Block {
	return &ChatMessageReceipt{}
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

// MarshalBlock marshals the ChatMessageReceipt to bytes.
func (r *ChatMessageReceipt) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatMessageReceipt from bytes.
func (r *ChatMessageReceipt) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatMessageReceipt.
func (r *ChatMessageReceipt) Validate() error {
	return nil
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
	return nil
}

var _ block.Block = (*ChatChannel)(nil)

var _ block.Block = (*ChatMessage)(nil)

var _ block.Block = (*ChatMessagePage)(nil)
