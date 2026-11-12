// Development React boots a document on the development build of React and
// React DOM instead of the release's production build, for readable errors and
// component stacks. ReactDev.tla specifies the states and their invariants.
//
// Two phases. While the app runs, fillReactDevGeneration copies the
// development files of the release's React version into the service worker's
// cache as one generation and records it in the device setting. At the next
// boot the shell's inline script reads that setting and binds React to
// /b/react-dev/<version>/<production path> instead of the production path.
// The service worker answers those URLs through ReactDevPicks: from the
// generation, read whole on the document's first request, or with the
// production files when the generation is incomplete or gone. It decides once
// per document, so React and React DOM never come from different builds.

// reactDevPathPrefix prefixes every development React URL.
export const reactDevPathPrefix = '/b/react-dev/'

// reactDevCacheName is the service worker cache holding the generation.
export const reactDevCacheName = 'bldr-react-dev'

// reactDevStorageKey is the localStorage key of the device setting. The shell's
// inline script reads it synchronously before any module loads.
export const reactDevStorageKey = 'bldr-react-dev'

// pkgsDir is the directory a release serves its web packages from. A
// generation keys each file by its path below it, so one generation serves
// every release that pins the same React version.
const pkgsDir = '/pkgs/'

// markerName names the entry that marks a generation complete. The fill
// writes it after every file, listing them.
const markerName = '.complete'

// ReactDevSetting is the device setting for Development React.
export interface ReactDevSetting {
  // enabled is whether the person turned Development React on.
  enabled: boolean
  // version is the React version of the complete generation in the cache,
  // or empty when none is.
  version: string
}

// readReactDevSetting reads the device setting, treating a missing or
// unreadable value as off.
export function readReactDevSetting(storage: Storage): ReactDevSetting {
  try {
    const value: unknown = JSON.parse(
      storage.getItem(reactDevStorageKey) ?? 'null',
    )
    if (value && typeof value === 'object') {
      const setting = value as Partial<ReactDevSetting>
      return {
        enabled: setting.enabled === true,
        version: typeof setting.version === 'string' ? setting.version : '',
      }
    }
  } catch {
    // An unreadable setting is off.
  }
  return { enabled: false, version: '' }
}

// writeReactDevSetting stores the device setting.
export function writeReactDevSetting(
  storage: Storage,
  setting: ReactDevSetting,
): void {
  storage.setItem(reactDevStorageKey, JSON.stringify(setting))
}

// ReactDevPath is a parsed development React URL path.
export interface ReactDevPath {
  // version is the React version the shell bound.
  version: string
  // prodPath is the production file the URL stands in for.
  prodPath: string
  // file is the path below the release's package directory, which keys the
  // file within a generation.
  file: string
}

// parseReactDevPath parses /b/react-dev/<version>/<production path>.
export function parseReactDevPath(path: string): ReactDevPath | null {
  if (!path.startsWith(reactDevPathPrefix)) {
    return null
  }
  const rest = path.slice(reactDevPathPrefix.length)
  const slash = rest.indexOf('/')
  if (slash <= 0) {
    return null
  }
  const prodPath = rest.slice(slash)
  const pkgs = prodPath.indexOf(pkgsDir)
  if (pkgs < 0) {
    return null
  }
  return {
    version: decodeURIComponent(rest.slice(0, slash)),
    prodPath,
    file: prodPath.slice(pkgs + pkgsDir.length),
  }
}

// generationURL returns the cache key of a file in a generation.
function generationURL(origin: string, version: string, file: string): string {
  return new URL(
    reactDevPathPrefix + encodeURIComponent(version) + '/' + file,
    origin,
  ).href
}

// ReactDevFile is one development file to copy into a generation.
export interface ReactDevFile {
  // file is the path below the package directory, such as react/index.mjs.
  file: string
  // url is where the development build serves it.
  url: string
}

