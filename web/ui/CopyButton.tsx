import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEvent,
} from 'react'
import { LuCheck, LuCopy, LuTriangleAlert } from 'react-icons/lu'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: { color: themeColors['--color-emerald-500'] },
  rule4: { color: themeColors['--color-destructive'] },
  rule5: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule6: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule8: {
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule9: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
})

// CopyButtonProps configures the CopyButton.
export interface CopyButtonProps {
  className?: never
  // text is the string copied to the clipboard when the button is pressed.
  text: string
  // className is appended to the rendered button's class list.
  sx?: ComponentStyles
  // label overrides the default aria-label used in the idle state.
  label?: string
  // size selects between the small and medium icon sizes.
  size?: 'sm' | 'md'
  variant?: 'default' | 'toolbar'
}

// CopyButtonIcon renders the current feedback icon at the button's configured size.
function CopyButtonIcon({
  status,
  size,
}: {
  status: 'idle' | 'copied' | 'error'
  size: NonNullable<CopyButtonProps['size']>
}) {
  return (
    <>
      {status === 'copied' ? (
        <LuCheck
          {...stylex.props([
            size === 'md' ? stylexStyles.rule1 : stylexStyles.rule2,
            stylexStyles.rule3,
          ])}
        />
      ) : status === 'error' ? (
        <LuTriangleAlert
          {...stylex.props([
            size === 'md' ? stylexStyles.rule1 : stylexStyles.rule2,
            stylexStyles.rule4,
          ])}
        />
      ) : (
        <LuCopy
          {...stylex.props(
            size === 'md' ? stylexStyles.rule1 : stylexStyles.rule2,
          )}
        />
      )}
    </>
  )
}

// COPIED_RESET_MS is how long the "Copied" affordance stays visible.
const COPIED_RESET_MS = 2000

// CopyButton renders a small button that copies text to the clipboard
// and shows a transient "Copied" affordance. It is the shared primitive
// for inline copy buttons across the app.
export function CopyButton({
  text,
  sx,
  label = 'Copy',
  size = 'sm',
  variant = 'default',
}: Omit<CopyButtonProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  // Keep copy feedback, its reset timer and click generation together.
  const [status, setStatus] = useState<'idle' | 'copied' | 'error'>('idle')
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const generationRef = useRef(0)
  const mountedRef = useRef(true)

  // Bind the clipboard action to the text currently shown by this button.
  const handleCopy = useCallback(
    async (event: MouseEvent<HTMLButtonElement>) => {
      // Keep clipboard feedback within this click's generation.
      event.preventDefault()
      event.stopPropagation()
      const generation = ++generationRef.current
      if (timerRef.current != null) {
        clearTimeout(timerRef.current)
      }
      try {
        await navigator.clipboard.writeText(text)
        if (!mountedRef.current || generation !== generationRef.current) return
        setStatus('copied')
      } catch {
        if (!mountedRef.current || generation !== generationRef.current) return
        setStatus('error')
      }
      timerRef.current = setTimeout(() => {
        if (!mountedRef.current || generation !== generationRef.current) return
        setStatus('idle')
        timerRef.current = null
      }, COPIED_RESET_MS)
    },
    [text],
  )

  // Invalidate pending clipboard work and reset timers when the button unmounts.
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
      if (timerRef.current != null) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }
  }, [])

  // Keep the configured hit target separate from the feedback icon size.

  return (
    <button
      type="button"
      onClick={handleCopy}
      onMouseDown={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') event.stopPropagation()
      }}
      aria-label={
        status === 'copied'
          ? 'Copied'
          : status === 'error'
            ? 'Copy failed'
            : label
      }
      sx={[
        stylexStyles.rule5,
        variant === 'toolbar' && stylexStyles.rule6,
        size === 'md' ? stylexStyles.rule7 : stylexStyles.rule8,
        sx,
      ]}
    >
      <span sx={stylexStyles.rule9} role="status" aria-live="polite">
        {status === 'copied'
          ? 'Copied'
          : status === 'error'
            ? 'Copy failed'
            : ''}
      </span>
      <CopyButtonIcon status={status} size={size} />
    </button>
  )
}
