import { isStaticRoute } from './static-routes.js'

function isPathnameAppRoute(pathname: string): boolean {
  return pathname === '/recover'
}

function stripQueryParams(path: string): string {
  const idx = path.indexOf('?')
  if (idx === -1) return path
  return path.slice(0, idx)
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
  const stripped = stripQueryParams(path)
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

// setAppPath sets the hash to the given path. If on a pathname-based
// static route, the first setAppPath call transitions to hash routing.
export function setAppPath(path: string): void {
  window.location.hash = path
}
