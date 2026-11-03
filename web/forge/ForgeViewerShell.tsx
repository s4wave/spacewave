import { ControlStylesProvider } from '@s4wave/web/style/ControlStylesProvider.js'
import {
  forgeControlStyles,
  forgeViewerMarker,
} from '@s4wave/web/style/forge-controls.stylex.js'
import { type ReactNode, useCallback, useMemo } from 'react'

import { useStateNamespace, useStateAtom } from '@s4wave/web/state/index.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { StateBadge } from './StateBadge.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': 0,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    alignItems: { default: null, '@media (min-width: 40rem)': 'center' },
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    flexShrink: 0,
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: {
      default: null,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 9)`,
      },
    },
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
    minWidth: 0,
  },
  rule3: {
    '--tw-tracking': 'var(--tracking-tight)',
    overflow: 'hidden',
    letterSpacing: 'var(--tracking-tight)',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: { default: null, '@media (min-width: 40rem)': 0 },
    marginLeft: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 3)`,
    },
    width: { default: '100%', '@media (min-width: 40rem)': 'auto' },
  },
  rule5: {
    flex: { default: 1, '@media (min-width: 40rem)': 'none' },
    justifyContent: 'center',
  },
  rule6: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    overflowX: 'auto',
  },
  rule7: {
    marginInline: 'auto',
    maxWidth: 'var(--container-5xl)',
    width: '100%',
  },
  rule8: {
    padding: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    gap: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'inline-flex',
    minWidth: 'max-content',
  },
  rule9: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': themeLayout['--spacing'],
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 2.5)`,
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule11: {
    borderColor: '#0000',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule12: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule13: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    flexWrap: { default: 'wrap', '@media (min-width: 40rem)': 'nowrap' },
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    height: {
      default: null,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 10)`,
      },
    },
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
  },
})

/** ForgeViewerTab provides a named content panel within a Forge viewer. */
export interface ForgeViewerTab {
  id: string
  label: string
  content: ReactNode
}

/** ForgeAction describes an action on a Forge viewer or its selected object. */
export interface ForgeAction {
  label: string
  icon?: ReactNode
  onClick: () => void
  variant?: 'default' | 'primary' | 'destructive'
  disabled?: boolean
}

interface ForgeViewerShellProps {
  sx?: stylex.StyleXStyles
  className?: never
  icon: ReactNode
  title: string
  state?: number
  stateLabels?: Record<number, string>
  tabs?: ForgeViewerTab[]
  /** Actions that apply to the current viewer object or selection. */
  actions?: ForgeAction[]
  /** Global actions for the viewer header (for example, object creation). */
  headerActions?: ForgeAction[]
  headerStatus?: ReactNode
  /** Stable object/viewer identity used to persist tab selection. */
  stateKey?: string
  children?: ReactNode
}

/** ForgeViewerShell owns Forge navigation, action bars, and touch target sizing. */
export function ForgeViewerShell({
  icon,
  title,
  state,
  stateLabels,
  tabs,
  actions,
  headerActions,
  headerStatus,
  stateKey,
  children,
  sx,
}: ForgeViewerShellProps) {
  // Keep tab selection in the viewer’s existing personal state namespace.
  const ns = useStateNamespace(['forge-viewer', stateKey ?? title])
  const [activeTab, setActiveTab] = useStateAtom(ns, 'tab', '')

  const resolvedTab = useMemo(() => {
    if (!tabs?.length) return null
    const found = tabs.find((t) => t.id === activeTab)
    return found ?? tabs[0]
  }, [tabs, activeTab])

  const onTabClick = useCallback(
    (id: string) => {
      setActiveTab(id)
    },
    [setActiveTab],
  )

  // Apply the touch floor to viewer-supplied controls as well as shared chrome.
  return (
    <ControlStylesProvider styles={forgeControlStyles.required}>
      <div
        data-testid="forge-viewer"
        sx={[stylexStyles2.rule1, [forgeViewerMarker, sx]]}
      >
        {/* Header */}
        <div sx={stylexStyles.rule1}>
          <div sx={stylexStyles.rule2}>
            {icon}
            <span sx={stylexStyles.rule3}>{title}</span>
            {stateLabels && state !== undefined && (
              <StateBadge state={state} labels={stateLabels} />
            )}
          </div>
          {headerActions && headerActions.length > 0 && (
            <div
              data-testid="forge-viewer-header-actions"
              sx={stylexStyles.rule4}
            >
              {headerActions.map((action) => (
                <Tooltip key={action.label}>
                  <TooltipTrigger asChild>
                    <DashboardButton
                      icon={action.icon}
                      sx={stylexStyles.rule5}
                      onClick={action.onClick}
                      disabled={action.disabled}
                      variant={
                        action.variant === 'primary'
                          ? 'primary'
                          : action.variant === 'destructive'
                            ? 'destructive'
                            : undefined
                      }
                    >
                      {action.label}
                    </DashboardButton>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">{action.label}</TooltipContent>
                </Tooltip>
              ))}
            </div>
          )}
        </div>
        {headerStatus}

        {/* Tab bar */}
        {tabs && tabs.length > 1 && (
          <div sx={stylexStyles.rule6}>
            <div sx={stylexStyles.rule7}>
              <div sx={stylexStyles.rule8}>
                {tabs.map((tab) => (
                  <button
                    type="button"
                    key={tab.id}
                    onClick={() => onTabClick(tab.id)}
                    sx={[
                      [
                        stylexStyles.rule9,
                        resolvedTab?.id === tab.id
                          ? stylexStyles.rule10
                          : stylexStyles.rule11,
                      ],
                      forgeControlStyles.required,
                    ]}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Content */}
        <div sx={stylexStyles.rule12}>
          <div sx={stylexStyles.rule7}>
            {resolvedTab ? resolvedTab.content : children}
          </div>
        </div>

        {/* Action bar: reserved for actions on the current object/selection. */}
        {actions && actions.length > 0 && (
          <div data-testid="forge-viewer-action-bar" sx={stylexStyles.rule13}>
            {actions.map((action) => (
              <Tooltip key={action.label}>
                <TooltipTrigger asChild>
                  <DashboardButton
                    icon={action.icon}
                    onClick={action.onClick}
                    disabled={action.disabled}
                    variant={
                      action.variant === 'primary'
                        ? 'primary'
                        : action.variant === 'destructive'
                          ? 'destructive'
                          : undefined
                    }
                  >
                    {action.label}
                  </DashboardButton>
                </TooltipTrigger>
                <TooltipContent side="top">{action.label}</TooltipContent>
              </Tooltip>
            ))}
          </div>
        )}
      </div>
    </ControlStylesProvider>
  )
}
