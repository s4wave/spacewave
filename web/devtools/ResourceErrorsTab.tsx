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
    '--tw-duration': '.1s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 30%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-ui-selected'],
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    transitionDuration: '.1s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    '--tw-duration': '.1s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 30%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-pulldown-hover']} 50%, transparent)`,
      },
    },
    color: themeColors['--color-text-secondary'],
    display: 'flex',
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    textAlign: 'left',
    transitionDuration: '.1s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    opacity: 0.5,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    flex: 1,
    overflow: 'auto',
    display: 'flex',
    flexDirection: 'column',
  },
  rule4: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-error'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    flexShrink: 0,
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
})

// ResourceErrorsTab displays a list of resources with errors.
export function ResourceErrorsTab({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const resources = useTrackedResources()

  const errorResources = useMemo(() => {
    return Array.from(resources.values()).filter((r) => r.state === 'error')
  }, [resources])

  if (errorResources.length === 0) {
    return (
      <div sx={[stylexStyles.rule1, sx]}>
        <RiErrorWarningLine sx={stylexStyles.rule2} />
        <span>No errors</span>
      </div>
    )
  }

  return (
    <div sx={[stylexStyles.rule3, sx]}>
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
