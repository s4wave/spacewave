import electron from 'electron'
import path from 'path'
import url from 'url'

import { BLDR_URI_PREFIXES } from '../../bldr/constants.js'

export const APP_SCHEME = 'app'

const app = electron.app
const distPath = app.getAppPath()

type ElectronRuntimeFetchSource = 'plugin-assets' | 'plugin-dist'

const pluginAssetsPathPrefix = '/b/pa/'
const pluginDistPathPrefix = '/b/pd/'
const fetchSourceHeader = 'X-Bldr-Fetch-Source'
const pluginAssetFetchResultHeader = 'X-Bldr-Plugin-Asset-Fetch-Result'

const crossOriginIsolationHeaders = {
  'Cross-Origin-Opener-Policy': 'same-origin',
  'Cross-Origin-Embedder-Policy': 'credentialless',
  'Cross-Origin-Resource-Policy': 'same-origin',
}

// extractWebDocumentClientId recovers the owning WebDocument ID from a request.
export function extractWebDocumentClientId(
  req: GlobalRequest,
): string | undefined {
  const requestUrls = [req.referrer, req.headers.get('referer'), req.url]
  for (const requestUrl of requestUrls) {
    if (!requestUrl) {
      continue
    }
    try {
      const parsed = new URL(requestUrl)
      const webDocumentId = parsed.searchParams.get('webDocumentId')
      if (webDocumentId) {
        return webDocumentId
      }
    } catch {
      continue
    }
  }
}

// appRequestHandler handles requests for distribution and Bldr runtime files.
export async function appRequestHandler(
  swFetch: (req: GlobalRequest, clientId?: string) => Promise<GlobalResponse>,
  req: GlobalRequest,
): Promise<GlobalResponse> {
  const reqUrl = new URL(req.url)
  let reqPath = path.posix.normalize(reqUrl.pathname)
  if (reqPath.length === 0 || reqPath === '/') {
    reqPath = '/index.html'
  }

  // Forward Bldr runtime requests to the Go runtime fetch service.
  const runtimeFetchSource: ElectronRuntimeFetchSource | undefined =
    reqPath.startsWith(pluginAssetsPathPrefix)
      ? 'plugin-assets'
      : reqPath.startsWith(pluginDistPathPrefix)
        ? 'plugin-dist'
        : undefined
  for (const matchPrefix of BLDR_URI_PREFIXES) {
    if (reqPath.startsWith(matchPrefix)) {
      console.log(`appRequestHandler: forwarding Bldr request: ${reqPath}`)
      const response = await swFetch(req, extractWebDocumentClientId(req))
      return withElectronAppResponseHeaders(response, runtimeFetchSource)
    }
  }

  // Serve a file from the Electron app.asar.
  // Make sure the path is within the distPath.
  let filePath = distPath
  if (reqPath.startsWith('/node_modules/')) {
    filePath = path.join(filePath, '../../../')
  }
  filePath = path.join(filePath, reqPath)
  if (!filePath.startsWith(distPath)) {
    console.warn('appRequestHandler: blocking fetch: ' + filePath)
    return withElectronAppResponseHeaders(
      new Response('Forbidden: Access is denied', {
        status: 403,
        headers: { 'Content-Type': 'text/plain' },
      }),
    )
  }

  // check if the file exists
  try {
    return withElectronAppResponseHeaders(
      await electron.net.fetch(url.pathToFileURL(filePath).toString()),
    )
  } catch (err) {
    // TODO: We know that .map files are not being fetched properly.
    // https://issues.chromium.org/issues/40765087
    // supersedes: https://issues.chromium.org/issues/41486524#comment4
    if (filePath.endsWith('.map')) {
      return withElectronAppResponseHeaders(
        new Response(
          'Source maps are not loaded via ServiceWorker correctly: see: https://issues.chromium.org/issues/40765087',
          {
            status: 503,
            headers: { 'Content-Type': 'text/plain' },
          },
        ),
      )
    }

    // SPA fallback: if the file doesn't exist and it's not a source map,
    // redirect to the correct base URL. This catches accidental navigations
    // to paths like app://index.html/feed.xml in a hash-routed SPA.
    const docId = reqUrl.searchParams.get('webDocumentId')
    let baseUrl = `${APP_SCHEME}://index.html`
    if (docId) {
      baseUrl += `?webDocumentId=${encodeURIComponent(docId)}`
    }
    console.warn(`appRequestHandler: SPA redirect: ${reqPath} -> ${baseUrl}`)
    return withElectronAppResponseHeaders(
      new Response(null, {
        status: 301,
        headers: { Location: baseUrl },
      }),
    )
  }
}

function withElectronAppResponseHeaders(
  response: GlobalResponse,
  source?: ElectronRuntimeFetchSource,
): GlobalResponse {
  const headers = new Headers(response.headers)
  for (const [key, value] of Object.entries(crossOriginIsolationHeaders)) {
    headers.set(key, value)
  }
  if (source) {
    headers.set(fetchSourceHeader, source)
    if (response.ok && !headers.has(pluginAssetFetchResultHeader)) {
      headers.set(pluginAssetFetchResultHeader, 'live')
    }
  }
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  }) as GlobalResponse
}

// The app:// scheme stays fetch-capable for protocol handlers, but Electron
// WebDocument uses the main-process protocol handler instead of a ServiceWorker.
electron.protocol.registerSchemesAsPrivileged([
  {
    scheme: APP_SCHEME,
    privileges: {
      standard: true,
      secure: true,
      bypassCSP: true,
      supportFetchAPI: true,
      corsEnabled: true,
      stream: true,
    },
  },
])
