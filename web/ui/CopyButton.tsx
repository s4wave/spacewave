import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEvent,
} from 'react'
import { LuCheck, LuCopy, LuTriangleAlert } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  motion as themeMotion,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
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
  rule2: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule5: {
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
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: { color: themeColors['--color-emerald-500'] },
  rule9: { color: themeColors['--color-destructive'] },
})

// CopyButtonProps configures the CopyButton.
export interface CopyButtonProps {
  className?: never
  // text is the string copied to the clipboard when the button is pressed.
  text: string
  // sx overrides the button's styles.
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
          sx={[
            size === 'md' ? stylexStyles.rule6 : stylexStyles.rule7,
            stylexStyles.rule8,
          ]}
        />
      ) : status === 'error' ? (
        <LuTriangleAlert
          sx={[
            size === 'md' ? stylexStyles.rule6 : stylexStyles.rule7,
            stylexStyles.rule9,
          ]}
        />
      ) : (
        <LuCopy sx={size === 'md' ? stylexStyles.rule6 : stylexStyles.rule7} />
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
        stylexStyles.rule1,
        !!(variant === 'toolbar') && stylexStyles.rule2,
        size === 'md' ? stylexStyles.rule3 : stylexStyles.rule4,
        sx,
      ]}
    >
      <span sx={stylexStyles.rule5} role="status" aria-live="polite">
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
