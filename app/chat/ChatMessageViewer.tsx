import { LuMessageCircle } from 'react-icons/lu'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ChatMessage } from '@s4wave/sdk/chat/chat.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'

export const ChatMessageTypeID = 'spacewave-chat/message'

// ChatMessageViewer displays a Chat Message entity.
export function ChatMessageViewer({
  objectInfo,
  objectState,
}: ObjectViewerComponentProps) {
  const message = useForgeBlockData(objectState, ChatMessage)

  const textContent =
    message?.content?.content?.case === 'text' ?
      message.content.content.value
    : undefined

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          <LuMessageCircle className="h-4 w-4" />
          <span className="tracking-tight">Message</span>
        </div>
      </div>
      <div className="flex-1 overflow-auto px-4 py-3">
        <InfoCard>
          <div className="space-y-2">
            {message?.senderPeerId && (
              <CopyableField
                label="Sender Peer ID"
                value={message.senderPeerId}
              />
            )}
            {textContent && (
              <CopyableField label="Content" value={textContent} />
            )}
            {message?.createdAt && (
              <CopyableField
                label="Created At"
                value={message.createdAt.toISOString()}
              />
            )}
            {message?.replyToKey && (
              <CopyableField label="Reply To Key" value={message.replyToKey} />
            )}
          </div>
        </InfoCard>
      </div>
    </div>
  )
}
