import { useMemo } from 'react'

import { useShellTabs } from './ShellTabContext.js'
import { ShellAppPanel } from './ShellAppPanel.js'

// ShellGridPanelProps are the props for ShellGridPanel.
export interface ShellGridPanelProps {
  tabId: string
}

// ShellGridPanel renders the content for a single grid panel.
// Grid panels reuse the same shared app panel as normal shell tabs.
export function ShellGridPanel({ tabId }: ShellGridPanelProps) {
  const { tabs } = useShellTabs()
  const tab = useMemo(() => tabs.find((t) => t.id === tabId), [tabs, tabId])
  const path = tab?.path ?? '/'

  return (
    <ShellAppPanel
      tabId={tabId}
      initialPath={path}
      namespace={['shell-grid-panel', tabId]}
    />
  )
}
