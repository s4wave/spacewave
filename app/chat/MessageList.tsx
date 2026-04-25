import { useRef, useEffect, useCallback } from 'react'

import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'

// truncatePeerId shortens a peer ID for display.
function truncatePeerId(id: string): string {
  if (id.length <= 12) return id
  return id.slice(0, 6) + '...' + id.slice(-4)
}

// formatTime formats a Date for display as HH:MM.
function formatTime(date: Date | undefined): string {
  if (!date) return ''
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

// MessageList renders a scrollable list of chat messages with auto-scroll.
export function MessageList({ messages }: { messages: ChatMessageInfo[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const wasAtBottomRef = useRef(true)

  // Auto-scroll when new messages arrive and user was at the bottom.
  // This is a DOM side effect (scroll position), so raw useEffect is correct.
  useEffect(() => {
    if (wasAtBottomRef.current && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [messages.length])

  const handleScroll = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    wasAtBottomRef.current =
      el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }, [])

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="flex-1 overflow-y-auto px-4 py-3"
    >
      {messages.length === 0 && (
        <div className="text-muted-foreground text-sm">No messages yet.</div>
      )}
      {messages.map((msg) => (
        <div key={msg.objectKey ?? ''} className="mb-2">
          <div className="flex items-baseline gap-2">
            <span className="text-foreground/60 text-xs font-medium">
              {truncatePeerId(msg.senderPeerId ?? '')}
            </span>
            <span className="text-foreground/40 text-xs">
              {formatTime(msg.createdAt)}
            </span>
          </div>
          <p className="text-foreground text-sm whitespace-pre-wrap">
            {msg.text ?? ''}
          </p>
        </div>
      ))}
    </div>
  )
}
