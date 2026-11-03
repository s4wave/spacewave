import { useEffect, useRef, type KeyboardEvent } from 'react'
import { CLASSES } from '@aptre/flex-layout'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: '100%',
    maxWidth: `calc(${themeLayout['--spacing']} * 64)`,
    minWidth: `calc(${themeLayout['--spacing']} * 12)`,
    borderRadius: themeRadii['--radius-menu-button'],
    '--tw-border-style': 'none',
    borderStyle: 'none',
    backgroundColor: themeColors['--color-background-secondary'],
    paddingInline: themeLayout['--spacing'],
    paddingBlock: 0,
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '-.01em',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
})

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
// its rename state names the tab, so the draft starts from the current name.
//
// FlexLayout renders every tab twice: the visible button and a hidden drag
// stamp, each through onRenderTab. Only the visible copy takes focus; a stamp
// copy that focused would blur the visible one, which saves and ends the
// rename the moment it starts.
export function FlexTabNameInput({
  sx,
  name,
  label,
  onSave,
  onCancel,
}: FlexTabNameInputProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
      sx={[[stylexStyles.rule1], sx]}
      defaultValue={name}
      onBlur={(event) => onSave(event.currentTarget.value.trim())}
      onKeyDown={handleKeyDown}
      onMouseDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
    />
  )
}
