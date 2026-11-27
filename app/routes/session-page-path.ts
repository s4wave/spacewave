// SESSION_PAGE_PREFIXES lists informational route families available inside a
// session.
export const SESSION_PAGE_PREFIXES = [
  'legal',
  'docs',
  'blog',
  'changelog',
  'community',
  'pricing',
  'download',
  'landing',
]

const legalPages = new Set(['tos', 'privacy', 'dmca', 'licenses'])

// sessionPagePath keeps informational destinations in the current session.
// Authentication, quickstarts, external URLs, and already scoped paths retain
// their destinations.
export function sessionPagePath(path: string, sessionIndex: number): string {
  if (!sessionIndex || !path.startsWith('/') || path.startsWith('//')) {
    return path
  }

  const prefix = path.slice(1).split(/[/?#]/, 1)[0]
  const base = `/u/${sessionIndex}`
  if (!prefix) return base + path.slice(1)
  if (legalPages.has(prefix)) return `${base}/legal${path}`
  if (SESSION_PAGE_PREFIXES.includes(prefix)) return base + path
  return path
}
