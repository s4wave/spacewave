import { createHash } from 'crypto'

// BrowserReleaseShellAssets identifies the hashed shell asset URLs for one
// browser release generation.
export interface BrowserReleaseShellAssets {
  entrypoint: string
  serviceWorker: string
  sharedWorker: string
  wasm: string
  css: string[]
}

// BrowserReleaseDescriptor defines one browser generation and the guaranteed
// prerendered route inventory attached to it.
export interface BrowserReleaseDescriptor {
  schemaVersion: 1
  generationId: string
  shellAssets: BrowserReleaseShellAssets
  prerenderedRoutes: string[]
  requiredStaticAssets: string[]
}

// buildBrowserReleaseDescriptor builds a deterministic browser release
// descriptor for the current shell assets and prerendered route inventory.
export function buildBrowserReleaseDescriptor(
  shellAssets: BrowserReleaseShellAssets,
  prerenderedRoutes: string[],
  requiredStaticAssets: string[],
): BrowserReleaseDescriptor {
  const routes = [...new Set(prerenderedRoutes)].sort((a, b) =>
    a.localeCompare(b),
  )
  const assets = [...new Set(requiredStaticAssets)].sort((a, b) =>
    a.localeCompare(b),
  )
  const generationId = createHash('sha256')
    .update(
      JSON.stringify({
        shellAssets,
        prerenderedRoutes: routes,
        requiredStaticAssets: assets,
      }),
    )
    .digest('hex')
    .slice(0, 16)

  return {
    schemaVersion: 1,
    generationId,
    shellAssets,
    prerenderedRoutes: routes,
    requiredStaticAssets: assets,
  }
}
