// BrowserReleaseShellAssets lists the shell assets for one browser release.
export interface BrowserReleaseShellAssets {
  entrypoint: string
  serviceWorker: string
  sharedWorker: string
  wasm: string
  css: string[]
}

// BrowserReleaseDescriptor defines one browser generation.
export interface BrowserReleaseDescriptor {
  schemaVersion: number
  generationId: string
  shellAssets: BrowserReleaseShellAssets
  prerenderedRoutes: string[]
  requiredStaticAssets: string[]
}

// BrowserReleaseState stores the ServiceWorker-owned release progression.
export interface BrowserReleaseState {
  schemaVersion: number
  discovered: BrowserReleaseDescriptor | null
  staged: BrowserReleaseDescriptor | null
  promotedCurrent: BrowserReleaseDescriptor | null
  promotedPrevious: BrowserReleaseDescriptor | null
}

// BROWSER_RELEASE_STATE_SCHEMA_VERSION is the cache-state schema version.
export const BROWSER_RELEASE_STATE_SCHEMA_VERSION = 1

// isBrowserCacheSupportedURL checks if the Cache API accepts requests for a URL.
export function isBrowserCacheSupportedURL(url: string | URL): boolean {
  const parsed = typeof url === 'string' ? new URL(url) : url
  return parsed.protocol === 'http:' || parsed.protocol === 'https:'
}

// createEmptyBrowserReleaseState builds the empty ServiceWorker release state.
export function createEmptyBrowserReleaseState(): BrowserReleaseState {
  return {
    schemaVersion: BROWSER_RELEASE_STATE_SCHEMA_VERSION,
    discovered: null,
    staged: null,
    promotedCurrent: null,
    promotedPrevious: null,
  }
}

// normalizeReleasePath canonicalizes a release path for cache lookup.
export function normalizeReleasePath(path: string): string {
  if (!path) {
    return ''
  }
  if (path === '/') {
    return path
  }
  let normalized = path
  if (!normalized.startsWith('/')) {
    normalized = `/${normalized}`
  }
  if (normalized.length > 1 && normalized.endsWith('/')) {
    normalized = normalized.slice(0, -1)
  }
  return normalized
}

// sameBrowserRelease checks if the two descriptors point at the same generation.
export function sameBrowserRelease(
  a: BrowserReleaseDescriptor | null | undefined,
  b: BrowserReleaseDescriptor | null | undefined,
): boolean {
  if (!a || !b) {
    return false
  }
  return a.generationId === b.generationId
}

// buildReleaseCachePaths returns the cache paths that define one release.
export function buildReleaseCachePaths(
  release: BrowserReleaseDescriptor,
): string[] {
  const paths = new Set<string>()
  const addPath = (path: string) => {
    const normalized = normalizeReleasePath(path)
    if (normalized) {
      paths.add(normalized)
    }
  }

  addPath(release.shellAssets.entrypoint)
  addPath(release.shellAssets.serviceWorker)
  addPath(release.shellAssets.sharedWorker)
  addPath(release.shellAssets.wasm)
  for (const path of release.shellAssets.css) {
    addPath(path)
  }
  for (const path of release.requiredStaticAssets) {
    addPath(path)
  }
  for (const path of release.prerenderedRoutes) {
    addPath(path)
  }

  return Array.from(paths).sort()
}

// buildOfflineNavigationFallbacks selects the cache keys for offline HTML boot.
export function buildOfflineNavigationFallbacks(
  pathname: string,
  release: BrowserReleaseDescriptor,
): string[] {
  const normalized = normalizeReleasePath(pathname)
  const routes = new Set(
    release.prerenderedRoutes.map((route) => normalizeReleasePath(route)),
  )
  if (routes.has(normalized)) {
    return [normalized]
  }
  if (routes.has('/')) {
    return ['/']
  }
  return []
}

// promoteBrowserRelease advances the state to a newly promoted release.
export function promoteBrowserRelease(
  state: BrowserReleaseState,
  release: BrowserReleaseDescriptor,
): BrowserReleaseState {
  if (sameBrowserRelease(state.promotedCurrent, release)) {
    return {
      ...state,
      discovered: release,
      staged: release,
      promotedCurrent: release,
    }
  }
  return {
    ...state,
    discovered: release,
    staged: release,
    promotedCurrent: release,
    promotedPrevious:
      sameBrowserRelease(state.promotedCurrent, state.promotedPrevious) ?
        state.promotedPrevious
      : state.promotedCurrent,
  }
}

// retainedGenerationIds returns the promoted generations that must stay cached.
export function retainedGenerationIds(state: BrowserReleaseState): string[] {
  const ids = new Set<string>()
  const addId = (release: BrowserReleaseDescriptor | null) => {
    const id = release?.generationId
    if (id) {
      ids.add(id)
    }
  }
  addId(state.promotedCurrent)
  addId(state.promotedPrevious)
  return Array.from(ids)
}
