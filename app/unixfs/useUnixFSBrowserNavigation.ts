import { useCallback } from 'react'

import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import {
  getUnixFSParentPath,
  joinUnixFSDisplayPath,
} from '@s4wave/sdk/unixfs/path.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { useTabContext } from '@s4wave/web/object/TabContext.js'
import type {
  ObjectInfo,
  UnixfsObjectInfo,
} from '@s4wave/web/object/object.pb.js'
import {
  localNavigation,
  useHistory,
} from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import type { UnixFSBrowserDispatch } from './useUnixFSBrowserState.js'

interface UnixFSBrowserNavigationOptions {
  unixfsId: string
  displayPath: string
  dispatch: UnixFSBrowserDispatch
}

/**
 * useUnixFSBrowserNavigation returns the history, parent, path, and open
 * handlers. Each clears the pending entry name before it navigates.
 */
export function useUnixFSBrowserNavigation({
  unixfsId,
  displayPath,
  dispatch,
}: UnixFSBrowserNavigationOptions) {
  const tabContext = useTabContext()
  const history = useHistory()
  const navigate = useNavigate()
  const canGoUp = displayPath !== '/'

  const handleBack = useCallback(() => {
    dispatch({ type: 'set-pending-name', name: null })
    history?.goBack()
  }, [dispatch, history])

  const handleForward = useCallback(() => {
    dispatch({ type: 'set-pending-name', name: null })
    history?.goForward()
  }, [dispatch, history])

  const handleUp = useCallback(() => {
    if (!canGoUp) return
    dispatch({ type: 'set-pending-name', name: null })
    navigate(localNavigation({ path: getUnixFSParentPath(displayPath) }))
  }, [canGoUp, dispatch, displayPath, navigate])

  // handlePathChange follows a path the user edited in the toolbar.
  const handlePathChange = useCallback(
    (newPath: string) => {
      dispatch({ type: 'set-pending-name', name: null })
      navigate(localNavigation({ path: newPath }))
    },
    [dispatch, navigate],
  )

  // handleOpen opens one entry in this tab, or several entries in new tabs.
  const handleOpen = useCallback(
    (entries: FileEntry[]) => {
      if (!entries.length) return

      if (entries.length === 1) {
        const entry = entries[0]
        dispatch({ type: 'set-pending-name', name: entry.id })
        navigate({ path: './' + entry.name })
        return
      }

      if (!tabContext) return
      for (const entry of entries) {
        const objectInfo: ObjectInfo = {
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId,
              path: joinUnixFSDisplayPath(displayPath, entry.name),
            } satisfies UnixfsObjectInfo,
          },
        }
        void tabContext.addTab({
          tab: {
            id: `tab-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
            name: entry.name,
            enableClose: true,
            data: ObjectLayoutTab.toBinary({ objectInfo, path: '' }),
          },
          select: entry === entries[0],
        })
      }
    },
    [dispatch, displayPath, navigate, tabContext, unixfsId],
  )

  return {
    canGoUp,
    canGoBack: history?.canGoBack ?? false,
    canGoForward: history?.canGoForward ?? false,
    handleBack,
    handleForward,
    handleUp,
    handlePathChange,
    handleOpen,
  }
}
