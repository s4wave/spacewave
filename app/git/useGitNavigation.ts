import { useCallback } from 'react'

import type { useNavigate } from '@s4wave/web/router/router.js'
import type { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

import type { RouteInfo, ViewMode } from './layout/route.js'

// useGitNavigation encapsulates the shared toolbar navigation callbacks
// for git viewers: back, forward, up, ref select, mode change, path change,
// and file open. Parameterized by route state, effectiveRef, and an optional
// workdir path for dual-mode worktree browsing.
export function useGitNavigation(opts: {
  route: RouteInfo
  effectiveRef: string | null
  displayPath: string
  navigate: ReturnType<typeof useNavigate>
  history: ReturnType<typeof useHistory>
  workdirPath?: string
  onPendingName?: (entryId: string | null) => void
}) {
  const {
    route,
    effectiveRef,
    displayPath,
    navigate,
    history,
    workdirPath,
    onPendingName,
  } = opts

  const buildTreePath = useCallback((ref: string, subpath?: string) => {
    const clean = subpath && subpath !== '/' ? subpath.replace(/^\//, '') : ''
    return clean ? '/tree/' + ref + '/' + clean : '/tree/' + ref
  }, [])

  const handleBack = useCallback(() => {
    onPendingName?.(null)
    history?.goBack()
  }, [history, onPendingName])

  const handleForward = useCallback(() => {
    onPendingName?.(null)
    history?.goForward()
  }, [history, onPendingName])

  const handleUp = useCallback(() => {
    if (route.mode === 'workdir' && workdirPath !== undefined) {
      if (workdirPath === '/') return
      const parent = workdirPath.replace(/\/[^/]+\/?$/, '') || '/'
      const clean = parent === '/' ? '' : parent.replace(/^\//, '')
      onPendingName?.(null)
      navigate({ path: clean ? '/workdir/' + clean : '/workdir' })
      return
    }
    if (displayPath === '/' || !effectiveRef) return
    const parent = displayPath.replace(/\/[^/]+\/?$/, '') || '/'
    onPendingName?.(null)
    navigate({ path: buildTreePath(effectiveRef, parent) })
  }, [
    route.mode,
    workdirPath,
    displayPath,
    effectiveRef,
    navigate,
    buildTreePath,
    onPendingName,
  ])

  const handlePathChange = useCallback(
    (newPath: string) => {
      if (route.mode === 'workdir') {
        const clean = newPath === '/' ? '' : newPath.replace(/^\//, '')
        onPendingName?.(null)
        navigate({ path: clean ? '/workdir/' + clean : '/workdir' })
        return
      }
      if (!effectiveRef) return
      onPendingName?.(null)
      navigate({ path: buildTreePath(effectiveRef, newPath) })
    },
    [route.mode, effectiveRef, navigate, buildTreePath, onPendingName],
  )

  const handleOpen = useCallback(
    (entries: FileEntry[]) => {
      if (!entries.length) return
      const entry = entries[0]

      if (route.mode === 'workdir' && workdirPath !== undefined) {
        const sub =
          workdirPath === '/' ?
            entry.name
          : workdirPath.replace(/^\//, '') + '/' + entry.name
        onPendingName?.(entry.id)
        navigate({ path: '/workdir/' + sub })
        return
      }

      if (!effectiveRef) return
      const sub =
        displayPath === '/' ?
          entry.name
        : displayPath.replace(/^\//, '') + '/' + entry.name
      onPendingName?.(entry.id)
      navigate({ path: '/tree/' + effectiveRef + '/' + sub })
    },
    [
      route.mode,
      effectiveRef,
      displayPath,
      workdirPath,
      navigate,
      onPendingName,
    ],
  )

  const handleRefSelect = useCallback(
    (refName: string) => {
      onPendingName?.(null)
      if (route.mode === 'log') {
        navigate({ path: '/commits/' + refName })
      } else if (route.mode === 'readme') {
        navigate({ path: '/readme/' + refName })
      } else {
        navigate({ path: '/tree/' + refName })
      }
    },
    [route.mode, navigate, onPendingName],
  )

  const handleModeChange = useCallback(
    (mode: ViewMode) => {
      const ref = effectiveRef
      onPendingName?.(null)
      if (mode === 'files') {
        navigate({ path: ref ? '/tree/' + ref : '/' })
      } else if (mode === 'readme') {
        navigate({ path: ref ? '/readme/' + ref : '/' })
      } else if (mode === 'log') {
        navigate({ path: ref ? '/commits/' + ref : '/' })
      } else if (mode === 'workdir') {
        navigate({ path: '/workdir' })
      } else if (mode === 'changes') {
        navigate({ path: '/changes' })
      }
    },
    [effectiveRef, navigate, onPendingName],
  )

  return {
    buildTreePath,
    handleBack,
    handleForward,
    handleUp,
    handlePathChange,
    handleOpen,
    handleRefSelect,
    handleModeChange,
  }
}
