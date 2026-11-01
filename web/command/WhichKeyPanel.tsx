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
    pointerEvents: 'none',
    position: 'fixed',
    inset: 0,
    zIndex: 40,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    '--tw-backdrop-blur': 'blur(var(--blur-xs))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule2: {
    pointerEvents: 'auto',
    position: 'fixed',
    right: 0,
    bottom: 0,
    left: 0,
    zIndex: 50,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 95%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    '--tw-shadow': '0 0 #0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
  },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule6: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule8: {
    minWidth: 0,
    flex: 1,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    maxHeight: `calc(${themeLayout['--spacing']} * 72)`,
    overflowY: 'auto',
  },
  rule11: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    borderColor: themeColors['--color-brand'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule13: {
    display: 'flex',
    minWidth: `calc(${themeLayout['--spacing']} * 24)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule14: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
  },
  rule15: { minWidth: 0, flex: 1 },
  rule16: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule17: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule19: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule20: {
    marginInline: 'auto',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    maxWidth: 'var(--container-3xl)',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
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
