import { useEffect, useState } from 'react'

import { useKeyDispatcherState } from './KeyDispatcher.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  utilityState as themeUtilityState,
  layout as themeLayout,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-backdrop-blur': 'blur(var(--blur-xs))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    inset: 0,
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    pointerEvents: 'none',
    position: 'fixed',
    zIndex: 40,
  },
  rule2: {
    '--tw-shadow': '0 0 #0000',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    pointerEvents: 'auto',
    position: 'fixed',
    zIndex: 50,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    bottom: 0,
    left: 0,
    right: 0,
  },
  rule3: {
    marginInline: 'auto',
    maxWidth: 'var(--container-3xl)',
    width: '100%',
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-brand'],
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    textTransform: 'uppercase',
  },
  rule6: {
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule8: {
    flex: 1,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    minWidth: 0,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
    maxHeight: `calc(${themeLayout['--spacing']} * 72)`,
    overflowY: 'auto',
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    minWidth: 0,
  },
  rule12: {
    borderColor: themeColors['--color-brand'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule13: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    minWidth: `calc(${themeLayout['--spacing']} * 24)`,
  },
  rule14: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-brand'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule15: { flex: 1, minWidth: 0 },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule17: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule18: {
    color: themeColors['--color-warning'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule20: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    marginInline: 'auto',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-warning'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    maxWidth: 'var(--container-3xl)',
  },
})

// WhichKeyPanel renders the active key sequence continuations owned by KeyDispatcher.
export function WhichKeyPanel() {
  const state = useKeyDispatcherState()
  const [visible, setVisible] = useState(state.mode === 'prefix')

  useEffect(() => {
    if (state.mode !== 'prefix') {
      setVisible(false)
      return
    }
    if (state.whichKeyDelayMs <= 0) {
      setVisible(true)
      return
    }
    setVisible(false)
    const timeout = window.setTimeout(() => {
      setVisible(true)
    }, state.whichKeyDelayMs)
    return () => {
      window.clearTimeout(timeout)
    }
  }, [state.mode, state.whichKeyDelayMs])

  if (state.mode !== 'prefix' || !visible) return null
  const path = state.activePath.map(formatResolvedKey).join(' ')
  const query = state.query ?? ''
  const selectedIndex = state.selectedIndex ?? 0

  return (
    <>
      <div sx={stylexStyles.rule1} />
      <section aria-label="Key sequence continuations" sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
          <div sx={stylexStyles.rule4}>
            <div sx={stylexStyles.rule5}>{path}</div>
            {query ? (
              <div sx={stylexStyles.rule6}>
                Filter: <span sx={stylexStyles.rule7}>{query}</span>
              </div>
            ) : (
              <div sx={stylexStyles.rule8}>Type a chord or command name</div>
            )}
            <div sx={stylexStyles.rule9}>
              ↑↓ Navigate · Enter Run · Esc Cancel
            </div>
          </div>
          <div
            aria-label="Matching key sequence commands"
            sx={stylexStyles.rule10}
            role="listbox"
          >
            {state.continuations.map((continuation, index) => {
              const remainingKeys =
                continuation.remainingKeys ??
                (continuation.key ? [continuation.key] : [])
              return (
                <div
                  aria-selected={index === selectedIndex}
                  key={`${continuation.key}:${continuation.commandId ?? 'branch'}`}
                  sx={[
                    stylexStyles.rule11,
                    index === selectedIndex && stylexStyles.rule12,
                  ]}
                  role="option"
                >
                  <div sx={stylexStyles.rule13}>
                    {remainingKeys.length > 0 ? (
                      remainingKeys.map((key, keyIndex) => (
                        <kbd
                          sx={stylexStyles.rule14}
                          key={`${key}:${keyIndex}`}
                        >
                          {formatResolvedKey(key)}
                        </kbd>
                      ))
                    ) : (
                      <kbd sx={stylexStyles.rule14}>Enter</kbd>
                    )}
                  </div>
                  <div sx={stylexStyles.rule15}>
                    <div sx={stylexStyles.rule16}>
                      {continuation.label ??
                        continuation.commandId ??
                        'More commands'}
                    </div>
                    {continuation.commandId && (
                      <div sx={stylexStyles.rule17}>
                        {continuation.commandId}
                      </div>
                    )}
                  </div>
                  {continuation.conflict && (
                    <span sx={stylexStyles.rule18}>Conflict</span>
                  )}
                </div>
              )
            })}
            {state.continuations.length === 0 && (
              <div sx={stylexStyles.rule19}>No matching leader commands</div>
            )}
          </div>
        </div>
        {state.conflicts.length > 0 && (
          <div sx={stylexStyles.rule20}>
            Conflict:{' '}
            {state.conflicts.map((conflict) => conflict.key).join(', ')}
          </div>
        )}
      </section>
    </>
  )
}

function formatResolvedKey(key: string): string {
  return key
    .split('+')
    .map((part) => {
      switch (part) {
        case 'ctrl':
          return 'Ctrl'
        case 'meta':
          return 'Cmd'
        case 'shift':
          return 'Shift'
        case 'alt':
          return 'Alt'
        case 'space':
          return 'Space'
        default:
          return part.length === 1 ? part.toUpperCase() : part
      }
    })
    .join('+')
}
