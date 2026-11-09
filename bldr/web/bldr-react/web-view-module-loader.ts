import {
  beginBootDownload,
  completeBootDownload,
  failBootDownload,
} from '../bldr/boot-downloads.js'

export interface WebViewRootAssetResult {
  scriptPath: string
  // status is the observed HTTP code, or zero when only import completion is known.
  status: number
  ok: boolean
  fetchSource?: string
  runtimeError?: string
  pluginAssetResult?: string
  contentType?: string
  bodyPrefix?: string
  classification: string
}

export interface WebViewModuleImportErrorResult {
  scriptPath: string
  name: string
  message: string
  stack?: string
  rootAsset?: WebViewRootAssetResult
}

declare global {
  var __bldrWebViewRootAssetStatus: WebViewRootAssetResult | undefined
  var __bldrWebViewModuleImportError: WebViewModuleImportErrorResult | undefined
}

export type WebViewModuleImporter<T> = (scriptPath: string) => Promise<T>

export interface LoadWebViewScriptModuleOptions<T> {
  fetchRootAsset?: typeof fetch
  importModule?: WebViewModuleImporter<T>
  // fetchDeadlineMillis overrides the root-asset fetch deadline (tests).
  fetchDeadlineMillis?: number
  // importDeadlineMillis overrides the module import deadline (tests).
  importDeadlineMillis?: number
}

// Root-asset fetches and dynamic imports have no built-in timeout. The fetch
// deadline bounds the diagnostic probe. The import deadline reports a stall:
// the browser keeps loading the module, so a retry waits on the same import,
// and the web view's reveal deadline remains the terminal bound.
const rootAssetFetchDeadlineMillis = 45_000
const moduleImportDeadlineMillis = 45_000

// maxModuleImportStalls is how many deadlines one import may miss before the
// next attempt starts a fresh import, for an engine that wedged the original.
const maxModuleImportStalls = 2

const rootPluginAssetPrefix = '/b/pa/'
export const webViewRootAssetStatusEvent = 'bldr:webview-root-asset-status'
export const webViewModuleImportErrorEvent = 'bldr:webview-module-import-error'
const moduleImportRetryNonceByScriptPath = new Map<string, number>()
const moduleLoadPromiseByImportPath = new Map<string, Promise<unknown>>()

// InflightModuleImport is a browser module import that has not settled.
interface InflightModuleImport {
  promise: Promise<unknown>
  // stalls counts the deadlines this import missed.
  stalls: number
}

// inflightImportByImportPath holds imports that outlived a deadline, so a
// retry waits on the browser's existing fetch instead of starting another.
const inflightImportByImportPath = new Map<string, InflightModuleImport>()

// webViewDownloadLabel derives a readable plugin name from a root asset path
// such as "/b/pa/spacewave-app/v/b/fe/module.mjs" -> "spacewave-app".
function webViewDownloadLabel(scriptPath: string): string {
  const afterPrefix = scriptPath.slice(
    scriptPath.indexOf(rootPluginAssetPrefix) + rootPluginAssetPrefix.length,
  )
  const pluginId = afterPrefix.split('/')[0]
  return pluginId || 'Plugin'
}

function headerValue(headers: Headers, name: string): string | undefined {
  return headers.get(name) ?? undefined
}

export function isWebViewRootPluginAssetPath(scriptPath: string): boolean {
  if (scriptPath.startsWith(rootPluginAssetPrefix)) {
    return true
  }

  try {
    const baseURL = globalThis.location?.href ?? 'http://localhost/'
    return new URL(scriptPath, baseURL).pathname.startsWith(
      rootPluginAssetPrefix,
    )
  } catch {
    return false
  }
}

function classifyRootAssetResponse(response: Response): string {
  const pluginAssetResult = headerValue(
    response.headers,
    'X-Bldr-Plugin-Asset-Fetch-Result',
  )
  if (pluginAssetResult) {
    return pluginAssetResult
  }

  const runtimeError = headerValue(
    response.headers,
    'X-Bldr-Runtime-Fetch-Error',
  )
  if (runtimeError) {
    return runtimeError
  }

  if (!headerValue(response.headers, 'X-Bldr-Fetch-Source')) {
    return 'bypass'
  }

  return response.ok ? 'live' : 'failed'
}

async function readBodyPrefix(response: Response): Promise<string | undefined> {
  try {
    return (await response.text()).slice(0, 300)
  } catch {
    return undefined
  }
}

async function closeResponseBody(response: Response) {
  try {
    await response.body?.cancel()
  } catch {
    // The root module is imported through the browser module loader after the
    // diagnostic probe; failed cancellation should not mask the import result.
  }
}

