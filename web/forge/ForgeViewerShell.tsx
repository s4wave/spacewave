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

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': 0,
    },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: { default: null, '@media (min-width: 40rem)': 'center' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
    height: {
      default: null,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 9)`,
      },
    },
  },
  rule2: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule4: {
    display: 'flex',
    width: { default: '100%', '@media (min-width: 40rem)': 'auto' },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    marginLeft: {
      default: null,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 3)`,
    },
    flexShrink: { default: null, '@media (min-width: 40rem)': 0 },
  },
  rule5: {
    flex: { default: 1, '@media (min-width: 40rem)': 'none' },
    justifyContent: 'center',
  },
  rule6: {
    flexShrink: 0,
    overflowX: 'auto',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule7: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
  },
  rule8: {
    display: 'inline-flex',
    minWidth: 'max-content',
    gap: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    padding: themeLayout['--spacing'],
  },
  rule9: {
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 2.5)`,
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    paddingBlock: {
      default: null,
      '@media (min-width: 40rem)': themeLayout['--spacing'],
    },
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule11: {
    borderColor: '#0000',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule12: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule13: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    flexShrink: 0,
    flexWrap: { default: 'wrap', '@media (min-width: 40rem)': 'nowrap' },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    height: {
      default: null,
      '@media (min-width: 40rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 10)`,
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
    <div
      data-testid="forge-viewer"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: pointer-coarse:[&_button]:min-h-11 */ 'bg-background-primary flex h-full w-full flex-col overflow-hidden pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11'
      }
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
                    stylexStyles.rule9,
                    resolvedTab?.id === tab.id
                      ? stylexStyles.rule10
                      : stylexStyles.rule11,
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
  )
}
