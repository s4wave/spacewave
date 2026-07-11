import { useCallback } from 'react'

import { setAppPath } from '@s4wave/web/router/app-path.js'

// useSelectAccount returns the shared account-selection navigation action.
export function useSelectAccount(): (sessionIndex: number) => void {
  return useCallback((sessionIndex: number) => {
    setAppPath(`/u/${sessionIndex}/`)
  }, [])
}
