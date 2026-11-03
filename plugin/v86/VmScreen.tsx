import { useCallback, useEffect, useRef, useState } from 'react'
import type { KeyEvent } from '@aptre/v86'

import { attachScreenView, type ScreenView } from './screen-channel.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    minHeight: 0,
    flex: 1,
    cursor: 'text',
    overflow: 'hidden',
    backgroundColor: 'var(--color-black)',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
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
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    },
    '--tw-ring-inset': { default: null, ':focus-visible': 'inset' },
  },
  rule2: { display: 'none' },
  rule3: {
    pointerEvents: 'none',
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 2)`,
    right: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// VmScreenProps configures the screen of one VM.
export interface VmScreenProps {
  // objectKey is the VmV86 whose VGA screen and keyboard the view attaches to.
  objectKey: string
  // hidden keeps the screen mounted but not shown.
  hidden?: boolean
}

// VmScreen draws the VM's VGA screen scaled to fit. Clicking the screen
// captures the keyboard for the guest until Ctrl+Alt+G or focus leaves.
export function VmScreen({ objectKey, hidden }: VmScreenProps) {
  const displayRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<ScreenView | null>(null)
  const [captured, setCaptured] = useState(false)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    const display = displayRef.current
    if (!display || !objectKey) return

    // Load the screen adapter on first use and attach it to the runner.
    let closed = false
    import('@aptre/v86/screen')
      .then(({ ScreenAdapter }) => {
        if (closed) return
        viewRef.current = attachScreenView(ScreenAdapter, display, objectKey)
      })
      .catch((err: unknown) => {
        if (!closed)
          setLoadError(err instanceof Error ? err.message : String(err))
      })

    return () => {
      closed = true
      viewRef.current?.close()
      viewRef.current = null
      display.replaceChildren()
    }
  }, [objectKey])

  // release returns the keyboard to the page and lifts every guest key.
  const release = useCallback(() => {
    setCaptured(false)
    viewRef.current?.release()
  }, [])

  // handleKey sends captured keys to the guest; Ctrl+Alt+G releases.
  const handleKey = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>, down: boolean) => {
      if (!captured) return
      e.preventDefault()
      e.stopPropagation()
      if (down && e.ctrlKey && e.altKey && e.code === 'KeyG') {
        release()
        return
      }
      viewRef.current?.key(toKeyEvent(e), down)
    },
    [captured, release],
  )

  return (
    <div
      role="application"
      aria-label="Virtual machine screen"
      tabIndex={0}
      onClick={(e) => {
        e.currentTarget.focus()
        setCaptured(true)
      }}
      onKeyDown={(e) => handleKey(e, true)}
      onKeyUp={(e) => handleKey(e, false)}
      onBlur={() => {
        if (captured) release()
      }}
      sx={[stylexStyles.rule1, !!hidden && stylexStyles.rule2]}
    >
      <div
        ref={displayRef}
        className={
          /* TODO(stylex): global or unknown class: v86-screen */ 'v86-screen'
        }
        sx={screenContainerStyles.root}
      />
      <div
        sx={[
          stylexStyles.rule3,
          captured ? stylexStyles.rule4 : stylexStyles.rule5,
        ]}
        aria-live="polite"
      >
        {loadError
          ? `The screen failed to load: ${loadError}`
          : captured
            ? 'Keyboard captured. Ctrl+Alt+G releases.'
            : 'Click to type into the VM'}
      </div>
    </div>
  )
}

// toKeyEvent keeps the fields of a key event the v86 keyboard reads.
function toKeyEvent(e: React.KeyboardEvent): KeyEvent {
  return {
    code: e.code,
    key: e.key,
    keyCode: e.keyCode,
    altKey: e.altKey,
    shiftKey: e.shiftKey,
    ctrlKey: e.ctrlKey,
    repeat: e.repeat,
  }
}

// screenContainerStyles sizes the mount whose children the VGA adapter owns.
const screenContainerStyles = stylex.create({
  root: {
    display: 'flex',
    width: '100%',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
  },
})
