import { type ReactNode, useCallback, useMemo } from 'react'

import {
  ViewerBar,
  type ViewerBarAction,
} from '@s4wave/web/object/ViewerBar.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/index.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { StateBadge } from './StateBadge.js'

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
  /** Global actions for the viewer bar (for example, object creation). */
  headerActions?: ViewerBarAction[]
  headerStatus?: ReactNode
  /** Stable object/viewer identity used to persist tab selection. */
  stateKey?: string
  children?: ReactNode
}

/**
 * ForgeViewerShell owns Forge navigation, action bars, and touch target sizing.
 * Its viewer bar carries the title, state and header actions; the bottom bar
 * carries the actions on the current object or selection.
 */
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
      className="bg-background-primary flex h-full w-full flex-col overflow-hidden pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11"
    >
      <ViewerBar
        icon={icon}
        title={title}
        state={
          stateLabels && state !== undefined ? (
            <StateBadge state={state} labels={stateLabels} />
          ) : undefined
        }
        actions={headerActions}
      />
      {headerStatus}

      {/* Tab bar */}
      {tabs && tabs.length > 1 && (
        <div className="border-foreground/8 shrink-0 overflow-x-auto border-b px-3 py-1.5 sm:px-4">
          <div className="mx-auto w-full max-w-5xl">
            <div className="bg-foreground/5 inline-flex min-w-max gap-1 rounded-md p-1">
              {tabs.map((tab) => (
                <button
                  type="button"
                  key={tab.id}
                  onClick={() => onTabClick(tab.id)}
                  className={cn(
                    'rounded border px-3 text-xs font-medium transition-all duration-150 select-none sm:px-2.5 sm:py-1',
                    resolvedTab?.id === tab.id
                      ? 'border-brand/30 bg-brand/10 text-foreground'
                      : 'text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5 border-transparent',
                  )}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Content */}
      <div className="flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto w-full max-w-5xl">
          {resolvedTab ? resolvedTab.content : children}
        </div>
      </div>

      {/* Action bar: reserved for actions on the current object/selection. */}
      {actions && actions.length > 0 && (
        <div
          data-testid="forge-viewer-action-bar"
          className="border-foreground/8 flex min-h-14 shrink-0 flex-wrap items-center gap-2 border-t px-3 py-2 sm:flex-nowrap sm:px-4 sm:pointer-fine:h-10 sm:pointer-fine:min-h-0 sm:pointer-fine:py-0"
        >
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
