import { type ReactNode, useCallback, useMemo } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { StateBadge } from './StateBadge.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/index.js'

export interface ForgeViewerTab {
  id: string
  label: string
  content: ReactNode
}

export interface ForgeAction {
  label: string
  icon?: ReactNode
  onClick: () => void
  variant?: 'default' | 'destructive'
  disabled?: boolean
}

interface ForgeViewerShellProps {
  icon: ReactNode
  title: string
  state?: number
  stateLabels?: Record<number, string>
  tabs?: ForgeViewerTab[]
  actions?: ForgeAction[]
  children?: ReactNode
}

// ForgeViewerShell provides shared chrome for all Forge viewers:
// header (icon + name + state badge), tabbed content, and bottom action bar.
export function ForgeViewerShell({
  icon,
  title,
  state,
  stateLabels,
  tabs,
  actions,
  children,
}: ForgeViewerShellProps) {
  const ns = useStateNamespace(['forge-viewer'])
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

  return (
    <div
      data-testid="forge-viewer"
      className="bg-background-primary flex h-full w-full flex-col overflow-hidden"
    >
      {/* Header */}
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold select-none">
          {icon}
          <span className="tracking-tight">{title}</span>
          {stateLabels && state !== undefined && (
            <StateBadge state={state} labels={stateLabels} />
          )}
        </div>
      </div>

      {/* Tab bar */}
      {tabs && tabs.length > 1 && (
        <div className="border-foreground/8 flex shrink-0 items-center border-b px-3 py-1.5">
          <div className="bg-foreground/5 inline-flex gap-1 rounded-md p-1">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => onTabClick(tab.id)}
                className={cn(
                  'rounded px-2.5 py-1 text-xs font-medium transition-colors',
                  resolvedTab?.id === tab.id ?
                    'bg-brand/10 text-foreground border-brand/20 border'
                  : 'text-foreground-alt/60 hover:text-foreground-alt/90 border border-transparent',
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Content */}
      <div className="flex-1 overflow-auto px-4 py-3">
        {resolvedTab ? resolvedTab.content : children}
      </div>

      {/* Action bar */}
      {actions && actions.length > 0 && (
        <div className="border-foreground/8 flex h-10 shrink-0 items-center gap-2 border-t px-4">
          {actions.map((action) => (
            <button
              key={action.label}
              onClick={action.onClick}
              disabled={action.disabled}
              className={cn(
                'flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium transition-colors',
                'disabled:pointer-events-none disabled:opacity-40',
                action.variant === 'destructive' ?
                  'text-red-400 hover:bg-red-500/10'
                : 'text-foreground/80 hover:bg-foreground/5',
              )}
            >
              {action.icon}
              {action.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
