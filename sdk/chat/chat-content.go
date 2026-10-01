package spacewave_chat

import spacewave_chat_content "github.com/s4wave/spacewave/sdk/chat/content"

// ChatMessageContent is the shared generated content type used by storage and RPC.
type ChatMessageContent = spacewave_chat_content.ChatMessageContent

// ChatMessageContent_Text retains the public text-content constructor.
type ChatMessageContent_Text = spacewave_chat_content.ChatMessageContent_Text

// ChatMessageContent_Ciphertext carries an endpoint-encrypted message envelope.
type ChatMessageContent_Ciphertext = spacewave_chat_content.ChatMessageContent_Ciphertext

// ChatCiphertext is the shared encrypted envelope used by storage and RPC.
type ChatCiphertext = spacewave_chat_content.ChatCiphertext

// ChatRelation is shared relationship metadata stored beside supported message bodies.
type ChatRelation = spacewave_chat_content.ChatRelation

// ChatAnnotation is an attributed public reaction to a channel message.
type ChatAnnotation = spacewave_chat_content.ChatAnnotation

// ChatMessageContent_Annotation carries a public reaction independently of message encryption.
type ChatMessageContent_Annotation = spacewave_chat_content.ChatMessageContent_Annotation

// ChatStateChange is a public channel state event.
type ChatStateChange = spacewave_chat_content.ChatStateChange

// ChatMessageContent_StateChange carries a retained channel state event.
type ChatMessageContent_StateChange = spacewave_chat_content.ChatMessageContent_StateChange

// ChatEvent retains an external protocol message body.
type ChatEvent = spacewave_chat_content.ChatEvent

// ChatMessageContent_Event carries a protocol message in channel history.
type ChatMessageContent_Event = spacewave_chat_content.ChatMessageContent_Event

// ChatRedaction removes an earlier body by the same person.
type ChatRedaction = spacewave_chat_content.ChatRedaction

// ChatMessageContent_Redaction carries a public redaction in channel history.
type ChatMessageContent_Redaction = spacewave_chat_content.ChatMessageContent_Redaction
