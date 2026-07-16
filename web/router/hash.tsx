import { useEffect, useState, useCallback, useRef } from 'react'
import { resolvePath, To } from './router.js'
import { isPathnameAppRoute, normalizeAppPath } from './app-path.js'
import { isStaticRoute } from './static-routes.js'

export interface HashNavigation {
  path: string
  params: Record<string, string>
}

export function parseHashNavigation(rawHash: string): HashNavigation {
  const raw = rawHash.startsWith('#') ? rawHash.slice(1) : rawHash
  const queryIndex = raw.indexOf('?')
  const rawPath = queryIndex < 0 ? raw : raw.slice(0, queryIndex)
  const params: Record<string, string> = {}
  if (queryIndex >= 0) {
    for (const [key, value] of new URLSearchParams(raw.slice(queryIndex + 1))) {
      params[key] = value
    }
  }
  return { path: normalizeAppPath(rawPath), params }
}

export function formatHashNavigation(navigation: HashNavigation): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(navigation.params)) {
    query.set(key, value)
  }
  const encoded = query.toString()
  return `${navigation.path}${encoded ? `?${encoded}` : ''}`
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
    (to: To) => {
      const path = resolvePath(currentPathRef.current, to)
      setPath(
        to.params ? formatHashNavigation({ path, params: to.params }) : path,
      )
    },
    [setPath],
  )
}

export function useHashNavigation(): [
  HashNavigation,
  (navigation: HashNavigation) => void,
] {
  const [navigation, setNavigation] = useState<HashNavigation>(() => {
    const hash = window.location.hash.slice(1)
    if (hash) return parseHashNavigation(hash)
    const pathname = window.location.pathname
    return {
      path:
        isStaticRoute(pathname) || isPathnameAppRoute(pathname)
          ? normalizeAppPath(pathname)
          : '/',
      params: {},
    }
  })

  useEffect(() => {
    const handleHashChange = () => {
      const next = parseHashNavigation(window.location.hash)
      setNavigation((previous) =>
        previous.path === next.path &&
        JSON.stringify(previous.params) === JSON.stringify(next.params)
          ? previous
          : next,
      )
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  const updateNavigation = useCallback((next: HashNavigation) => {
    const normalizedPath = next.path.startsWith('/')
      ? next.path
      : `/${next.path}`
    const nextHash = formatHashNavigation({
      path: normalizedPath,
      params: next.params,
    })
    const currentHash = window.location.hash.startsWith('#')
      ? window.location.hash.slice(1)
      : window.location.hash
    if (currentHash !== nextHash) window.location.hash = nextHash
  }, [])

  return [navigation, updateNavigation]
}

export function useHashPath(): [string, (path: string) => void] {
  const [navigation, updateNavigation] = useHashNavigation()
  const updatePath = useCallback(
    (path: string) => {
      const parsed = path.includes('?') ? parseHashNavigation(path) : null
      updateNavigation(parsed ?? { path, params: navigation.params })
    },
    [navigation.params, updateNavigation],
  )
  return [navigation.path, updatePath]
}