// fillReactDevGeneration copies the development files of one React version
// into the cache and marks the generation complete once every file is in
// place. A failure leaves no complete generation of that version and keeps any
// other. Success removes the generations of other versions.
export async function fillReactDevGeneration(
  storage: CacheStorage,
  origin: string,
  version: string,
  files: readonly ReactDevFile[],
  fetchFile: (url: string) => Promise<Response> = (url) => fetch(url),
): Promise<void> {
  const cache = await storage.open(reactDevCacheName)
  const marker = generationURL(origin, version, markerName)
  await cache.delete(marker)
  for (const { file, url } of files) {
    const response = await fetchFile(url)
    if (!response.ok) {
      throw new Error(`development React ${file}: HTTP ${response.status}`)
    }
    await cache.put(
      generationURL(origin, version, file),
      new Response(await response.blob(), {
        headers: { 'Content-Type': 'text/javascript' },
      }),
    )
  }
  await cache.put(
    marker,
    new Response(JSON.stringify(files.map(({ file }) => file)), {
      headers: { 'Content-Type': 'application/json' },
    }),
  )

  // Drop the generations of other versions.
  const keep = generationURL(origin, version, '')
  for (const request of await cache.keys()) {
    if (!request.url.startsWith(keep)) {
      await cache.delete(request)
    }
  }
}

// ReactDevGeneration is a complete generation read into memory, by file.
export type ReactDevGeneration = ReadonlyMap<string, Blob>

// readReactDevGeneration reads the complete generation of a version, or
// returns null when the cache holds no complete generation of it.
export async function readReactDevGeneration(
  storage: CacheStorage,
  origin: string,
  version: string,
): Promise<ReactDevGeneration | null> {
  const cache = await storage.open(reactDevCacheName)
  const marker = await cache.match(generationURL(origin, version, markerName))
  if (!marker) {
    return null
  }
  const files: unknown = await marker.json()
  if (!Array.isArray(files)) {
    return null
  }
  const generation = new Map<string, Blob>()
  for (const file of files) {
    if (typeof file !== 'string') {
      return null
    }
    const response = await cache.match(generationURL(origin, version, file))
    if (!response) {
      return null
    }
    generation.set(file, await response.blob())
  }
  return generation
}

// ReactDevPicks holds the service worker's decision for each document: the
// generation it serves development React from, or null for production. The
// first request of a document decides and every later one reuses it, so a
// generation that disappears mid-boot cannot split React from React DOM.
export class ReactDevPicks {
  private readonly picks = new Map<string, Promise<ReactDevGeneration | null>>()

  public constructor(
    private readonly read: (
      version: string,
    ) => Promise<ReactDevGeneration | null>,
  ) {}

  // pick returns the decision for a document and version.
  public pick(
    clientId: string,
    version: string,
  ): Promise<ReactDevGeneration | null> {
    const key = clientId + '\n' + version
    let pick = this.picks.get(key)
    if (!pick) {
      pick = this.read(version).catch(() => null)
      this.picks.set(key, pick)
    }
    return pick
  }

  // retain drops the decisions of documents that are gone.
  public retain(clientIds: ReadonlySet<string>): void {
    for (const key of this.picks.keys()) {
      if (!clientIds.has(key.slice(0, key.indexOf('\n')))) {
        this.picks.delete(key)
      }
    }
  }

  // size returns the number of held decisions.
  public get size(): number {
    return this.picks.size
  }
}

// serveReactDev answers a development React request for a document: the file
// from the document's generation, or the production file when the document
// runs production React.
export async function serveReactDev(
  picks: ReactDevPicks,
  clientId: string,
  path: ReactDevPath,
  fetchProd: (prodPath: string) => Promise<Response>,
): Promise<Response> {
  const generation = await picks.pick(clientId, path.version)
  if (!generation) {
    return fetchProd(path.prodPath)
  }
  const body = generation.get(path.file)
  if (!body) {
    return new Response('development React file not found: ' + path.file, {
      status: 404,
    })
  }
  return new Response(body, {
    headers: { 'Content-Type': 'text/javascript' },
  })
}
