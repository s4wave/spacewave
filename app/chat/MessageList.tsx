import { LuMessageCircle } from 'react-icons/lu'
import { useCallback, useEffect, useRef } from 'react'

import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'

function peerLabel(id: string): string {
  if (!id) return 'Unknown peer'
  if (id.length <= 14) return id
  return `${id.slice(0, 7)}…${id.slice(-5)}`
}

const timeFormatter = new Intl.DateTimeFormat(undefined, {
  hour: 'numeric',
  minute: '2-digit',
})

function formatTime(date: Date | undefined): string {
  return date ? timeFormatter.format(date) : ''
}

// MessageList renders channel history and follows new messages while the reader remains at the bottom.
export function MessageList({ messages }: { messages: ChatMessageInfo[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const wasAtBottomRef = useRef(true)

  useEffect(() => {
    if (wasAtBottomRef.current && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [messages.length])

  const handleScroll = useCallback(() => {
    const element = containerRef.current
    if (!element) return
    wasAtBottomRef.current =
      element.scrollHeight - element.scrollTop - element.clientHeight < 40
  }, [])

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      aria-label="Channel messages"
      className="flex-1 overflow-y-auto overscroll-contain px-3 py-4 sm:px-5"
    >
      {messages.length === 0 ? (
        <div className="mx-auto flex h-full max-w-sm flex-col items-center justify-center px-6 py-12 text-center">
          <div className="bg-background-tertiary text-brand mb-4 flex size-11 items-center justify-center rounded-full">
            <LuMessageCircle className="size-5" aria-hidden="true" />
          </div>
          <h2 className="text-foreground text-sm font-semibold">
            Start the conversation
          </h2>
          <p className="text-muted-foreground mt-1.5 text-sm leading-5">
            This channel is ready. Send the first message to everyone in the
            peer group.
          </p>
        </div>
      ) : (
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-1">
          {messages.map((message, index) => {
            const previous = messages[index - 1]
            const grouped = previous?.senderPeerId === message.senderPeerId
            return (
              <article
                key={message.objectKey}
                className={
                  grouped ? 'pt-0.5 pl-11' : 'flex gap-3 pt-4 first:pt-0'
                }
              >
                {!grouped && (
                  <div className="bg-background-tertiary text-foreground-alt flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold uppercase">
                    {(message.senderPeerId || '?').slice(0, 1)}
                  </div>
                )}
                <div className="min-w-0 flex-1">
                  {!grouped && (
                    <div className="mb-0.5 flex min-w-0 items-baseline gap-2">
                      <span className="text-foreground-alt truncate text-xs font-semibold">
                        {peerLabel(message.senderPeerId ?? '')}
                      </span>
                      <time className="text-muted-foreground shrink-0 text-xs">
                        {formatTime(message.createdAt)}
                      </time>
                    </div>
                  )}
                  <p className="text-foreground text-sm leading-6 break-words whitespace-pre-wrap">
                    {message.text ?? ''}
                  </p>
                </div>
              </article>
            )
          })}
        </div>
      )}
    </div>
  )
}
