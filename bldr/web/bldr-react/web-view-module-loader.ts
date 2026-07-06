import {
  beginBootDownload,
  completeBootDownload,
  failBootDownload,
} from '../bldr/boot-downloads.js'

export interface WebViewRootAssetResult {
  scriptPath: string
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
}

const rootPluginAssetPrefix = '/b/pa/'
export const webViewRootAssetStatusEvent = 'bldr:webview-root-asset-status'
export const webViewModuleImportErrorEvent = 'bldr:webview-module-import-error'
const moduleImportRetryNonceByScriptPath = new Map<string, number>()

// webViewDownloadLabel derives a readable plugin name from a root asset path
// such as "/b/pa/spacewave-app/v/b/fe/module.mjs" -> "spacewave-app".
function webViewDownloadLabel(scriptPath: string): string {
  const afterPrefix = scriptPath.slice(scriptPath.indexOf(rootPluginAssetPrefix) + rootPluginAssetPrefix.length)
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
    return (await response.clone().text()).slice(0, 300)
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

export function getWebViewRootAssetLoadError(
  error: unknown,
): WebViewRootAssetLoadError | undefined {
  return error instanceof WebViewRootAssetLoadError ? error : undefined
}

export async function fetchWebViewRootAssetResult(
  scriptPath: string,
  fetchRootAsset: typeof fetch = fetch,
): Promise<WebViewRootAssetResult> {
  const response = await fetchRootAsset(scriptPath, { cache: 'no-store' })
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

  recordWebViewRootAssetResult(result)
  return result
}

async function importWebViewScriptModule<T>(scriptPath: string): Promise<T> {
  return import(/* @vite-ignore */ scriptPath) as Promise<T>
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
}

export async function loadWebViewScriptModule<T>(
  scriptPath: string,
  options: LoadWebViewScriptModuleOptions<T> = {},
): Promise<T> {
  const isRootPluginAsset = isWebViewRootPluginAssetPath(scriptPath)
  // The browser's native dynamic import() exposes no byte progress, so a plugin
  // module load is tracked at honest started/completed granularity (no faked
  // percentage) rather than a smooth bar. The boot download registry owns the
  // per-plugin accounting; the loading screen renders it.
  if (isRootPluginAsset) {
    beginBootDownload(scriptPath, webViewDownloadLabel(scriptPath))
  }
  const rootAsset = isRootPluginAsset
    ? await fetchWebViewRootAssetResult(scriptPath, options.fetchRootAsset)
    : undefined
  if (rootAsset && (!rootAsset.ok || rootAsset.classification !== 'live')) {
    if (isRootPluginAsset) {
      failBootDownload(scriptPath, rootAsset.classification)
    }
    throw new WebViewRootAssetLoadError(rootAsset)
  }

  const importModule = options.importModule ?? importWebViewScriptModule<T>
  const moduleImportPath = buildModuleImportPath(scriptPath)
  try {
    const module = await importModule(moduleImportPath)
    recordModuleImportSuccess(scriptPath)
    if (isRootPluginAsset) {
      completeBootDownload(scriptPath)
    }
    return module
  } catch (error) {
    recordModuleImportFailure(scriptPath)
    if (isRootPluginAsset) {
      failBootDownload(scriptPath, serializeImportError(error).message)
    }
    recordWebViewModuleImportError({
      scriptPath,
      ...serializeImportError(error),
      rootAsset,
    })
    throw error
  }
}
