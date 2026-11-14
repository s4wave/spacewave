import { LuHash } from 'react-icons/lu'
import { useCallback, useEffect, useState } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'

import { ChatHandle, ChatChannelTypeID } from '@s4wave/sdk/chat/chat.js'
import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'
import { MessageInput } from './MessageInput.js'
import { MessageList } from './MessageList.js'

export { ChatChannelTypeID }

// ChatChannelViewer displays live channel history and its composer.
export function ChatChannelViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  return (
    <ChatChannelContent
      key={objectKey}
      objectKey={objectKey}
      worldState={worldState}
    />
  )
}

function ChatChannelContent({
  objectKey,
  worldState,
}: {
  objectKey: string
  worldState: ObjectViewerComponentProps['worldState']
}) {
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    ChatHandle,
    ChatChannelTypeID,
  )
  const [messages, setMessages] = useState<ChatMessageInfo[]>([])

  const streamFactory = useCallback((chat: ChatHandle, signal: AbortSignal) => {
    setMessages([])
    return chat.watchMessages(signal)
  }, [])
  const messagesResource = useStreamingResource(handle, streamFactory, [])
  const infoResource = useStreamingResource(
    handle,
    (chat: ChatHandle, signal: AbortSignal) => chat.watchChannelInfo(signal),
    [],
  )
  useObjectName(infoResource.value?.name)

  useEffect(() => {
    const batch = messagesResource.value
    if (messagesResource.loading || !batch?.length) return
    setMessages((current) => {
      const keys = new Set(current.map((message) => message.objectKey))
      const additions = batch.filter((message) => !keys.has(message.objectKey))
      return additions.length === 0 ? current : [...current, ...additions]
    })
  }, [messagesResource.loading, messagesResource.value])

  const handleSend = useCallback(
    async (text: string) => {
      const chat = handle.value
      if (!chat) throw new Error('The channel is not connected yet.')
      await chat.sendMessage(text)
    },
    [handle.value],
  )

  const loading = messagesResource.loading && messages.length === 0
  const error = messagesResource.error

  return (
    <section
      className="bg-background-primary flex h-full min-h-0 w-full flex-col"
      aria-label="Chat channel"
    >
      <header className="border-foreground/10 bg-background-secondary/60 flex min-h-14 shrink-0 items-center gap-3 border-b px-3 sm:px-5">
        <div className="bg-background-tertiary text-brand flex size-8 shrink-0 items-center justify-center rounded-sm">
          <LuHash className="size-4" aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <h1 className="text-foreground truncate text-sm font-semibold">
            Chat channel
          </h1>
          <p className="text-muted-foreground truncate text-xs">
            {objectKey || 'Conversation'}
          </p>
        </div>
      </header>

      {error ? (
        <div className="min-h-0 flex-1">
          <ViewerStatus
            state="failed"
            message={`Could not read the messages: ${error.message}`}
            retry={messagesResource.retry}
          />
        </div>
      ) : loading ? (
        <div className="min-h-0 flex-1">
          <ViewerStatus state="waiting" what="the messages" />
        </div>
      ) : (
        <MessageList messages={messages} />
      )}

      <MessageInput
        disabled={loading || Boolean(error) || !handle.value}
        onSend={handleSend}
      />
    </section>
  )
}
