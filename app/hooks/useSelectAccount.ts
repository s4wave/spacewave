import { useCallback } from 'react'

import { useNavigate } from '@s4wave/web/router/router.js'

// useSelectAccount returns the shared account-selection navigation action.
export function useSelectAccount(): (sessionIndex: number) => void {
  const navigate = useNavigate()

  return useCallback(
    (sessionIndex: number) => {
      navigate({ path: `/u/${sessionIndex}/` })
    },
    [navigate],
  )
}
