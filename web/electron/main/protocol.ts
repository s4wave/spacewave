import electron from 'electron'
import path from 'path'
import url from 'url'

import { BLDR_URI_PREFIXES } from '../../bldr/constants.js'

export const APP_SCHEME = 'app'

const app = electron.app
const distPath = app.getAppPath()

// handle requests for distribution files
export async function appRequestHandler(
  swFetch: (req: GlobalRequest) => Promise<GlobalResponse>,
  req: GlobalRequest,
): Promise<GlobalResponse> {
  const reqUrl = new URL(req.url)
  let reqPath = path.normalize(reqUrl.pathname)
  if (reqPath.length === 0 || reqPath === path.sep) {
    reqPath = path.sep + 'index.html'
  }

  // If reqPath starts with /p/ or /b/, forward to ServiceWorker.
  const matchPrefixes = BLDR_URI_PREFIXES
  for (const matchPrefix of matchPrefixes) {
    if (reqPath.startsWith(matchPrefix)) {
      // This request should have been intercepted by the ServiceWorker.
      // If it got here: it must be a SourceMap file (.map).
      // See: https://stackoverflow.com/q/77706210/431369
      // See: https://bugs.chromium.org/p/chromium/issues/detail?id=1513959
      console.log(
        `appRequestHandler: forwarding ServiceWorker request: ${reqPath}`,
      )
      return swFetch(req)
    }
  }

  // Serve a file from the Electron app.asar.
  // Make sure the path is within the distPath.
  let filePath = distPath
  if (reqPath.startsWith(path.sep + 'node_modules' + path.sep)) {
    filePath = path.join(filePath, '../../../')
  }
  filePath = path.join(filePath, reqPath)
  if (!filePath.startsWith(distPath)) {
    console.warn('appRequestHandler: blocking fetch: ' + filePath)
    return new Response('Forbidden: Access is denied', {
      status: 403,
      headers: { 'Content-Type': 'text/plain' },
    })
  }

  // check if the file exists
  try {
    return await electron.net.fetch(url.pathToFileURL(filePath).toString())
  } catch (err) {
    console.warn(`appRequestHandler: failed fetch: ${filePath} -> ${err}`)
    return new Response('Not found', {
      status: 404,
      headers: { 'Content-Type': 'text/plain' },
    })
  }
}

// set paths as privileged for service worker
electron.protocol.registerSchemesAsPrivileged([
  {
    scheme: APP_SCHEME,
    privileges: {
      standard: true,
      secure: true,
      allowServiceWorkers: true,
      bypassCSP: true,
      supportFetchAPI: true,
      corsEnabled: true,
      stream: true,
    },
  },
])
