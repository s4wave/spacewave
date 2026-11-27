import { joinPath } from '@aptre/bldr'
import { useMemo, type ReactNode } from 'react'

import type { AddTabRequest } from '@s4wave/sdk/layout/layout.pb.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { SUBPATH_DELIMITER } from '@s4wave/sdk/space/object-uri.js'
import {
  joinUnixFSDisplayPath,
  normalizeUnixFSLookupPath,
} from '@s4wave/sdk/unixfs/path.js'
import {
  TabContextProvider,
  useTabContext,
} from '@s4wave/web/object/TabContext.js'

// buildObjectRoutePath returns the shell path of the object objectKey in the
// Space at parentPaths.
export function buildObjectRoutePath(
  parentPaths: readonly string[],
  objectKey: string,
): string {
  return joinPath([...parentPaths, SUBPATH_DELIMITER, objectKey], true)
}

// spaceTabViewerPath returns the shell path of the object layout tab at
// parentPaths, or undefined when the tab specifies no object in the Space.
// A world object opens at its route with the tab's viewer path under it. A
// UnixFS object opens at the route of its UnixFS with the file path under it,
// the same path the UnixFS viewer reads from the router below its base path.
function spaceTabViewerPath(
  layoutTab: ObjectLayoutTab,
  parentPaths: readonly string[],
): string | undefined {
  const info = layoutTab.objectInfo?.info
  if (info?.case === 'worldObjectInfo' && info.value.objectKey) {
    const objectPath = buildObjectRoutePath(parentPaths, info.value.objectKey)
    return layoutTab.path
      ? objectPath + SUBPATH_DELIMITER + layoutTab.path
      : objectPath
  }
  if (info?.case === 'unixfsObjectInfo' && info.value.unixfsId) {
    const objectPath = buildObjectRoutePath(parentPaths, info.value.unixfsId)
    const filePath = normalizeUnixFSLookupPath(
      joinUnixFSDisplayPath(info.value.path || '/', layoutTab.path || '/'),
    )
    return filePath ? objectPath + SUBPATH_DELIMITER + filePath : objectPath
  }
  return undefined
}

// spaceAddTabRequest rewrites a request for an object layout tab of a world
// or UnixFS object into the shell path of that object in the Space at
// parentPaths, with the tab's viewer path under it. Other requests pass
// through.
export function spaceAddTabRequest(
  request: AddTabRequest,
  parentPaths: readonly string[],
): AddTabRequest {
  const data = request.tab?.data
  if (!request.tab || !data?.length) return request
  const path = spaceTabViewerPath(ObjectLayoutTab.fromBinary(data), parentPaths)
  if (path === undefined) return request
  return {
    ...request,
    tab: { ...request.tab, data: ObjectLayoutTab.toBinary({ path }) },
  }
}

// SpaceTabContextProviderProps configures SpaceTabContextProvider.
interface SpaceTabContextProviderProps {
  parentPaths: readonly string[]
  children?: ReactNode
}

// SpaceTabContextProvider gives the viewers of a Space shown in a shell tab
// the shell tab's operations, with addTab opening a world or UnixFS object's
// tab at the object's route in this Space, so a viewer opens object tabs the same
// way in a shell tab as in an object layout.
export function SpaceTabContextProvider({
  parentPaths,
  children,
}: SpaceTabContextProviderProps) {
  const outer = useTabContext()
  const value = useMemo(
    () =>
      outer && !outer.isObjectLayout
        ? {
            ...outer,
            addTab: (request: AddTabRequest) =>
              outer.addTab(spaceAddTabRequest(request, parentPaths)),
          }
        : outer,
    [outer, parentPaths],
  )
  return <TabContextProvider value={value}>{children}</TabContextProvider>
}