function cloneRootAssetResult(
  result: WebViewRootAssetResult,
): WebViewRootAssetResult {
  return { ...result }
}

export function recordWebViewRootAssetResult(result: WebViewRootAssetResult) {
  const snapshot = cloneRootAssetResult(result)
  globalThis.__bldrWebViewRootAssetStatus = snapshot
  if (typeof globalThis.dispatchEvent === 'function') {
    try {
      globalThis.dispatchEvent(
        new CustomEvent(webViewRootAssetStatusEvent, { detail: snapshot }),
      )
    } catch {
      // Status publication is diagnostic only; asset loading owns the result.
    }
  }
}

function serializeImportError(error: unknown): {
  name: string
  message: string
  stack?: string
} {
  if (error instanceof Error) {
    return {
      name: error.name || 'Error',
      message: error.message || String(error),
      stack: error.stack,
    }
  }
  return {
    name: 'NonError',
    message: String(error),
  }
}

export function recordWebViewModuleImportError(
  result: WebViewModuleImportErrorResult,
) {
  const snapshot = {
    ...result,
    rootAsset: result.rootAsset
      ? cloneRootAssetResult(result.rootAsset)
      : undefined,
  }
  globalThis.__bldrWebViewModuleImportError = snapshot
  if (typeof globalThis.dispatchEvent === 'function') {
    try {
      globalThis.dispatchEvent(
        new CustomEvent(webViewModuleImportErrorEvent, { detail: snapshot }),
      )
    } catch {
      // Status publication is diagnostic only; module loading owns the result.
    }
  }
}

export class WebViewRootAssetLoadError extends Error {
  public readonly rootAsset: WebViewRootAssetResult

  constructor(rootAsset: WebViewRootAssetResult) {
    super(
      `failed to load root plugin asset: ${rootAsset.scriptPath} (${rootAsset.status} ${rootAsset.classification})`,
    )
    this.name = 'WebViewRootAssetLoadError'
    this.rootAsset = rootAsset
  }
}

// WebViewModuleImportStallError reports an import still loading at its
// deadline. It is recoverable: retrying waits on the same import.
export class WebViewModuleImportStallError extends Error {
  public readonly scriptPath: string

  constructor(scriptPath: string, deadlineMillis: number) {
    super(
      `module import ${scriptPath}: still loading after ${deadlineMillis}ms`,
    )
    this.name = 'WebViewModuleImportStallError'
    this.scriptPath = scriptPath
  }
}

export function getWebViewModuleImportStallError(
  error: unknown,
): WebViewModuleImportStallError | undefined {
  return error instanceof WebViewModuleImportStallError ? error : undefined
}

export function getWebViewRootAssetLoadError(
  error: unknown,
): WebViewRootAssetLoadError | undefined {
  return error instanceof WebViewRootAssetLoadError ? error : undefined
}

// fetchWebViewRootAssetResult records one bounded diagnostic probe.
export async function fetchWebViewRootAssetResult(
  scriptPath: string,
  fetchRootAsset: typeof fetch = fetch,
  fetchDeadlineMillis: number = rootAssetFetchDeadlineMillis,
): Promise<WebViewRootAssetResult> {
  const abort = new AbortController()
  try {
    const result = await withDeadline(
      readWebViewRootAssetResult(scriptPath, fetchRootAsset, abort.signal),
      fetchDeadlineMillis,
      `root asset fetch ${scriptPath}`,
    )
    recordWebViewRootAssetResult(result)
    return result
  } finally {
    abort.abort()
  }
}

// readWebViewRootAssetResult classifies the response without publishing partial results.
async function readWebViewRootAssetResult(
  scriptPath: string,
  fetchRootAsset: typeof fetch,
  signal: AbortSignal,
): Promise<WebViewRootAssetResult> {
  const response = await fetchRootAsset(scriptPath, {
    cache: 'no-store',
    signal,
  })
  const classification = classifyRootAssetResponse(response)
  const result: WebViewRootAssetResult = {
    scriptPath,
    status: response.status,
    ok: response.ok,
    fetchSource: headerValue(response.headers, 'X-Bldr-Fetch-Source'),
    runtimeError: headerValue(response.headers, 'X-Bldr-Runtime-Fetch-Error'),
    pluginAssetResult: headerValue(
      response.headers,
      'X-Bldr-Plugin-Asset-Fetch-Result',
    ),
    contentType: headerValue(response.headers, 'content-type'),
    classification,
  }

  if (!response.ok) {
    result.bodyPrefix = await readBodyPrefix(response)
  } else {
    await closeResponseBody(response)
  }

  return result
}

