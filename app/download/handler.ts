import { useCallback } from 'react'

import { useNavigate } from '@s4wave/web/router/router.js'

// useDownloadDesktopApp returns a callback that navigates to the /download
// landing page.
export function useDownloadDesktopApp(): () => void {
  const navigate = useNavigate()
  return useCallback(() => navigate({ path: '/download' }), [navigate])
}
