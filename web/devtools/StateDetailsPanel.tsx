import { useCallback, useMemo } from 'react'

import { Button } from '@s4wave/web/ui/button.js'

import {
  useSelectedStatePath,
  useStateDevToolsContext,
} from './StateDevToolsContext.js'
import {
  useStateInspectorValue,
  type StateInspectorEntry,
} from './useStateInspectorEntries.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 64)`,
    flexShrink: 0,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'auto',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
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
    color: themeColors['--color-foreground'],
  },
  rule7: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    overflow: 'hidden',
  },
  rule8: {
    flex: 1,
    overflow: 'auto',
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-background-deep'],
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
    whiteSpace: 'pre-wrap',
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

interface StateDetailsPanelProps {
  entry: StateInspectorEntry
}

// StateDetailsPanel displays detailed information about a selected state atom.
export function StateDetailsPanel({ entry }: StateDetailsPanelProps) {
  const devtools = useStateDevToolsContext()
  const selectedPath = useSelectedStatePath()
  const value = useStateInspectorValue(entry)

  const handleCopy = useCallback(() => {
    const targetValue = getValueAtPath(value, selectedPath)
    void navigator.clipboard.writeText(JSON.stringify(targetValue, null, 2))
  }, [selectedPath, value])

  const handleClose = useCallback(() => {
    devtools?.setSelectedAtomId(null)
  }, [devtools])

  const displayValue = useMemo(() => {
    const targetValue = getValueAtPath(value, selectedPath)
    return JSON.stringify(targetValue, null, 2)
  }, [selectedPath, value])

  const pathDisplay =
    selectedPath.length > 0 ? selectedPath.join('.') : '(root)'

  return (
    <div sx={[stylexStyles.rule1]} data-testid="state-details-panel">
      <div sx={stylexStyles.rule2}>
        <span sx={stylexStyles2.rule1}>{entry.label}</span>
        <button type="button" onClick={handleClose} sx={stylexStyles.rule3}>
          Close
        </button>
      </div>

      <div sx={stylexStyles.rule4}>
        <span sx={stylexStyles.rule5}>Path:</span>
        <span sx={stylexStyles.rule6}>{pathDisplay}</span>
      </div>

      <div sx={stylexStyles.rule4}>
        <span sx={stylexStyles.rule5}>Scope:</span>
        <span sx={stylexStyles.rule6}>{entry.scope}</span>
      </div>

      <div sx={stylexStyles.rule7}>
        <span sx={stylexStyles.rule5}>Value:</span>
        <pre sx={[stylexStyles.rule8]}>{displayValue}</pre>
      </div>

      <div sx={stylexStyles.rule9}>
        <Button size="sm" variant="outline" onClick={handleCopy}>
          Copy
        </Button>
      </div>
    </div>
  )
}

function getValueAtPath(obj: unknown, path: string[]): unknown {
  if (path.length === 0) return obj
  if (obj === null || typeof obj !== 'object') return undefined

  let current: unknown = obj
  for (const key of path) {
    if (current === null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[key]
  }
  return current
}
