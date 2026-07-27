import { ShellAppPanel } from './ShellAppPanel.js'
import { useIsGridMode } from './ShellContext.js'

// ShellTabContentProps are the props for ShellTabContent.
export interface ShellTabContentProps {
  tabId: string
  path: string
}

// ShellTabContent renders the content for a single tab panel in the FlexLayout.
// Each tab reuses the shared shell app panel; grid mode disables only URL syncing.
//
// Grid mode comes from the shell context, not from a prop. The layout keeps this
// element mounted across the mode transition, so a value sampled when the
// content was first created would describe the mode the tab was born in: a tab
// created in normal mode would keep writing its own path over the grid URL, and
// a tab opened from a grid deep link would stop synchronizing once the split
// collapsed.
export function ShellTabContent({ tabId, path }: ShellTabContentProps) {
  const isGridMode = useIsGridMode()
  return (
    <ShellAppPanel
      tabId={tabId}
      initialPath={path}
      namespace={['shell-tab', tabId]}
      syncAppPath={!isGridMode}
    />
  )
}
