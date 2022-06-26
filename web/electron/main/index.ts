import electron, { MessageChannelMain } from 'electron'
import net from 'net'
import path from 'path'
import fs from 'fs'
import console from 'console'

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
let runtimePort: Electron.MessagePortMain | undefined
let socket: NodeJS.Socket | undefined

// setup the ipc socket
async function setupSocket(runtimeUuid: string) {
  const pipeName = `.pipe-${runtimeUuid}`
  let ipcPath = path.join(process.cwd(), pipeName)
  if (process.platform === 'win32') {
    ipcPath = path.join('\\\\.\\pipe', process.cwd(), pipeName)
  }

  return new Promise<void>((resolve, reject) => {
    socket = net.connect(ipcPath, () => {
      resolve()
    })
    socket.on('error', (err) => {
      debug.error('ipc connection failed', err)
      reject(err)

      // ...but also exit if this happens.
      process.exit(1)
    })
  })
}

// setup handler for MessagePort updates.
function setupRuntimePort() {
  ipcMain.on('BLDR_PORT', (event, webRuntimeUuid: string) => {
    const channel = new MessageChannelMain()
    const socketPort = channel.port1
    const remotePort = channel.port2

    // send the remote port to the web runtime
    event.sender.postMessage(webRuntimeUuid, null, [remotePort])

    // connect to the socket & start flow of messages
    runtimePort = socketPort
    const sock = socket!
    sock.removeAllListeners('data')
    sock.on('data', (data) => {
      socketPort.postMessage(data)
    })
    socketPort.on('message', (event) => {
      const data = event?.data as Uint8Array
      if (data && data.length) {
        sock.write(data)
      }
    })
    socketPort.start()
  })
}

function createWindow() {
  const preloadPath = path.join(distPath, 'preload.js')
  mainWindow = new electron.BrowserWindow({
    frame: false,
    height: 680,
    width: 900,
    webPreferences: {
      sandbox: true,
      nodeIntegration: false,
      contextIsolation: true,
      // enableRemoteModule: false,
      preload: path.join(distPath, 'preload.js'),
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
  await setupSocket(runtimeUuid)
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
