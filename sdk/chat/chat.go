package spacewave_chat

import (
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// ChatChannelTypeID is the type identifier for chat channel objects.
const ChatChannelTypeID = "spacewave-chat/channel"

// ChatMessageTypeID is the type identifier for chat message objects.
const ChatMessageTypeID = "spacewave-chat/message"

// ChatMessagePageTypeID is the type identifier for channel message pages.
const ChatMessagePageTypeID = "spacewave-chat/message-page"

// ChatThreadTypeID is the type identifier for thread summary objects.
const ChatThreadTypeID = "spacewave-chat/thread"

// PredChannelMessage is the graph predicate linking a channel to its messages.
var PredChannelMessage = quad.IRI("spacewave-chat/channel-message")

// PredMessageSender is the graph predicate linking a message to its sender.
var PredMessageSender = quad.IRI("spacewave-chat/message-sender")

// PredChannelState links a channel to the latest event for each state identity.
var PredChannelState = quad.IRI("spacewave-chat/channel-state")

// PredThreadParticipant marks a thread summary with an attributed person label.
var PredThreadParticipant = quad.IRI("spacewave-chat/thread-participant")

// NewChatStateQuad links a channel state identity to its retained message.
// Empty messageKey selects the current event for this identity in a graph query.
func NewChatStateQuad(channelKey, messageKey, stateType, stateKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(channelKey, PredChannelState.String(), messageKey, quad.String(stateType+"\x00"+stateKey).String())
}

// NewChatThreadParticipantQuad records a verified person who authored a canonical thread reply.
func NewChatThreadParticipantQuad(threadKey, personPeerID string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		threadKey,
		PredThreadParticipant.String(),
		threadKey,
		quad.String(personPeerID).String(),
	)
}

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

// NewChatThreadBlock constructs a new ChatThread block.
func NewChatThreadBlock() block.Block {
	return &ChatThread{}
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

// MarshalBlock marshals the ChatThread to bytes.
func (t *ChatThread) MarshalBlock() ([]byte, error) {
	return t.MarshalVT()
}

// UnmarshalBlock unmarshals the ChatThread from bytes.
func (t *ChatThread) UnmarshalBlock(data []byte) error {
	return t.UnmarshalVT(data)
}

// Validate performs cursory checks on the ChatThread.
func (t *ChatThread) Validate() error {
	return nil
}

var _ block.Block = (*ChatChannel)(nil)

var _ block.Block = (*ChatMessage)(nil)

var _ block.Block = (*ChatMessagePage)(nil)

var _ block.Block = (*ChatThread)(nil)
