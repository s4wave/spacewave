// plugin-host-quickjs.ts runs a JavaScript plugin in the QuickJS WASI reactor.
//
// This module is imported by shared-worker.ts when the worker type is QUICKJS.
// It sets up the QuickJS WASI environment and runs the plugin script inside
// the QuickJS VM with a re-entrant event loop that yields to the browser.
//
// Architecture:
// 1. Fetch QuickJS WASM from /b/qjs/qjs-wasi.wasm
// 2. Fetch boot harness from /b/qjs/plugin-quickjs.esm.js
// 3. Fetch plugin script from scriptPath
// 4. Create WASI environment with stdin/dev-out for yamux
// 5. Call qjs.init(["qjs", "--std", bootHarnessPath]) to initialize and run boot harness
// 6. Run event loop with loopOnce()
//    - Returns >0: setTimeout(loop, ms)
//    - Returns 0: queueMicrotask(loop)
//    - Returns -1: idle, wait for I/O
//    - Returns -2: error
// 7. Yields to browser event loop between iterations

import { StreamConn } from 'starpc'
import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import {
  QuickJS,
  buildFileSystem,
  createReadOnlyMount,
  PollableStdin,
  LOOP_IDLE,
  LOOP_ERROR,
  type Fd,
  type ReadOnlyFileMount,
} from 'quickjs-wasi-reactor'

import { BackendAPI } from '@aptre/bldr-sdk'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

type ViteManifestEntry = {
  file?: string
  imports?: string[]
  dynamicImports?: string[]
  css?: string[]
  assets?: string[]
}

const backendAssetsRoot = 'v/b/be/'

// Cached compiled QuickJS WASM module (shared across plugin restarts)
let cachedWasmModule: WebAssembly.Module | null = null
// Cached boot harness code
let cachedBootHarness: string | null = null

// loadQuickJSModule fetches and compiles the QuickJS WASM module.
async function loadQuickJSModule(): Promise<WebAssembly.Module> {
  if (cachedWasmModule) {
    return cachedWasmModule
  }
  const response = await fetch('/b/qjs/qjs-wasi.wasm')
  if (!response.ok) {
    throw new Error(`Failed to fetch QuickJS WASM: ${response.status}`)
  }
  cachedWasmModule = await WebAssembly.compileStreaming(response)
  return cachedWasmModule
}

// loadBootHarness fetches the boot harness JavaScript code.
async function loadBootHarness(): Promise<string> {
  if (cachedBootHarness) {
    return cachedBootHarness
  }
  const response = await fetch('/b/qjs/plugin-quickjs.esm.js')
  if (!response.ok) {
    throw new Error(`Failed to fetch boot harness: ${response.status}`)
  }
  cachedBootHarness = await response.text()
  return cachedBootHarness
}

// fetchPluginScript fetches the plugin script from the HTTP server.
async function fetchPluginScript(scriptPath: string): Promise<string> {
  const response = await fetch(scriptPath)
  if (!response.ok) {
    throw new Error(
      `Failed to fetch plugin script ${scriptPath}: ${response.status}`,
    )
  }
  return response.text()
}

// collectViteManifestAssetPaths returns the unique asset-relative files referenced by a Vite manifest.
export function collectViteManifestAssetPaths(
  manifest: Record<string, ViteManifestEntry>,
): string[] {
  const paths = new Set<string>()

  const addPath = (path?: string) => {
    if (!path) {
      return
    }
    paths.add(path)
  }

  const visited = new Set<string>()
  const visitRef = (ref: string) => {
    const entry = manifest[ref]
    if (!entry) {
      addPath(ref)
      return
    }
    if (visited.has(ref)) {
      return
    }
    visited.add(ref)

    addPath(entry.file)
    for (const path of entry.css ?? []) {
      addPath(path)
    }
    for (const path of entry.assets ?? []) {
      addPath(path)
    }
    for (const dep of entry.imports ?? []) {
      visitRef(dep)
    }
    for (const dep of entry.dynamicImports ?? []) {
      visitRef(dep)
    }
  }

  for (const ref of Object.keys(manifest)) {
    visitRef(ref)
  }
  return [...paths]
}

