import { LuSendHorizontal } from 'react-icons/lu'
import { useCallback, useEffect, useRef, useState } from 'react'

interface MessageInputProps {
  disabled?: boolean
  onSend: (text: string) => Promise<void>
}

// MessageInput provides the channel composer and preserves a failed draft for retry.
export function MessageInput({ disabled = false, onSend }: MessageInputProps) {
  const [state, setState] = useState({ text: '', sending: false, error: '' })
  const ref = useRef<HTMLTextAreaElement>(null)
  const restoreFocusRef = useRef(false)
  useEffect(() => {
    if (!state.sending && restoreFocusRef.current) {
      restoreFocusRef.current = false
      ref.current?.focus()
    }
  }, [state.sending])

  const canSend = state.text.trim().length > 0 && !state.sending && !disabled

  const handleSubmit = useCallback(async () => {
    const trimmed = state.text.trim()
    if (!trimmed || state.sending || disabled) return
    setState((current) => ({ ...current, sending: true, error: '' }))
    try {
      await onSend(trimmed)
      restoreFocusRef.current = true
      setState({ text: '', sending: false, error: '' })
    } catch (error) {
      restoreFocusRef.current = true
      setState((current) => ({
        ...current,
        sending: false,
        error:
          error instanceof Error ? error.message : 'Message could not be sent.',
      }))
    }
  }, [disabled, onSend, state.sending, state.text])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault()
        void handleSubmit()
      }
    },
    [handleSubmit],
  )

  return (
    <div className="border-foreground/10 bg-background-secondary/70 shrink-0 border-t px-3 py-3 sm:px-4">
      <div className="border-foreground/15 bg-background-primary focus-within:border-primary/60 flex items-end gap-2 rounded-md border p-2 transition-colors">
        <textarea
          ref={ref}
          aria-label="Message"
          value={state.text}
          onChange={(event) =>
            setState((current) => ({
              ...current,
              text: event.target.value,
              error: '',
            }))
          }
          onKeyDown={handleKeyDown}
          placeholder="Message this channel"
          rows={1}
          disabled={state.sending || disabled}
          className="text-foreground placeholder:text-muted-foreground max-h-32 min-h-9 flex-1 resize-none bg-transparent p-2 text-sm leading-5 outline-none disabled:cursor-not-allowed disabled:opacity-60"
        />
        <button
          type="button"
          aria-label={state.sending ? 'Sending message' : 'Send message'}
          disabled={!canSend}
          onClick={() => void handleSubmit()}
          className="bg-primary text-primary-foreground hover:bg-primary/90 focus-visible:ring-primary focus-visible:ring-offset-background flex size-9 shrink-0 items-center justify-center rounded-sm transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-35"
        >
          <LuSendHorizontal className="size-4" aria-hidden="true" />
        </button>
      </div>
      <div className="mt-1.5 flex min-h-4 items-center justify-between gap-3 px-1 text-xs">
        <span role="alert" className="text-error-text">
          {state.error}
        </span>
        <span className="text-muted-foreground ml-auto hidden sm:inline">
          Enter to send · Shift+Enter for a new line
        </span>
      </div>
    </div>
  )
}
