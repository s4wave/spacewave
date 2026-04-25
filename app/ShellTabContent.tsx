import { ShellAppPanel } from './ShellAppPanel.js'

// ShellTabContentProps are the props for ShellTabContent.
export interface ShellTabContentProps {
  tabId: string
  path: string
}

// ShellTabContent renders the content for a single tab panel in the FlexLayout.
// Each tab reuses the shared shell app panel with URL syncing enabled.
export function ShellTabContent({ tabId, path }: ShellTabContentProps) {
  return (
    <ShellAppPanel
      tabId={tabId}
      initialPath={path}
      namespace={['shell-tab', tabId]}
      syncAppPath
    />
  )
}