// collectViteManifestStaticAssetPaths returns the static asset graph for entrypoint files.
export function collectViteManifestStaticAssetPaths(
  manifest: Record<string, ViteManifestEntry>,
  entryAssetPaths: string[],
): string[] {
  const entryByResolvedFile = new Map<string, string>()
  for (const [ref, entry] of Object.entries(manifest)) {
    if (entry.file) {
      entryByResolvedFile.set(resolveBackendAssetPath(entry.file), ref)
    }
  }

  const paths = new Set<string>()
  const addPath = (path?: string) => {
    if (!path) {
      return
    }
    paths.add(path)
  }

  const visited = new Set<string>()
  const visitRef = (ref: string) => {
    const entry = manifest[ref]
    if (!entry) {
      addPath(ref)
      return
    }
    if (visited.has(ref)) {
      return
    }
    visited.add(ref)

    addPath(entry.file)
    for (const path of entry.css ?? []) {
      addPath(path)
    }
    for (const path of entry.assets ?? []) {
      addPath(path)
    }
    for (const dep of entry.imports ?? []) {
      visitRef(dep)
    }
  }

  for (const entryPath of entryAssetPaths) {
    const resolvedPath = resolveBackendAssetPath(entryPath)
    const ref = entryByResolvedFile.get(resolvedPath)
    if (ref) {
      visitRef(ref)
      continue
    }
    addPath(resolvedPath)
  }
  return [...paths]
}

// resolveBackendAssetPath normalizes a backend asset path to the v/b/be asset tree.
export function resolveBackendAssetPath(path: string): string {
  const trimmed = path.replace(/^\/+/, '')
  if (!trimmed) {
    return ''
  }
  if (trimmed.startsWith('v/')) {
    return trimmed
  }
  if (trimmed.startsWith('assets/')) {
    return resolveBackendAssetPath(trimmed.slice('assets/'.length))
  }
  if (trimmed.startsWith('b/')) {
    return 'v/' + trimmed
  }
  return backendAssetsRoot + trimmed
}

// addAssetToFileSystem mirrors an asset under both its asset-relative path and /assets/.
export function addAssetToFileSystem(
  files: Map<string, string | Uint8Array>,
  assetPath: string,
  content: string | Uint8Array,
): void {
  const resolvedPath = resolveBackendAssetPath(assetPath)
  if (!resolvedPath) {
    return
  }
  files.set(resolvedPath, content)
  files.set('/assets/' + resolvedPath, content)
}

type BackendAssetCacheEntry =
  | { ok: true; data: Uint8Array }
  | { ok: false; status: number; url: string }

export type BackendAssetLoadingMode = 'lazy-http' | 'bounded-preload'

type BackendAssetAPI = {
  startInfo: Pick<BackendAPI['startInfo'], 'pluginId'>
  utils: Pick<BackendAPI['utils'], 'pluginAssetHttpPath'>
}

// canUseSynchronousBackendAssetFetch returns true when the worker can service
// WASI filesystem misses without suspending the QuickJS import path.
export function canUseSynchronousBackendAssetFetch(): boolean {
  if (typeof XMLHttpRequest === 'undefined') {
    return false
  }
  return typeof XMLHttpRequest === 'function'
}

export function selectBackendAssetLoadingMode(): BackendAssetLoadingMode {
  return canUseSynchronousBackendAssetFetch() ? 'lazy-http' : 'bounded-preload'
}

// createBackendAssetMount returns a synchronous read-only mount over plugin HTTP assets.
export function createBackendAssetMount(
  api: BackendAssetAPI,
  signal: AbortSignal,
  pathPrefix = '',
): ReadOnlyFileMount | null {
  return createBackendAssetMountWithCache(
    api,
    signal,
    pathPrefix,
    new Map<string, BackendAssetCacheEntry>(),
  )
}

