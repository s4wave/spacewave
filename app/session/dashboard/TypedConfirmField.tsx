import { useId, type KeyboardEvent, type ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
})

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
  sx,
  label,
  value,
  onChange,
  placeholder,
  ariaLabel,
  canSubmit,
  onSubmit,
}: TypedConfirmFieldProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const inputId = useId()

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.nativeEvent.isComposing) return
    if (e.key === 'Enter' && canSubmit) {
      onSubmit()
    }
  }

  return (
    <div sx={sx}>
      <label htmlFor={inputId} sx={stylexStyles.rule1}>
        {label}
      </label>
      <input
        id={inputId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        aria-label={ariaLabel}
        sx={[stylexStyles.rule2]}
        onKeyDown={handleKeyDown}
      />
    </div>
  )
}
