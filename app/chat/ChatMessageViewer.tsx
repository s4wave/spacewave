import { LuMessageCircle } from 'react-icons/lu'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ChatMessage } from '@s4wave/sdk/chat/chat.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

export const ChatMessageTypeID = 'spacewave-chat/message'

// ChatMessageViewer displays a Chat Message entity.
export function ChatMessageViewer({ objectState }: ObjectViewerComponentProps) {
  const message = useForgeBlockData(objectState, ChatMessageTypeID, ChatMessage)

  const textContent =
    message?.content?.content?.case === 'text'
      ? message.content.content.value
      : undefined

  return (
    <div className="bg-background-primary flex h-full w-full flex-col overflow-auto">
      <ViewerBar icon={<LuMessageCircle />} title="Message" />
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