function createBackendAssetMountWithCache(
  api: BackendAssetAPI,
  signal: AbortSignal,
  pathPrefix: string,
  cache: Map<string, BackendAssetCacheEntry>,
): ReadOnlyFileMount | null {
  const pluginId = api.startInfo.pluginId
  if (!pluginId) {
    return null
  }

  return {
    getFile(path: string) {
      const resolvedPath = resolveBackendAssetPath(pathPrefix + path)
      if (!resolvedPath) {
        return null
      }
      if (signal.aborted) {
        throw new Error(`QuickJS backend asset read aborted: ${resolvedPath}`)
      }

      const cached = cache.get(resolvedPath)
      if (cached?.ok) {
        return backendAssetFile(cached.data)
      }
      if (cached && cached.status === 404) {
        return null
      }
      if (cached) {
        throw new Error(
          `Failed to fetch backend asset ${cached.url}: ${cached.status}`,
        )
      }

      const url = api.utils.pluginAssetHttpPath(pluginId, resolvedPath)
      const entry = fetchBackendAssetSync(url)
      cache.set(resolvedPath, entry)
      if (entry.ok) {
        return backendAssetFile(entry.data)
      }
      if (entry.status === 404) {
        return null
      }
      throw new Error(`Failed to fetch backend asset ${url}: ${entry.status}`)
    },
  }
}

export function createBackendAssetPreopens(
  api: BackendAssetAPI,
  signal: AbortSignal,
): Fd[] {
  const cache = new Map<string, BackendAssetCacheEntry>()
  const assetsMount = createBackendAssetMountWithCache(api, signal, '', cache)
  const rootVMount = createBackendAssetMountWithCache(api, signal, 'v/', cache)
  const preopens: Fd[] = []
  if (assetsMount) {
    preopens.push(createReadOnlyMount('/assets', assetsMount))
  }
  if (rootVMount) {
    preopens.push(createReadOnlyMount('/v', rootVMount))
  }
  return preopens
}

function backendAssetFile(data: Uint8Array) {
  return {
    size: BigInt(data.byteLength),
    readAt(offset: bigint, size: number) {
      return data.slice(Number(offset), Number(offset) + size)
    },
  }
}

function fetchBackendAssetSync(url: string): BackendAssetCacheEntry {
  const xhr = new XMLHttpRequest()
  xhr.open('GET', url, false)
  xhr.responseType = 'arraybuffer'
  xhr.send()

  if (xhr.status < 200 || xhr.status >= 300) {
    return { ok: false, status: xhr.status, url }
  }
  const response = xhr.response
  if (response instanceof ArrayBuffer) {
    return { ok: true, data: new Uint8Array(response) }
  }
  return {
    ok: true,
    data: new TextEncoder().encode(xhr.responseText),
  }
}

async function loadBackendAssets(
  api: BackendAPI,
  signal: AbortSignal,
  files: Map<string, string | Uint8Array>,
  entryAssetPaths: string[],
): Promise<boolean> {
  const pluginId = api.startInfo.pluginId
  if (!pluginId) {
    return false
  }

  const manifestPath = backendAssetsRoot + '.vite/manifest.json'
  const manifestURL = api.utils.pluginAssetHttpPath(pluginId, manifestPath)
  const manifestResponse = await fetch(manifestURL, { signal })
  if (manifestResponse.status === 404) {
    return false
  }
  if (!manifestResponse.ok) {
    throw new Error(
      `Failed to fetch backend asset manifest ${manifestURL}: ${manifestResponse.status}`,
    )
  }

  const manifestText = await manifestResponse.text()
  addAssetToFileSystem(files, manifestPath, manifestText)

  const manifest = JSON.parse(
    manifestText,
  ) as Record<string, ViteManifestEntry>
  const assetPaths = collectViteManifestStaticAssetPaths(
    manifest,
    entryAssetPaths,
  )

  await Promise.all(
    assetPaths.map(async (assetPath) => {
      const resolvedPath = resolveBackendAssetPath(assetPath)
      if (!resolvedPath) {
        return
      }
      const assetURL = api.utils.pluginAssetHttpPath(pluginId, resolvedPath)
      const assetResponse = await fetch(assetURL, { signal })
      if (!assetResponse.ok) {
        throw new Error(
          `Failed to fetch backend asset ${assetURL}: ${assetResponse.status}`,
        )
      }
      addAssetToFileSystem(
        files,
        resolvedPath,
        new Uint8Array(await assetResponse.arrayBuffer()),
      )
    }),
  )
  return true
}

