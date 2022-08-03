import electron from 'electron'
import fs from 'fs'
import path from 'path'

import { debugConsole } from './console.js'

export const APP_SCHEME = 'app'

const app = electron.app
const distPath = app.getAppPath()

// from reasonably-secure-electron
const mimeTypes: { [ext: string]: string } = {
  '.js': 'text/javascript',
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
  return ['.html', '.htm', '.js', '.mjs'].some((m) => m === mimeType)
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
  if (reqPath === '/') {
    reqPath = '/index.html'
  }
  const reqFilename = path.basename(reqPath)
  fs.readFile(path.join(distPath, reqPath), (err, data) => {
    const mimeType = mime(reqFilename)
    if (!err && mimeType !== null) {
      next({
        mimeType: mimeType,
        charset: charset(mimeType) || undefined,
        data: data,
      })
    } else {
      debugConsole.error(err)
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
