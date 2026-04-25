// STATIC_ROUTES is the set of routes that can be served as
// pre-rendered HTML at their pathname (not hash).
export const STATIC_ROUTES = new Set([
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
  '/blog',
])

// isStaticRoute returns whether the given pathname is a static route.
export function isStaticRoute(pathname: string): boolean {
  if (STATIC_ROUTES.has(pathname)) return true
  // Blog post and tag pages are also static routes.
  if (pathname.startsWith('/blog/')) return true
  // Landing sub-pages are also static routes.
  if (pathname.startsWith('/landing/')) return true
  // Quickstart loading pages are static routes.
  if (pathname.startsWith('/quickstart/')) return true
  return false
}
