import { useCallback, useMemo } from 'react'
import { RiErrorWarningLine } from '@s4wave/web/images/icons.js'

import {
  useResourceDevToolsContext,
  useTrackedResources,
  useSelectedResourceId,
  getResourceLabel,
  type TrackedResource,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 30%, transparent)`,
    backgroundColor: themeColors['--color-ui-selected'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    textAlign: 'left',
    color: themeColors['--color-text-secondary'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-pulldown-hover']} 50%, transparent)`,
      },
    },
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    opacity: 0.5,
  },
  rule3: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    overflow: 'auto',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    flexShrink: 0,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-error'],
  },
  rule5: {
    flexShrink: 0,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule6: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground-alt'],
  },
})

// ResourceErrorsTab displays a list of resources with errors.
export function ResourceErrorsTab() {
  const resources = useTrackedResources()

  const errorResources = useMemo(() => {
    return Array.from(resources.values()).filter((r) => r.state === 'error')
  }, [resources])

  if (errorResources.length === 0) {
    return (
      <div sx={stylexStyles.rule1}>
        <RiErrorWarningLine sx={stylexStyles.rule2} />
        <span>No errors</span>
      </div>
    )
  }

  return (
    <div sx={stylexStyles.rule3}>
      {errorResources.map((resource) => (
        <ErrorRow key={resource.id} resource={resource} />
      ))}
    </div>
  )
}

interface ErrorRowProps {
  resource: TrackedResource
}

function ErrorRow({ resource }: ErrorRowProps) {
  const devtools = useResourceDevToolsContext()
  const selectedId = useSelectedResourceId()
  const isSelected = selectedId === resource.id

  const handleResourceSelect = useCallback(() => {
    devtools?.setSelectedId(resource.id)
  }, [devtools, resource.id])

  const label = getResourceLabel(resource)

  const errorMessage = useMemo(() => {
    if (!resource.error) return ''
    const msg = resource.error.message
    const firstLine = msg.split('\n')[0]
    return firstLine.length > 50 ? firstLine.slice(0, 50) + '...' : firstLine
  }, [resource.error])

  return (
    <button
      type="button"
      onClick={handleResourceSelect}
      sx={isSelected ? stylexStyles2.rule1 : stylexStyles2.rule2}
      data-testid="resource-error-row"
    >
      <span sx={stylexStyles.rule4} />
      <span sx={stylexStyles.rule5}>{label}</span>
      <span sx={stylexStyles.rule6}>{errorMessage}</span>
    </button>
  )
}
