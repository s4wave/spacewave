import { LuSendHorizontal } from '@s4wave/web/images/icons.js'
import { useCallback, useEffect, useRef, useState } from 'react'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  radii as themeRadii,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flexShrink: 0,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-secondary']} 70%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      ':focus-within': `color-mix(in oklab, ${themeColors['--color-primary']} 60%, transparent)`,
    },
    backgroundColor: themeColors['--color-background-primary'],
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    maxHeight: `calc(${themeLayout['--spacing']} * 32)`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 9)`,
      },
    },
    minWidth: 0,
    flex: 1,
    resize: 'none',
    backgroundColor: '#0000',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-leading': `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.6 },
  },
  rule4: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 9)`,
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 9)`,
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 'var(--radius-sm)',
    backgroundColor: {
      default: themeColors['--color-primary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-primary']} 90%, transparent)`,
      },
    },
    color: themeColors['--color-primary-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-primary'],
    },
    '--tw-ring-offset-width': { default: null, ':focus-visible': '2px' },
    '--tw-ring-offset-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 var(--tw-ring-offset-width) var(--tw-ring-offset-color)',
    },
    '--tw-ring-offset-color': {
      default: null,
      ':focus-visible': themeColors['--color-background'],
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.35 },
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: { color: themeColors['--color-error-text'] },
  rule7: {
    marginLeft: 'auto',
    display: {
      default: 'none',
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 'inline',
      },
    },
    color: themeColors['--color-muted-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface MessageInputProps {
  disabled?: boolean
  onSend: (text: string) => Promise<void>
}

/** MessageInput provides the channel composer and preserves a failed draft for retry. */
export function MessageInput({ disabled = false, onSend }: MessageInputProps) {
  // Keep the draft and focus restoration with the send operation.
  const [state, setState] = useState({ text: '', sending: false, error: '' })
  const ref = useRef<HTMLTextAreaElement>(null)
  const restoreFocusRef = useRef(false)
  useEffect(() => {
    if (!state.sending && restoreFocusRef.current) {
      restoreFocusRef.current = false
      ref.current?.focus()
    }
  }, [state.sending])

  // Send once and retain a failed draft for an explicit retry.
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

  // Keep the composer reachable by touch in either orientation.
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
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
          sx={stylexStyles2.rule3}
        />
        <button
          type="button"
          aria-label={state.sending ? 'Sending message' : 'Send message'}
          disabled={!canSend}
          onClick={() => void handleSubmit()}
          sx={stylexStyles2.rule4}
        >
          <LuSendHorizontal sx={stylexStyles.rule1} aria-hidden="true" />
        </button>
      </div>
      <div sx={stylexStyles2.rule5}>
        <span role="alert" sx={stylexStyles2.rule6}>
          {state.error}
        </span>
        <span sx={stylexStyles2.rule7}>
          Enter to send · Shift+Enter for a new line
        </span>
      </div>
    </div>
  )
}
