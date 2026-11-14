import { LuMessageCircle } from 'react-icons/lu'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'
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
      <div className="viewer-inset flex-1 overflow-auto">
        <InfoCard>
          <FactList>
            {message?.senderPeerId && (
              <FactRow label="Sender Peer ID" value={message.senderPeerId} />
            )}
            {message?.createdAt && (
              <FactRow
                kind="text"
                label="Created At"
                value={message.createdAt.toISOString()}
              />
            )}
            {message?.replyToKey && (
              <FactRow label="Reply To Key" value={message.replyToKey} />
            )}
          </FactList>
          {textContent && (
            <p className="text-foreground mt-3 text-sm break-words whitespace-pre-wrap">
              {textContent}
            </p>
          )}
        </InfoCard>
      </div>
    </div>
  )
}
