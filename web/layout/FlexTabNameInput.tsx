import { useEffect, useRef, type KeyboardEvent } from 'react'
import { CLASSES } from '@aptre/flex-layout'

import { cn } from '@s4wave/web/style/utils.js'

// FlexTabNameInputProps are the props for FlexTabNameInput.
export interface FlexTabNameInputProps {
  // name is the draft's starting value.
  name: string
  // label is the input's accessible name.
  label: string
  // onSave receives the trimmed draft on Enter or blur.
  onSave: (name: string) => void
  // onCancel ends the rename on Escape without saving.
  onCancel: () => void
}

// FlexTabNameInput edits one FlexLayout tab name. The caller mounts it while
// its rename state references the tab, so the draft starts from the current
// name.
//
// FlexLayout renders every tab twice: the visible button and a hidden drag
// stamp, each through onRenderTab. Only the visible copy takes focus. A stamp
// copy that focused would blur the visible copy, saving and ending the rename
// the moment the rename starts.
export function FlexTabNameInput({
  name,
  label,
  onSave,
  onCancel,
}: FlexTabNameInputProps) {
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const input = inputRef.current
    if (!input || input.closest(`.${CLASSES.FLEXLAYOUT__TAB_BUTTON_STAMP}`)) {
      return
    }
    input.focus()
    input.select()
  }, [])

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    event.stopPropagation()
    if (event.nativeEvent.isComposing) return
    if (event.key === 'Enter') {
      event.preventDefault()
      onSave(event.currentTarget.value.trim())
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      onCancel()
    }
  }

  return (
    <input
      ref={inputRef}
      aria-label={label}
      className={cn(
        'bg-background-secondary text-foreground rounded-menu-button',
        'border-none outline-none',
        'text-metadata leading-5 font-medium tracking-tight-brand',
        'w-full max-w-64 min-w-12 px-1 py-0',
      )}
      defaultValue={name}
      onBlur={(event) => onSave(event.currentTarget.value.trim())}
      onKeyDown={handleKeyDown}
      onMouseDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
    />
  )
}
