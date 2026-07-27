import { ShellAppPanel } from './ShellAppPanel.js'

// ShellTabContentProps are the props for ShellTabContent.
export interface ShellTabContentProps {
  tabId: string
  path: string
  syncAppPath?: boolean
}

// ShellTabContent renders the content for a single tab panel in the FlexLayout.
// Each tab reuses the shared shell app panel; grid mode disables only URL syncing.
export function ShellTabContent({
  tabId,
  path,
  syncAppPath = true,
}: ShellTabContentProps) {
  return (
    <ShellAppPanel
      tabId={tabId}
      initialPath={path}
      namespace={['shell-tab', tabId]}
      syncAppPath={syncAppPath}
    />
  )
}
