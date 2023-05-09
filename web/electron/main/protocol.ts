import electron from 'electron'
import fs from 'fs'
import path from 'path'

import { debugConsole } from './console.js'

export const APP_SCHEME = 'app'

const app = electron.app
const distPath = app.getAppPath()

// originally from reasonably-secure-electron
const mimeTypes: { [ext: string]: string } = {
  '.js': 'text/javascript',
  '.ts': 'application/x-typescript',
  '.mjs': 'text/javascript',
  '.html': 'text/html',
  '.htm': 'text/html',
  '.json': 'application/json',
  '.css': 'text/css',
  '.svg': 'application/svg+xml',
  '.ico': 'image/vnd.microsoft.icon',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.map': 'text/plain',
}

function charset(mimeType: string) {
  return ['.html', '.htm', '.js', '.mjs', '.ts'].some((m) => m === mimeType)
    ? 'utf-8'
    : null
}

function mime(filename: string) {
  const type = mimeTypes[path.extname(`${filename || ''}`).toLowerCase()]
  return type || null
}

// handle requests for distribution files
function appRequestHandler(
  req: electron.ProtocolRequest,
  next: (response: Buffer | electron.ProtocolResponse) => void
) {
  const reqUrl = new URL(req.url)
  let reqPath = path.normalize(reqUrl.pathname)
  if (reqPath.length === 0 || reqPath === path.sep) {
    reqPath = path.sep + 'index.html'
  }
  const reqFilename = path.basename(reqPath)
  let filePath = distPath
  if (reqPath.startsWith(path.sep + 'node_modules' + path.sep)) {
    filePath = path.join(filePath, '../../../')
  }
  filePath = path.join(filePath, reqPath)
  fs.readFile(filePath, (err, data) => {
    const mimeType = mime(reqFilename)
    if (!err && mimeType !== null) {
      next({
        mimeType: mimeType,
        charset: charset(mimeType) || undefined,
        data: data,
      })
    } else {
      // file doesn't exist
      // TODO: forward requests for /b/ and /p/ to service worker fetch()
      debugConsole.error('appRequestHandler: failed to fetch', filePath) // , err)
    }
  })
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

export function initProtocol() {
  electron.protocol.registerBufferProtocol(APP_SCHEME, appRequestHandler)
}
