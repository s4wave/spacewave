import { useCallback } from 'react'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate } from '@s4wave/web/router/router.js'

// useLandingBackNavigation prefers tab-local history and falls back to landing.
export function useLandingBackNavigation() {
  const history = useHistory()
  const navigate = useNavigate()

  return useCallback(() => {
    if (history?.canGoBack) {
      history.goBack()
      return
    }
    navigate({ path: '/' })
  }, [history, navigate])
}
