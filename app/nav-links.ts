import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useDownloadDesktopApp } from '@s4wave/app/download/handler.js'

// useNavLinks returns shared navigation callbacks for links used across landing and session pages.
export function useNavLinks() {
  const navigate = useNavigate()

  return {
    download: useDownloadDesktopApp(),
    docs: useCallback(() => navigate({ path: '/docs' }), [navigate]),
    blog: useCallback(() => navigate({ path: '/blog' }), [navigate]),
    changelog: useCallback(() => navigate({ path: '/changelog' }), [navigate]),
    legal: useCallback(() => navigate({ path: '/tos' }), [navigate]),
    support: useCallback(() => navigate({ path: '/community' }), [navigate]),
    cloud: useCallback(() => navigate({ path: '/pricing' }), [navigate]),
    getStarted: useCallback(() => navigate({ path: '/sessions' }), [navigate]),
  }
}
