import { useId, type KeyboardEvent, type ReactNode } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

export interface TypedConfirmFieldProps {
  label: ReactNode
  value: string
  onChange: (value: string) => void
  placeholder: string
  // ariaLabel overrides the accessible name derived from the label.
  ariaLabel?: string
  // canSubmit gates onSubmit when the user presses Enter.
  canSubmit: boolean
  onSubmit: () => void
}

// TypedConfirmField is the labelled text input of a destructive confirmation
// step. Enter submits when canSubmit holds, except while an IME composes.
export function TypedConfirmField({
  label,
  value,
  onChange,
  placeholder,
  ariaLabel,
  canSubmit,
  onSubmit,
}: TypedConfirmFieldProps) {
  const inputId = useId()

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.nativeEvent.isComposing) return
    if (e.key === 'Enter' && canSubmit) {
      onSubmit()
    }
  }

  return (
    <div>
      <label
        htmlFor={inputId}
        className="text-foreground-alt mb-1.5 block text-xs select-none"
      >
        {label}
      </label>
      <input
        id={inputId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        aria-label={ariaLabel}
        className={cn(
          'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
          'focus:border-destructive/50',
        )}
        onKeyDown={handleKeyDown}
      />
    </div>
  )
}
