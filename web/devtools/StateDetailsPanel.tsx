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
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: themeColors['--color-popover-border'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'auto',
    display: 'flex',
    flexDirection: 'column',
    flexShrink: 0,
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    width: `calc(${themeLayout['--spacing']} * 64)`,
  },
  rule2: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule7: {
    flex: 1,
    gap: themeLayout['--spacing'],
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
  },
  rule8: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    flex: 1,
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-deep'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'pre-wrap',
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

interface StateDetailsPanelProps {
  entry: StateInspectorEntry
}

// StateDetailsPanel displays detailed information about a selected state atom.
export function StateDetailsPanel({
  sx,
  entry,
}: StateDetailsPanelProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
    <div sx={[[stylexStyles.rule1], sx]} data-testid="state-details-panel">
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
