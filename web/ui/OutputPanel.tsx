import type { StyleXStyles } from '@stylexjs/stylex'
import type { ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  root: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: themeColors['--color-ui-outline'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-secondary'],
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  errorMessage: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-error'],
    fontWeight: themeFonts['--font-weight-medium'],
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  empty: { color: themeColors['--color-foreground-alt'] },
  success: { color: themeColors['--color-success'] },
  error: { color: themeColors['--color-error'] },
  warning: { color: themeColors['--color-warning'] },
  running: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': null,
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  failure: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-error'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  plain: {
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
  },
  line: {
    '--tw-leading': 'var(--leading-relaxed)',
    lineHeight: 'var(--leading-relaxed)',
  },
})

// OutputLineRule defines a coloring rule for output lines.
export interface OutputLineRule {
  // match is either a prefix string to check or a function.
  match: string | ((line: string) => boolean)
  // sx supplies the styles for a matching line.
  sx: StyleXStyles
  className?: never
}

// OutputPanelProps are the props for the OutputPanel component.
export interface OutputPanelProps {
  className?: never
  // lines is the array of output lines to display.
  lines: string[]
  // rules is an optional array of line coloring rules.
  // Rules are evaluated in order; first match wins.
  rules?: OutputLineRule[]
  // placeholder is shown when lines is empty and there's no error.
  placeholder?: ReactNode
  // error is an optional error message to display at the top.
  error?: string | null
  // sx overrides the container styles.
  sx?: StyleXStyles
}

const defaultRules: OutputLineRule[] = [
  { match: 'PASS', sx: styles.success },
  { match: 'FAIL', sx: styles.error },
  { match: '[stderr]', sx: styles.warning },
  { match: 'Running:', sx: styles.running },
  { match: 'Error:', sx: styles.failure },
]

// OutputPanel displays scrollable log output with syntax-highlighted lines.
export function OutputPanel({
  lines,
  rules = defaultRules,
  placeholder = 'No output',
  error,
  sx,
}: OutputPanelProps) {
  // Find the first matching rule without rebuilding or copying its styles.
  const getLineStyles = (line: string): StyleXStyles => {
    for (const rule of rules) {
      const matches =
        typeof rule.match === 'string'
          ? line.startsWith(rule.match)
          : rule.match(line)
      if (matches) {
        return rule.sx
      }
    }
    return styles.plain
  }

  return (
    <div sx={[styles.root, sx]}>
      {error && <div sx={styles.errorMessage}>Error: {error}</div>}
      {lines.length === 0 && !error && (
        <div sx={styles.empty}>{placeholder}</div>
      )}
      {lines.map((line) => (
        <div key={line} sx={[styles.line, getLineStyles(line)]}>
          {line}
        </div>
      ))}
    </div>
  )
}
