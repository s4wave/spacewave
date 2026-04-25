import { useMemo, useCallback } from 'react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { ILocalState, cloneLocalState } from '@s4wave/sdk/layout/layout.js'
import { LayoutHostHandle } from '@s4wave/sdk/layout/layout-host.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import {
  BaseLayout,
  ITabComponentProps,
} from '@s4wave/web/layout/BaseLayout.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state'

import type { ObjectViewerComponentProps } from './object.js'
import { getObjectKey } from './object.js'
import { TabContentContainer } from './TabContent.js'
import { buildObjectLayoutExternalDrag } from './layout-object-app-drag.js'

// ObjectLayoutTypeID is the type identifier for ObjectLayout objects.
export const ObjectLayoutTypeID = 'alpha/object-layout'

// LayoutObjectViewer renders an ObjectLayout world object using BaseLayout.
export function LayoutObjectViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  // Create namespace for persisting local state
  const namespace = useStateNamespace(['layout', objectKey])

  // Persist local layout state (selected tabs, active tabset, maximized tab)
  const [localState, setLocalState] = useStateAtom<ILocalState>(
    namespace,
    'localState',
    { tabSetSelected: {} },
  )

  // Access the typed object resource for this layout
  const typedObjectResource = useAccessTypedHandle(
    worldState,
    objectKey,
    LayoutHostHandle,
    ObjectLayoutTypeID,
  )

  const layoutHost = useResourceValue(typedObjectResource)

  // Handle local state changes
  const handleLocalStateChange = useCallback(
    (nextState: ILocalState) => {
      setLocalState(cloneLocalState(nextState))
    },
    [setLocalState],
  )

  // Memoize local state to avoid unnecessary re-renders
  const memoizedLocalState = useMemo(
    () => cloneLocalState(localState),
    [localState],
  )

  // Render tab content
  const renderTab = useCallback(
    ({ tabID, tabData, navigate, addTab }: ITabComponentProps) => {
      return (
        <TabContentContainer
          tabID={tabID}
          tabData={tabData}
          navigate={navigate}
          addTab={addTab}
        />
      )
    },
    [],
  )

  const handleExternalDrag = useCallback(
    (event: Parameters<typeof buildObjectLayoutExternalDrag>[0]) =>
      buildObjectLayoutExternalDrag(event),
    [],
  )

  if (layoutHost === null) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <LoadingCard
          view={{ state: 'loading', title: 'Loading layout' }}
          className="w-full max-w-sm"
        />
      </div>
    )
  }

  if (layoutHost === undefined) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center">
        Failed to load layout
      </div>
    )
  }

  return (
    <div className="space-flexlayout bg-foreground/6 relative flex h-full w-full flex-col gap-1 overflow-hidden text-xs">
      <div className="relative flex h-full w-full flex-1 flex-col">
        <BaseLayout
          layoutHost={layoutHost}
          renderTab={renderTab}
          flexLayoutProps={{ onExternalDrag: handleExternalDrag }}
          localState={memoizedLocalState}
          onLocalStateChange={handleLocalStateChange}
        />
      </div>
    </div>
  )
}
