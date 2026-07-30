import { isStaticRoute } from './static-routes.js'
export interface AppPathNavigation {
  path: string
  params: Record<string, string>
}

function parseNamedParams(raw: string): Record<string, string> {
  const params: Record<string, string> = {}
  const queryIndex = raw.indexOf('?')
  if (queryIndex < 0) return params
  for (const [key, value] of new URLSearchParams(raw.slice(queryIndex + 1))) {
    params[key] = value
  }
  return params
}

function formatNamedParams(params: Record<string, string>): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) query.set(key, value)
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

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
    pathname.startsWith('/checkout/') ||
    pathname === '/display' ||
    pathname.startsWith('/display/')
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

export function getAppNavigation(): AppPathNavigation {
  const rawHash = window.location.hash.slice(1)
  if (rawHash) {
    const queryIndex = rawHash.indexOf('?')
    const rawPath = queryIndex < 0 ? rawHash : rawHash.slice(0, queryIndex)
    return {
      path: normalizeAppPath(rawPath),
      params: parseNamedParams(rawHash),
    }
  }
  const pathname = window.location.pathname
  return {
    path:
      isStaticRoute(pathname) || isPathnameAppRoute(pathname)
        ? normalizeAppPath(pathname)
        : '/',
    params: {},
  }
}

// getAppPath returns the current app path, checking hash first
// then falling back to pathname for static routes.
export function getAppPath(): string {
  return getAppNavigation().path
}

let navigationGeneration = 0
let navigationGenerationTracked = false
let observedLocation = ''

function currentLocationKey(): string {
  return `${window.location.pathname}${window.location.search}${window.location.hash}`
}

// observeNavigation advances the counter when the document location differs
// from the one last observed. Reading the location is what makes a direct
// `window.location.hash` write countable: that write moves the document
// synchronously and only queues `hashchange`, so a counter driven by the
// event alone still reads stale inside the same task.
function observeNavigation(): void {
  const key = currentLocationKey()
  if (key === observedLocation) return
  observedLocation = key
  navigationGeneration++
}

function trackNavigationGeneration(): void {
  if (navigationGenerationTracked) return
  navigationGenerationTracked = true
  observedLocation = currentLocationKey()
  window.addEventListener('hashchange', observeNavigation)
  window.addEventListener('popstate', observeNavigation)
}

// getAppNavigationGeneration returns a counter that advances on every
// document navigation, whoever caused it: history traversal, an in-app
// setAppPath, or an external hash edit. Comparing the counter across an
// awaited boundary answers whether the document moved, which comparing
// paths cannot, since a navigation away and back restores the old string.
export function getAppNavigationGeneration(): number {
  trackNavigationGeneration()
  observeNavigation()
  return navigationGeneration
}

// setAppPath sets the hash to the given path while retaining named parameters
// unless an explicit parameter set is provided.
export function setAppPath(
  path: string,
  params?: Record<string, string>,
): void {
  const normalized = normalizeAppPath(path)
  const retainedParams = params ?? getAppNavigation().params
  const nextHash = `${normalized}${formatNamedParams(retainedParams)}`
  const currentHash = window.location.hash.startsWith('#')
    ? window.location.hash.slice(1)
    : window.location.hash
  if (currentHash === nextHash) return
  // replaceState fires neither hashchange nor popstate, so the move is
  // observed here rather than left to the listeners.
  trackNavigationGeneration()
  if (window.location.search) {
    window.history.replaceState(
      {},
      '',
      `${window.location.pathname}${window.location.search}#${nextHash}`,
    )
    observeNavigation()
    return
  }
  if (window.location.pathname !== '/') {
    window.history.replaceState({}, '', `/#${nextHash}`)
    observeNavigation()
    return
  }
  window.location.hash = nextHash
  observeNavigation()
}
