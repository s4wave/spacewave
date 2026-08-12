import { LuCircleAlert, LuHash, LuRefreshCw } from 'react-icons/lu'
import { useCallback, useEffect, useState } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'

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

      {loading ? (
        <div
          className="flex flex-1 items-center justify-center p-6"
          role="status"
        >
          <div className="flex items-center gap-3 text-sm">
            <div className="border-brand/30 border-t-brand size-5 animate-spin rounded-full border-2" />
            <div>
              <div className="text-foreground font-medium">
                Loading messages
              </div>
              <div className="text-muted-foreground text-xs">
                Reading conversation history…
              </div>
            </div>
          </div>
        </div>
      ) : error ? (
        <div
          className="flex flex-1 items-center justify-center p-6"
          role="alert"
        >
          <div className="border-error/30 bg-background-card max-w-sm rounded-md border p-5 text-center">
            <LuCircleAlert
              className="text-error-text mx-auto size-6"
              aria-hidden="true"
            />
            <h2 className="text-foreground mt-3 text-sm font-semibold">
              Messages unavailable
            </h2>
            <p className="text-muted-foreground mt-1 text-sm leading-5">
              The channel history could not be read. Check the connection and
              try again.
            </p>
            <button
              type="button"
              onClick={messagesResource.retry}
              className="border-foreground/15 bg-background-tertiary text-foreground hover:bg-background-panel focus-visible:ring-primary mx-auto mt-4 flex items-center gap-2 rounded-sm border px-3 py-2 text-xs font-semibold focus-visible:ring-2"
            >
              <LuRefreshCw className="size-3.5" aria-hidden="true" />
              Try again
            </button>
          </div>
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