async function importWebViewScriptModule<T>(scriptPath: string): Promise<T> {
  return import(/* @vite-ignore */ scriptPath) as Promise<T>
}

// withDeadline rejects with a timeout error if the wrapped promise does not
// settle within deadlineMillis. The wrapped promise itself is left running;
// its eventual result is ignored once the deadline has fired.
async function withDeadline<T>(
  promise: Promise<T>,
  deadlineMillis: number,
  description: string,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new Error(`${description}: timed out after ${deadlineMillis}ms`))
    }, deadlineMillis)
  })
  try {
    return await Promise.race([promise, timeout])
  } finally {
    if (timer !== undefined) clearTimeout(timer)
  }
}

// settleWithin resolves with the promise's value, or undefined when the
// deadline passes first. A rejection propagates.
async function settleWithin<T>(
  promise: Promise<T>,
  deadlineMillis: number,
): Promise<{ value: T } | undefined> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const deadline = new Promise<undefined>((resolve) => {
    timer = setTimeout(() => resolve(undefined), deadlineMillis)
  })
  try {
    return await Promise.race([promise.then((value) => ({ value })), deadline])
  } finally {
    if (timer !== undefined) clearTimeout(timer)
  }
}

// startModuleImport returns the import in flight for the path or starts one.
function startModuleImport<T>(
  moduleImportPath: string,
  importModule: WebViewModuleImporter<T>,
): InflightModuleImport {
  const existing = inflightImportByImportPath.get(moduleImportPath)
  if (existing) {
    return existing
  }
  const inflight: InflightModuleImport = {
    promise: importModule(moduleImportPath),
    stalls: 0,
  }
  inflightImportByImportPath.set(moduleImportPath, inflight)
  const release = () => {
    if (inflightImportByImportPath.get(moduleImportPath) === inflight) {
      inflightImportByImportPath.delete(moduleImportPath)
    }
  }
  void inflight.promise.then(release, release)
  return inflight
}

function buildModuleImportPath(scriptPath: string): string {
  const retryNonce = moduleImportRetryNonceByScriptPath.get(scriptPath) ?? 0
  if (retryNonce === 0) {
    return scriptPath
  }

  const separator = scriptPath.includes('?') ? '&' : '?'
  return `${scriptPath}${separator}bldr_retry=${retryNonce}`
}

function recordModuleImportFailure(scriptPath: string) {
  const retryNonce = moduleImportRetryNonceByScriptPath.get(scriptPath) ?? 0
  moduleImportRetryNonceByScriptPath.set(scriptPath, retryNonce + 1)
}

function recordModuleImportSuccess(scriptPath: string) {
  moduleImportRetryNonceByScriptPath.delete(scriptPath)
  try {
    if (
      globalThis.sessionStorage?.getItem(moduleGraphReloadSessionKey) ===
      scriptPath
    ) {
      globalThis.sessionStorage.removeItem(moduleGraphReloadSessionKey)
    }
  } catch {
    // Storage is unavailable; no reload guard was recorded.
  }
}

// moduleGraphReloadSessionKey names the root script whose failed module graph
// already reloaded this tab, so a persistent failure reloads only once.
const moduleGraphReloadSessionKey = 'bldr:webview-module-graph-reload'

function isModuleFetchFailureMessage(message: string): boolean {
  return (
    message.includes('Failed to fetch dynamically imported module') ||
    message.includes('error loading dynamically imported module') ||
    message.includes('Importing a module script failed')
  )
}

// reloadForFailedModuleGraph reloads the document once when an import failed
// although the root asset probe was live. The browser keeps a failed module
// fetch in the document's module map, and the retry nonce changes only the
// root URL, so a dependency that failed once fails every retry in this
// document. Returns true when it started the reload.
export function reloadForFailedModuleGraph(
  error: unknown,
  reload: () => void = () => globalThis.location.reload(),
): boolean {
  const record = globalThis.__bldrWebViewModuleImportError
  if (!(error instanceof Error) || !record?.rootAsset) {
    return false
  }
  if (record.message !== error.message) {
    return false
  }
  if (!isModuleFetchFailureMessage(record.message)) {
    return false
  }
  if (!record.rootAsset.ok || record.rootAsset.classification !== 'live') {
    return false
  }
  try {
    const storage = globalThis.sessionStorage
    if (!storage) {
      return false
    }
    if (storage.getItem(moduleGraphReloadSessionKey) === record.scriptPath) {
      return false
    }
    storage.setItem(moduleGraphReloadSessionKey, record.scriptPath)
  } catch {
    return false
  }
  reload()
  return true
}

