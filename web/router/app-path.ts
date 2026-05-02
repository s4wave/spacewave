import { isStaticRoute } from './static-routes.js'

export function isPathnameAppRoute(pathname: string): boolean {
  return (
    pathname === '/login' ||
    pathname === '/signup' ||
    pathname === '/sessions' ||
    pathname === '/recover' ||
    pathname === '/pair' ||
    pathname.startsWith('/pair/') ||
    pathname === '/join' ||
    pathname.startsWith('/join/') ||
    pathname.startsWith('/auth/') ||
    pathname.startsWith('/checkout/')
  )
}

function stripQueryParams(path: string): string {
  const idx = path.indexOf('?')
  if (idx === -1) return path
  return path.slice(0, idx)
}

function stripHashPrefix(path: string): string {
  return path.startsWith('#') ? path.slice(1) : path
}

function decodePath(path: string): string {
  try {
    return decodeURIComponent(path)
  } catch {
    return path
  }
}

// normalizeAppPath returns the decoded app route path for a raw hash/pathname.
export function normalizeAppPath(path: string): string {
  const stripped = stripQueryParams(stripHashPrefix(path))
  if (!stripped) return '/'
  const normalized = stripped.startsWith('/') ? stripped : '/' + stripped
  return decodePath(normalized)
}

// getAppPath returns the current app path, checking hash first
// then falling back to pathname for static routes.
export function getAppPath(): string {
  const hash = window.location.hash.slice(1)
  if (hash) return normalizeAppPath(hash)
  const pathname = window.location.pathname
  if (isStaticRoute(pathname) || isPathnameAppRoute(pathname)) {
    return normalizeAppPath(pathname)
  }
  return '/'
}

// setAppPath sets the hash to the given path. If on a pathname-based static
// route, the first setAppPath call transitions to the canonical root hash URL.
export function setAppPath(path: string): void {
  const normalized = normalizeAppPath(path)
  if (window.location.pathname !== '/' || window.location.search) {
    window.history.replaceState({}, '', `/#${normalized}`)
    return
  }
  window.location.hash = normalized
}
