package spacewave_chat

import (
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// PredChannelMessage is the graph predicate linking a channel to its messages.
var PredChannelMessage = quad.IRI("spacewave-chat/channel-message")

// PredMessageSender is the graph predicate linking a message to its sender.
var PredMessageSender = quad.IRI("spacewave-chat/message-sender")

// PredChannelState links a channel to the latest event for each state identity.
var PredChannelState = quad.IRI("spacewave-chat/channel-state")

// PredPendingRedaction links a channel to a redaction whose target has not
// arrived. The label holds the target's envelope message key.
var PredPendingRedaction = quad.IRI("spacewave-chat/pending-redaction")

// PredThreadParticipant marks a thread summary with an attributed person label.
var PredThreadParticipant = quad.IRI("spacewave-chat/thread-participant")

// NewChatStateQuad links a channel state identity to its retained message.
// Empty messageKey selects the current event for this identity in a graph query.
func NewChatStateQuad(channelKey, messageKey, stateType, stateKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(channelKey, PredChannelState.String(), messageKey, quad.String(stateType+"\x00"+stateKey).String())
}

// NewChatPendingRedactionQuad records a redaction waiting for its target.
// Empty redactionKey selects every redaction waiting for targetKey.
func NewChatPendingRedactionQuad(channelKey, redactionKey, targetKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(channelKey, PredPendingRedaction.String(), redactionKey, quad.String(targetKey).String())
}

// NewChatThreadParticipantQuad records a verified person who authored a canonical thread reply.
func NewChatThreadParticipantQuad(threadKey, personID string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		threadKey,
		PredThreadParticipant.String(),
		threadKey,
		quad.String(personID).String(),
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

// Validate performs cursory checks on the ChatChannel.
func (c *ChatChannel) Validate() error {
	return nil
}

// Validate performs cursory checks on the ChatMessage.
func (m *ChatMessage) Validate() error {
	return nil
}

// Validate performs cursory checks on the ChatMessagePage.
func (m *ChatMessagePage) Validate() error {
	return nil
}

// Validate performs cursory checks on the ChatThread.
func (t *ChatThread) Validate() error {
	return nil
}

var _ block.Block = (*ChatChannel)(nil)

var _ block.Block = (*ChatMessage)(nil)

var _ block.Block = (*ChatMessagePage)(nil)

var _ block.Block = (*ChatThread)(nil)