export function loadWebViewScriptModule<T>(
  scriptPath: string,
  options: LoadWebViewScriptModuleOptions<T> = {},
): Promise<T> {
  const moduleImportPath = buildModuleImportPath(scriptPath)
  const existing = moduleLoadPromiseByImportPath.get(moduleImportPath)
  if (existing) {
    return existing as Promise<T>
  }

  const promise = loadWebViewScriptModuleUncached(
    scriptPath,
    moduleImportPath,
    options,
  )
  moduleLoadPromiseByImportPath.set(moduleImportPath, promise)
  void promise.then(
    () => {
      if (moduleLoadPromiseByImportPath.get(moduleImportPath) === promise) {
        moduleLoadPromiseByImportPath.delete(moduleImportPath)
      }
    },
    () => {
      if (moduleLoadPromiseByImportPath.get(moduleImportPath) === promise) {
        moduleLoadPromiseByImportPath.delete(moduleImportPath)
      }
    },
  )
  return promise
}

// loadWebViewScriptModuleUncached imports once and probes HTTP details only on failure.
async function loadWebViewScriptModuleUncached<T>(
  scriptPath: string,
  moduleImportPath: string,
  options: LoadWebViewScriptModuleOptions<T>,
): Promise<T> {
  const isRootPluginAsset = isWebViewRootPluginAssetPath(scriptPath)
  // Native imports expose completion but no byte progress.
  if (isRootPluginAsset) {
    beginBootDownload(scriptPath, webViewDownloadLabel(scriptPath))
  }

  const importModule = options.importModule ?? importWebViewScriptModule<T>
  const deadlineMillis =
    options.importDeadlineMillis ?? moduleImportDeadlineMillis
  const startedAt = performance.now()
  const inflight = startModuleImport(moduleImportPath, importModule)
  let settled: { value: unknown } | undefined
  try {
    settled = await settleWithin(inflight.promise, deadlineMillis)
  } catch (error) {
    return failModuleImport(scriptPath, isRootPluginAsset, error, {
      startedAt,
      deadlineMillis,
      options,
    })
  }

  if (!settled) {
    // The browser is still loading the module: report a recoverable stall and
    // leave the boot download active for the retry to finish.
    inflight.stalls++
    if (inflight.stalls >= maxModuleImportStalls) {
      inflightImportByImportPath.delete(moduleImportPath)
      recordModuleImportFailure(scriptPath)
    }
    const stall = new WebViewModuleImportStallError(
      moduleImportPath,
      deadlineMillis,
    )
    recordWebViewModuleImportError({
      scriptPath,
      ...serializeImportError(stall),
    })
    throw stall
  }

  recordModuleImportSuccess(scriptPath)
  if (isRootPluginAsset) {
    // Import completion proves usability without inventing an HTTP response.
    recordWebViewRootAssetResult({
      scriptPath,
      status: 0,
      ok: true,
      classification: 'imported',
    })
    completeBootDownload(scriptPath)
  }
  return settled.value as T
}

// failModuleImport probes a failed root plugin asset, records the failure, and
// throws the most specific error.
async function failModuleImport<T>(
  scriptPath: string,
  isRootPluginAsset: boolean,
  error: unknown,
  probe: {
    startedAt: number
    deadlineMillis: number
    options: LoadWebViewScriptModuleOptions<T>
  },
): Promise<never> {
  const { startedAt, deadlineMillis, options } = probe
  let failure = error
  let rootAsset: WebViewRootAssetResult | undefined
  if (isRootPluginAsset) {
    failBootDownload(scriptPath, serializeImportError(failure).message)
    // Diagnostics share the import deadline rather than extending a failed load.
    const remaining = deadlineMillis - (performance.now() - startedAt)
    if (remaining > 0) {
      try {
        rootAsset = await fetchWebViewRootAssetResult(
          scriptPath,
          options.fetchRootAsset,
          Math.min(
            options.fetchDeadlineMillis ?? rootAssetFetchDeadlineMillis,
            remaining,
          ),
        )
        if (!rootAsset.ok || rootAsset.classification !== 'live') {
          failure = new WebViewRootAssetLoadError(rootAsset)
        }
      } catch {
        // A failed diagnostic probe must preserve the original import error.
      }
    }
    failBootDownload(scriptPath, serializeImportError(failure).message)
  }
  recordModuleImportFailure(scriptPath)
  recordWebViewModuleImportError({
    scriptPath,
    ...serializeImportError(failure),
    rootAsset,
  })
  throw failure
}
