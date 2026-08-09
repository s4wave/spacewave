import { useCallback, useRef, useState } from 'react'

interface MessageInputState {
  error: string
  sending: boolean
  text: string
}

// MessageInput keeps the draft until the channel confirms the send.
export function MessageInput({
  onSend,
}: {
  onSend: (text: string) => Promise<void>
}) {
  const [state, setState] = useState<MessageInputState>({
    error: '',
    sending: false,
    text: '',
  })
  const ref = useRef<HTMLTextAreaElement>(null)

  const handleSubmit = useCallback(async () => {
    const trimmed = state.text.trim()
    if (!trimmed || state.sending) return
    setState((current) => ({ ...current, error: '', sending: true }))
    try {
      await onSend(trimmed)
      setState({ error: '', sending: false, text: '' })
    } catch {
      setState((current) => ({
        ...current,
        error: 'Message could not be sent. Try again.',
        sending: false,
      }))
    }
    ref.current?.focus()
  }, [onSend, state.sending, state.text])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault()
        void handleSubmit()
      }
    },
    [handleSubmit],
  )

  return (
    <div className="border-foreground/8 border-t p-2">
      <div className="flex items-end gap-2">
        <textarea
          ref={ref}
          value={state.text}
          onChange={(event) =>
            setState((current) => ({
              ...current,
              error: '',
              text: event.target.value,
            }))
          }
          onKeyDown={handleKeyDown}
          placeholder="Type a message…"
          rows={1}
          disabled={state.sending}
          className="bg-background-primary text-foreground min-w-0 flex-1 resize-none rounded border p-2 text-sm"
        />
        <button
          type="button"
          disabled={state.sending || !state.text.trim()}
          onClick={() => void handleSubmit()}
          className="border-foreground/10 bg-background-card text-foreground h-9 rounded border px-3 text-xs disabled:opacity-50"
        >
          {state.sending ? 'Sending…' : state.error ? 'Retry' : 'Send'}
        </button>
      </div>
      {state.error && (
        <p className="text-destructive mt-1 text-xs" role="alert">
          {state.error}
        </p>
      )}
    </div>
  )
}