// main runs a JavaScript plugin in the QuickJS WASI reactor.
//
// Unlike native JS plugins that run directly in the browser, QuickJS plugins
// run inside a WebAssembly-based JavaScript VM. This provides isolation and
// allows running plugins that use synchronous I/O patterns.
export default async function main(
  api: BackendAPI,
  signal: AbortSignal,
  scriptPath: string,
): Promise<void> {
  console.log('quickjs-runner: loading QuickJS and boot harness...')

  // Load WASM module, boot harness, and plugin script in parallel
  const [wasmModule, bootHarness] = await Promise.all([
    loadQuickJSModule(),
    loadBootHarness(),
  ])

  console.log('quickjs-runner: setting up WASI environment...')

  // Create pollable stdin for yamux communication (host -> plugin)
  const stdin = new PollableStdin()

  // Track /dev/out writes for yamux communication (plugin -> host)
  const devOutStream = pushable<Uint8Array>({ objectMode: true })

  // Build virtual filesystem with boot harness and plugin script
  // The scriptPath is expected to be like /b/pd/plugin-name/plugin-HASH.mjs
  const files = new Map<string, string | Uint8Array>()

  // Add boot harness at /boot/plugin-quickjs.esm.js
  files.set('/boot/plugin-quickjs.esm.js', bootHarness)
  files.set(scriptPath, await fetchPluginScript(scriptPath))

  const assetLoadingMode = selectBackendAssetLoadingMode()
  const preopens =
    assetLoadingMode === 'lazy-http'
      ? createBackendAssetPreopens(api, signal)
      : []

  if (preopens.length === 0) {
    console.log(
      'quickjs-runner: synchronous backend asset mount unavailable; preloading backend assets',
    )
    const loadedBackendAssets = await loadBackendAssets(api, signal, files, [
      scriptPath,
    ])
    if (!loadedBackendAssets) {
      console.log(
        'quickjs-runner: backend asset manifest unavailable; using direct backend script',
      )
    }
    if (loadedBackendAssets) {
      console.log('quickjs-runner: using bounded backend asset preload')
    }
  }
  if (preopens.length !== 0) {
    console.log('quickjs-runner: using lazy backend asset mount')
  }

  const fs = buildFileSystem(files)

  // Encode start info for the plugin
  const startInfoB64 = btoa(PluginStartInfo.toJsonString(api.startInfo))

  console.log('quickjs-runner: instantiating QuickJS reactor...')

  // Create QuickJS instance
  const qjs = new QuickJS(wasmModule, {
    args: ['qjs'],
    env: [
      `BLDR_SCRIPT_PATH=${scriptPath}`,
      `BLDR_PLUGIN_START_INFO=${startInfoB64}`,
    ],
    fs,
    preopens,
    stdin,
    stdout: (line) => console.log('[QuickJS stdout]', line),
    stderr: (line) => console.error('[QuickJS stderr]', line),
    onDevOut: (data) => devOutStream.push(new Uint8Array(data)),
  })

  console.log('quickjs-runner: initializing QuickJS...')

  // Initialize QuickJS with --std flag and boot harness path.
  // This sets up the module loader and evaluates the boot harness as the main script.
  qjs.init(['qjs', '--std', '/boot/plugin-quickjs.esm.js'])

  console.log('quickjs-runner: starting reactor event loop...')

  // Set up yamux connection for RPC
  // The host side (us) is 'outbound' - we initiate streams to WebRuntime
  const hostConn = new StreamConn(
    { handlePacketStream: api.handleStreamCtr.handleStreamFunc },
    {
      direction: 'outbound',
      yamuxParams: {
        enableKeepAlive: false,
        maxMessageSize: 32 * 1024,
      },
    },
  )

  // Pipe devOut to hostConn, and hostConn output to stdin
  pipe(devOutStream, hostConn, async (source) => {
    for await (const chunk of source) {
      // chunk may be Uint8Array or Uint8ArrayList, normalize to Uint8Array
      const data =
        chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.subarray())
      qjs.pushStdin(data)
    }
  }).catch((err) => {
    console.error('quickjs-runner: yamux pipe error:', err)
  })

  // Set up stream handling from plugin to host
  api.handleStreamCtr.set(async (stream) => {
    // When plugin opens a stream, forward it to WebRuntime
    const hostStream = await api.openStream()
    pipe(stream, hostStream, stream).catch((err) => {
      console.error('quickjs-runner: stream pipe error:', err)
    })
  })

  // Run the reactor event loop
  let running = true
  let exitCode = 0
  let pendingTimeout: ReturnType<typeof setTimeout> | null = null
  let waitingForWake = false
  let exitResolve: (() => void) | null = null

  // Handle abort signal
  const onAbort = () => {
    running = false
    if (pendingTimeout !== null) {
      clearTimeout(pendingTimeout)
      pendingTimeout = null
    }
    exitResolve?.()
  }
  signal.addEventListener('abort', onAbort)

  // Wake callback: when stdin receives data, cancel any pending timeout and run immediately
  qjs.onStdinWake(() => {
    if (pendingTimeout !== null) {
      clearTimeout(pendingTimeout)
      pendingTimeout = null
      queueMicrotask(runLoop)
    } else if (waitingForWake) {
      waitingForWake = false
      queueMicrotask(runLoop)
    }
  })

  const runLoop = () => {
    if (!running) {
      exitResolve?.()
      return
    }
    pendingTimeout = null
    waitingForWake = false

    let result: number
    try {
      result = qjs.loopOnce()
    } catch (e) {
      running = false
      exitCode = 1
      console.error('quickjs-runner: error in loopOnce:', e)
      exitResolve?.()
      return
    }

    if (result === LOOP_ERROR) {
      console.error('quickjs-runner: JavaScript error occurred')
      running = false
      exitResolve?.()
      return
    }

    if (result === 0) {
      // More microtasks pending, continue immediately
      queueMicrotask(runLoop)
      return
    }

    if (result > 0) {
      // Timer pending - but also check stdin
      if (qjs.hasStdinData()) {
        try {
          qjs.pollIO(0)
        } catch (e) {
          running = false
          exitCode = 1
          console.error('quickjs-runner: error in pollIO:', e)
          exitResolve?.()
          return
        }
        queueMicrotask(runLoop)
        return
      }
      // Wait for timer (onStdinWake will interrupt if data arrives)
      pendingTimeout = setTimeout(runLoop, result)
      return
    }

    if (result === LOOP_IDLE) {
      // Idle - check if stdin has data
      if (qjs.hasStdinData()) {
        try {
          qjs.pollIO(0)
        } catch (e) {
          running = false
          exitCode = 1
          console.error('quickjs-runner: error in pollIO:', e)
          exitResolve?.()
          return
        }
        queueMicrotask(runLoop)
        return
      }
      // No data - wait for onStdinWake callback to restart the loop
      waitingForWake = true
      return
    }
  }

  // Start the event loop
  runLoop()

  // Wait for the plugin to exit
  await new Promise<void>((resolve) => {
    exitResolve = resolve
    if (!running) resolve()
  })

  // Cleanup
  signal.removeEventListener('abort', onAbort)
  devOutStream.end()
  qjs.destroy()

  if (exitCode !== 0) {
    throw new Error(`Plugin exited with code ${exitCode}`)
  }
}
