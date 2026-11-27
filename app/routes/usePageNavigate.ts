import { useCallback } from 'react'

import { useSessionIndex } from '@s4wave/web/contexts/SessionIndexContext.js'
import { useNavigate, type To } from '@s4wave/web/router/router.js'

import { sessionPagePath } from './session-page-path.js'

// usePageNavigate opens informational pages within the current session when a
// session is present.
export function usePageNavigate(): (to: To) => void {
  const navigate = useNavigate()
  const sessionIndex = useSessionIndex()
  return useCallback(
    (to: To) =>
      navigate({ ...to, path: sessionPagePath(to.path, sessionIndex) }),
    [navigate, sessionIndex],
  )
}
