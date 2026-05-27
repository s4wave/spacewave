const staticRoutes = new Set([
  '/',
  '/landing',
  '/landing/drive',
  '/landing/chat',
  '/landing/devices',
  '/landing/plugins',
  '/landing/notes',
  '/landing/cli',
  '/landing/hydra',
  '/landing/bifrost',
  '/landing/controllerbus',
  '/tos',
  '/privacy',
  '/pricing',
  '/dmca',
  '/community',
  '/licenses',
  '/download',
  '/download/cli',
  '/blog',
])

function isStaticRoute(pathname: string): boolean {
  if (staticRoutes.has(pathname)) return true
  if (pathname.startsWith('/blog/')) return true
  if (pathname.startsWith('/landing/')) return true
  if (pathname.startsWith('/quickstart/')) return true
  return false
}

function isPathnameAppRoute(pathname: string): boolean {
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

function normalizeAppPath(path: string): string {
  const stripped = stripQueryParams(stripHashPrefix(path))
  if (!stripped) return '/'
  const normalized = stripped.startsWith('/') ? stripped : '/' + stripped
  return decodePath(normalized)
}

function getAppPath(): string {
  const hash = window.location.hash.slice(1)
  if (hash) return normalizeAppPath(hash)
  const pathname = window.location.pathname
  if (isStaticRoute(pathname) || isPathnameAppRoute(pathname)) {
    return normalizeAppPath(pathname)
  }
  return '/'
}

export function setAppPath(path: string): void {
  const normalized = normalizeAppPath(path)
  if (
    window.location.pathname === '/' &&
    !window.location.search &&
    getAppPath() === normalized
  ) {
    return
  }
  if (window.location.search) {
    window.history.replaceState(
      {},
      '',
      `${window.location.pathname}${window.location.search}#${normalized}`,
    )
    return
  }
  if (window.location.pathname !== '/') {
    window.history.replaceState({}, '', `/#${normalized}`)
    return
  }
  window.location.hash = normalized
}
