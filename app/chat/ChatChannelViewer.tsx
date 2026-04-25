import { useCallback, useEffect, useRef, useState } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { useRenderDelay } from '@s4wave/app/loading/useRenderDelay.js'
import { ChatHandle, ChatChannelTypeID } from '@s4wave/sdk/chat/chat.js'
import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'
import { MessageList } from './MessageList.js'
import { MessageInput } from './MessageInput.js'

export { ChatChannelTypeID }

// ChatChannelViewer displays a live chat interface for a chat channel object.
// It streams message batches via WatchMessages and accumulates them for display.
export function ChatChannelViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    ChatHandle,
    ChatChannelTypeID,
  )

  // Accumulate streamed message batches in a ref.
  // Each yield from watchMessages is a delta (new messages only).
  const allRef = useRef<ChatMessageInfo[]>([])
  const [version, setVersion] = useState(0)

  const streamFactory = useCallback((h: ChatHandle, signal: AbortSignal) => {
    allRef.current = []
    setVersion(0)
    return h.watchMessages(signal)
  }, [])

  const messagesResource = useStreamingResource(handle, streamFactory, [])

  // 300 ms render delay so fast history loads do not flash the card.
  const showLoadingCard = useRenderDelay(300)

  // Merge each new batch into the accumulator.
  // This is a synchronous state derivation from streamed values, not async data loading.
  useEffect(() => {
    const batch = messagesResource.value
    if (!batch || batch.length === 0) return
    const existing = new Set(allRef.current.map((m) => m.objectKey))
    const newMsgs = batch.filter((m) => !existing.has(m.objectKey))
    if (newMsgs.length === 0) return
    allRef.current = [...allRef.current, ...newMsgs]
    setVersion((v) => v + 1)
  }, [messagesResource.value])

  // Suppress unused-variable warning; version drives re-renders.
  void version

  const handleSend = useCallback(
    async (text: string) => {
      const h = handle.value
      if (!h) return
      await h.sendMessage(text)
    },
    [handle.value],
  )

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Chat
        </span>
      </div>
      {messagesResource.loading &&
        allRef.current.length === 0 &&
        showLoadingCard && (
          <div className="flex flex-1 items-center justify-center p-6">
            <div className="w-full max-w-sm">
              <LoadingCard
                view={{
                  state: 'active',
                  title: 'Loading messages',
                  detail: 'Reading channel history from the peer group.',
                }}
              />
            </div>
          </div>
        )}
      <MessageList messages={allRef.current} />
      <MessageInput onSend={handleSend} />
    </div>
  )
}
