import { useEffect, useState, useCallback, useRef } from 'react'
import { resolvePath, To } from './router.js'
import { normalizeAppPath } from './app-path.js'
import { isStaticRoute } from './static-routes.js'

function isPathnameAppRoute(pathname: string): boolean {
  return pathname === '/recover'
}

/**
 * Creates a memoized navigation handler function
 * @param currentPath - The current router path
 * @param setPath - Function to set the path
 * @returns A memoized function compatible with Router's onNavigate prop
 */
export const useNavigateHandler = (
  currentPath: string,
  setPath: (path: string) => void,
): ((to: To) => void) => {
  const currentPathRef = useRef(currentPath)
  // eslint-disable-next-line react-hooks/refs
  currentPathRef.current = currentPath

  return useCallback(
    (to: To) => setPath(resolvePath(currentPathRef.current, to)),
    [setPath],
  )
}

export function useHashPath(): [string, (path: string) => void] {
  // Initialize with current hash path, removing the # if present.
  // When no hash exists, fall back to pathname for static routes and select
  // app routes that may arrive without a hash fragment.
  // Query parameters are stripped so route matching works correctly;
  // components that need query params can read window.location.hash directly.
  const [path, setPath] = useState(() => {
    const hash = window.location.hash.slice(1)
    if (hash) {
      return normalizeAppPath(hash)
    }
    const pathname = window.location.pathname
    if (isStaticRoute(pathname) || isPathnameAppRoute(pathname)) {
      return normalizeAppPath(pathname)
    }
    return '/'
  })

  useEffect(() => {
    // Handler for hash changes
    const handleHashChange = () => {
      const raw =
        window.location.hash.startsWith('#') ?
          window.location.hash.slice(1)
        : window.location.hash
      setPath(normalizeAppPath(raw))
    }

    // Listen for hash changes
    window.addEventListener('hashchange', handleHashChange)

    // Clean up listener
    return () => {
      window.removeEventListener('hashchange', handleHashChange)
    }
  }, [])

  // Function to update the hash path
  const updatePath = useCallback((newPath: string) => {
    // eslint-disable-next-line react-compiler/react-compiler
    window.location.hash = newPath.startsWith('/') ? newPath : `/${newPath}`
  }, [])

  return [normalizeAppPath(path), updatePath]
}
