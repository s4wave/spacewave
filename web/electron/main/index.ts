import electron, { MessageChannelMain } from 'electron'
import net from 'net'
import path from 'path'
import fs from 'fs'
import console from 'console'

import { pushable } from 'it-pushable'

const app = electron.app
const ipcMain: Electron.IpcMain = electron.ipcMain

const scheme = 'app'
const distPath = app.getAppPath()

const debug = new console.Console(process.stdout, process.stderr)

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
      debug.error(err)
    }
  })
}

// set paths as privileged for service worker
electron.protocol.registerSchemesAsPrivileged([
  {
    scheme,
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

function initProtocol() {
  electron.protocol.registerBufferProtocol(scheme, appRequestHandler)
}

let mainWindow: electron.BrowserWindow | null

// socketTx is data outgoing to the socket.
let socketTx = pushable<Uint8Array>({ objectMode: true })
// socketRx is data incoming from the socket to the page.
let socketRx = pushable<Uint8Array>({ objectMode: true })

// setup the ipc socket
// retries if disconnected
function setupSocket(runtimeUuid: string) {
  const pipeName = `.pipe-${runtimeUuid}`
  let ipcPath = path.join(process.cwd(), pipeName)
  if (process.platform === 'win32') {
    ipcPath = path.join('\\\\.\\pipe', process.cwd(), pipeName)
  }

  const sock = net.connect(ipcPath, async () => {
    debug.log('ipc connection opened')
    for await (const data of socketTx) {
      debug.log('socketTx: wrote data', data)
      sock.write(data)
    }
  })
  sock.on('data', (data) => {
    debug.log('socketRx: got data', data)
    socketRx.push(data)
  })
  sock.on('end', () => {
    // assume we are exiting
    debug.error('ipc connection closed')
    process.exit(0)
  })
  sock.on('error', (err) => {
    debug.error('ipc connection errored', err)
    // ...but also exit if this happens.
    process.exit(1)
  })
}

// setup handler for MessagePort updates.
function setupRuntimePort() {
  ipcMain.on('BLDR_PORT', async (event, webRuntimeUuid: string) => {
    const channel = new MessageChannelMain()
    const socketPort = channel.port1
    const remotePort = channel.port2

    // send the remote port to the web runtime
    event.sender.postMessage(webRuntimeUuid, null, [remotePort])

    socketPort.on('message', (event) => {
      const data = event?.data as Uint8Array
      if (data && data.length) {
        socketTx.push(data)
      }
    })
    ;(async () => {
      for await (const pkt of socketRx) {
        socketPort.postMessage(pkt)
      }
    })()
    socketPort.start()
  })
}

function createWindow() {
  const preload = path.join(distPath, 'preload.js')
  mainWindow = new electron.BrowserWindow({
    frame: false,
    height: 680,
    width: 900,
    webPreferences: {
      sandbox: true,
      nodeIntegration: false,
      contextIsolation: true,
      preload,
    },
  })

  // installExtension(REACT_DEVELOPER_TOOLS)
  // mainWindow.loadURL('http://localhost:5100');
  mainWindow.webContents.openDevTools()

  // mainWindow.loadFile('index.html')
  mainWindow.loadURL(`${scheme}://index.html`)
  mainWindow.on('closed', () => (mainWindow = null))
}

async function startup() {
  const runtimeUuid: string = process.env['BLDR_RUNTIME_ID'] || 'default'

  initProtocol()
  setupSocket(runtimeUuid)
  setupRuntimePort()
  createWindow()
}

app.on('ready', startup)

app.on('window-all-closed', () => {
  app.quit()
})

app.on('activate', () => {
  if (mainWindow === null) {
    createWindow()
  }
})
