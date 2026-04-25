import { useState, useRef, useCallback } from 'react'

// MessageInput provides a textarea for composing and sending chat messages.
// Enter sends the message, Shift+Enter inserts a newline.
export function MessageInput({
  onSend,
}: {
  onSend: (text: string) => Promise<void>
}) {
  const [state, setState] = useState({ text: '', sending: false })
  const ref = useRef<HTMLTextAreaElement>(null)

  const handleSubmit = useCallback(async () => {
    const trimmed = state.text.trim()
    if (!trimmed || state.sending) return
    setState({ text: '', sending: true })
    await onSend(trimmed).catch(() => {})
    setState((s) => ({ ...s, sending: false }))
    ref.current?.focus()
  }, [state.text, state.sending, onSend])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        handleSubmit()
      }
    },
    [handleSubmit],
  )

  return (
    <div className="border-foreground/8 border-t p-2">
      <textarea
        ref={ref}
        value={state.text}
        onChange={(e) => setState((s) => ({ ...s, text: e.target.value }))}
        onKeyDown={handleKeyDown}
        placeholder="Type a message..."
        rows={1}
        disabled={state.sending}
        className="bg-background-primary text-foreground w-full resize-none rounded border p-2 text-sm"
      />
    </div>
  )
}
