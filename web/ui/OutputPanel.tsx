import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type { StyleXStyles } from '@stylexjs/stylex'
import type { ReactNode } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors2,
  layout as themeLayout2,
  fonts as themeFonts2,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    overflow: 'auto',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors2['--color-ui-outline'],
    backgroundColor: themeColors2['--color-background-secondary'],
    padding: `calc(${themeLayout2['--spacing']} * 2)`,
    fontFamily: themeFonts2['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts2['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts2['--text-xs--line-height']})`,
    color: themeColors2['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-error'],
  },
  rule2: { color: themeColors['--color-foreground-alt'] },
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
  // className is an optional CSS class for the container.
  sx?: ComponentStyles
}

const lineStyles = stylex.create({
  success: { color: themeColors['--color-success'] },
  error: { color: themeColors['--color-error'] },
  warning: { color: themeColors['--color-warning'] },
  running: {
    '--tw-leading': null,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  failure: {
    color: themeColors['--color-error'],
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  plain: {
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
  line: {
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
  },
})

const defaultRules: OutputLineRule[] = [
  { match: 'PASS', sx: lineStyles.success },
  { match: 'FAIL', sx: lineStyles.error },
  { match: '[stderr]', sx: lineStyles.warning },
  { match: 'Running:', sx: lineStyles.running },
  { match: 'Error:', sx: lineStyles.failure },
]

// OutputPanel displays scrollable log output with syntax-highlighted lines.
export function OutputPanel({
  lines,
  rules = defaultRules,
  placeholder = 'No output',
  error,
  sx,
}: Omit<OutputPanelProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
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
    return lineStyles.plain
  }

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      {error && <div sx={stylexStyles.rule1}>Error: {error}</div>}
      {lines.length === 0 && !error && (
        <div sx={stylexStyles.rule2}>{placeholder}</div>
      )}
      {lines.map((line) => (
        <div key={line} sx={[lineStyles.line, getLineStyles(line)]}>
          {line}
        </div>
      ))}
    </div>
  )
}
