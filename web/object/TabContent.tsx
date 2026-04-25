import { useCallback, useMemo, type ReactNode } from 'react'
import { WebViewErrorBoundary } from '@aptre/bldr-react'
import {
  ObjectLayoutTab,
  type ObjectLayoutTab as ObjectLayoutTabType,
} from '@s4wave/sdk/layout/world/world.pb.js'
import type {
  AddTabRequest,
  AddTabResponse,
} from '@s4wave/sdk/layout/layout.pb.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { resolvePath, type To } from '@s4wave/web/router/router.js'
import { TabContextProvider, type TabContextValue } from './TabContext.js'
import { ObjectViewer } from './ObjectViewer.js'

// NavigateFunc is the function signature for navigating to a new path.
export type NavigateFunc = (path: string) => void | Promise<unknown>

// AddTabFunc is the function signature for adding a new tab.
export type AddTabFunc = (request: AddTabRequest) => Promise<AddTabResponse>

// TabContentProps are the props passed to the TabContent component.
export interface TabContentProps {
  // tabID is the unique identifier for the tab.
  tabID: string
  // tabData is the serialized ObjectLayoutTab message.
  tabData?: Uint8Array
  // navigate is the function to navigate to a new path within the tab.
  navigate: NavigateFunc
  // addTab is the function to add a new tab to the layout.
  addTab: AddTabFunc
}

// decodeTabData decodes the tab data into an ObjectLayoutTab message.
function decodeTabData(data?: Uint8Array): ObjectLayoutTabType | null {
  if (!data || data.length === 0) {
    return null
  }
  try {
    return ObjectLayoutTab.fromBinary(data)
  } catch {
    return null
  }
}

// TabContent renders the content of a layout tab based on the ObjectLayoutTab data.
export function TabContent({
  tabID,
  tabData,
  navigate,
  addTab,
}: TabContentProps) {
  const layoutTab = useMemo(() => decodeTabData(tabData), [tabData])
  const spaceContext = SpaceContainerContext.useContextSafe()
  const sessionIndex = useSessionIndex()
  const worldState = spaceContext?.spaceWorldResource ?? null

  const exportUrl = useMemo(
    () =>
      sessionIndex != null && spaceContext?.spaceId ?
        `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(spaceContext.spaceId)}`
      : undefined,
    [sessionIndex, spaceContext?.spaceId],
  )

  const tabPath = layoutTab?.path ?? ''

  const handleNavigate = useCallback(
    (to: To) => {
      const resolvedPath = resolvePath(tabPath || '/', to)
      Promise.resolve(navigate(resolvedPath)).catch((err) => {
        console.warn('TabContent: navigation failed:', err)
      })
    },
    [navigate, tabPath],
  )

  const navigateTab = useCallback(
    (path: string) => Promise.resolve(navigate(path)).then(() => ({})),
    [navigate],
  )

  const tabContext = useMemo<TabContextValue>(
    () => ({ tabId: tabID, addTab, navigateTab }),
    [tabID, addTab, navigateTab],
  )

  let content: ReactNode
  if (!layoutTab) {
    content = (
      <div className="text-muted-foreground flex h-full w-full items-center justify-center">
        <div className="text-ui">Empty tab: {tabID}</div>
      </div>
    )
  } else {
    const objectInfo = layoutTab.objectInfo
    if (!objectInfo?.info?.case || !worldState) {
      content = (
        <div className="text-muted-foreground flex h-full w-full items-center justify-center">
          <div className="text-ui">
            {!worldState ? 'No world state available' : `Tab: ${tabID}`}
            {layoutTab.componentId && (
              <span className="ml-2">Component: {layoutTab.componentId}</span>
            )}
          </div>
        </div>
      )
    } else {
      content = (
        <ObjectViewer
          objectInfo={objectInfo}
          worldState={worldState}
          standalone
          bottomBarId={`tab-${tabID}`}
          path={tabPath || '/'}
          exportUrl={exportUrl}
          onNavigate={handleNavigate}
          stateNamespace={['tab', tabID]}
        />
      )
    }
  }

  return <TabContextProvider value={tabContext}>{content}</TabContextProvider>
}

// TabContentContainer wraps TabContent with an error boundary.
export function TabContentContainer(props: TabContentProps) {
  return (
    <WebViewErrorBoundary>
      <TabContent {...props} />
    </WebViewErrorBoundary>
  )
}
