import electron, { MessagePortMain, MessageChannelMain } from 'electron'
import net from 'net'
import path from 'path'
import { OpenStreamCtr, Conn, buildPushableSink } from 'starpc'
import { pushable } from 'it-pushable'
import { pipe } from 'it-pipe'

import { initProtocol, APP_SCHEME } from './protocol.js'
import { debugConsole } from './console.js'
import {
  CreateWebDocumentRequest,
  CreateWebDocumentResponse,
  RemoveWebDocumentRequest,
  RemoveWebDocumentResponse,
  WebRuntimeClientInit,
} from '../../runtime/runtime.pb.js'
import { WebRuntime } from '../../bldr/web-runtime.js'
import debugWhenReady from './debug.js'

const app = electron.app
const distPath = app.getAppPath()
const pipeWorkdir = distPath
const ipcMain: Electron.IpcMain = electron.ipcMain

function createWindow(urlSuffix?: string): electron.BrowserWindow {
  const preload = path.join(distPath, 'preload.js')
  const nwindow = new electron.BrowserWindow({
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
  nwindow.webContents.openDevTools()

  // mainWindow.loadFile('index.html')
  nwindow.loadURL(`${APP_SCHEME}://index.html${urlSuffix || ''}`)
  return nwindow
}

// mainWindow contains the main electron browser window.
let mainWindow: electron.BrowserWindow | null
// createdDocs contains the list of created browser windows.
const createdDocs: Record<string, electron.BrowserWindow> = {}

// createDocCb is called to create a new browser window.
const createDocCb = async (
  req: CreateWebDocumentRequest,
): Promise<CreateWebDocumentResponse> => {
  createdDocs[req.id] = createWindow(`#webDocumentUuid=${req.id}`)
  return { created: true }
}
// removeDocCb is called to remove a browser window.
const removeDocCb = async (
  req: RemoveWebDocumentRequest,
): Promise<RemoveWebDocumentResponse> => {
  const doc = createdDocs[req.id]
  if (!doc) {
    return { removed: false }
  }
  // NOTE: the close() might not work if !closable or interrupted
  // this behaves the same as if the user clicked the X
  delete createdDocs[req.id]
  doc.close()
  return { removed: true }
}

// openStreamCtr will contain the runtime open stream func.
const openStreamCtr = new OpenStreamCtr(undefined)
// openStreamFunc is a function that waits for OpenStreamFunc, then calls it.
const openStreamFunc = openStreamCtr.openStreamFunc

// create the WebRuntime instance
const workerHost = new WebRuntime(
  `electron:main`,
  openStreamFunc,
  createDocCb,
  removeDocCb,
)

// connect the WebRuntime to the socket ports
// setup the ipc socket
// retries if disconnected
function setupSocket(workdir: string, runtimeUuid: string) {
  if (path.extname(workdir) === '.asar') {
    workdir = path.dirname(workdir)
  }
  debugConsole.log('setupSocket', workdir, runtimeUuid)

  // see: util/pipesock
  let ipcPath: string
  if (process.platform === 'win32') {
    ipcPath = '\\\\.\\pipe\\bldr\\' + runtimeUuid
  } else {
    ipcPath = path.join(workdir, `.pipe-${runtimeUuid}`)
  }

  // socketTx is data outgoing to the socket.
  const socketTx = pushable<Uint8Array>({ objectMode: true })
  // socketRx is data incoming from the socket.
  const socketRx = pushable<Uint8Array>({ objectMode: true })

  // socketConn reads and writes to the socket.
  const socketConn = new Conn(workerHost.getWebRuntimeServer(), {
    direction: 'inbound',
  })
  const openStream = socketConn.buildOpenStreamFunc()
  pipe(socketRx, socketConn, buildPushableSink<Uint8Array>(socketTx))

  // sock is the connected socket instance
  const sock = net.connect(ipcPath, async () => {
    debugConsole.log('ipc connection opened')
    openStreamCtr.set(openStream)
    for await (const data of socketTx) {
      sock.write(data)
    }
  })
  sock.on('data', (data) => {
    socketRx.push(data)
  })
  sock.on('end', () => {
    // assume we are exiting
    debugConsole.error('ipc connection closed')
    process.exit(0)
  })
  sock.on('error', (err) => {
    debugConsole.error('ipc connection errored', err)
    // ...but also exit if this happens.
    process.exit(1)
  })
}

// convert MessagePort to a MessagePortMain.
function messagePortToMessagePortMain(port: MessagePort): MessagePortMain {
  const channel = new MessageChannelMain()
  channel.port1.on('message', (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortMainToMessagePort(port))
      port.postMessage(ev.data, ports)
    } else {
      port.postMessage(ev.data)
    }
  })
  port.onmessage = (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortToMessagePortMain(port))
      channel.port1.postMessage(ev.data, ports)
    } else {
      channel.port1.postMessage(ev.data)
    }
  }
  port.start()
  channel.port1.start()
  return channel.port2
}

// convert MessagePortMain to a MessagePort.
function messagePortMainToMessagePort(portMain: MessagePortMain): MessagePort {
  const channel = new MessageChannel()
  channel.port1.onmessage = (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortToMessagePortMain(port))
      portMain.postMessage(ev.data, ports)
    } else {
      portMain.postMessage(ev.data)
    }
  }
  portMain.on('message', (ev) => {
    if (ev.ports && ev.ports.length) {
      const ports = ev.ports.map((port) => messagePortMainToMessagePort(port))
      channel.port1.postMessage(ev.data, ports)
    } else {
      channel.port1.postMessage(ev.data)
    }
  })
  portMain.start()
  channel.port1.start()
  return channel.port2
}

// setup handler for MessagePort updates.
function setupRuntimePort() {
  ipcMain.on('BLDR_PORT', async (event, init: Uint8Array) => {
    const initMsg = WebRuntimeClientInit.decode(init)
    const clientPort = event.ports[0]
    workerHost.handleClient(initMsg, messagePortMainToMessagePort(clientPort))
  })
}

async function startup() {
  const runtimeUuid: string = process.env['BLDR_RUNTIME_ID'] || 'default'

  debugWhenReady()
  initProtocol()
  setupSocket(pipeWorkdir, runtimeUuid)
  setupRuntimePort()
  if (!mainWindow) {
    mainWindow = createWindow()
    mainWindow.on('closed', () => (mainWindow = null))
  }
}

app.on('ready', startup)

app.on('window-all-closed', () => {
  app.quit()
})

app.on('activate', () => {
  if (!mainWindow) {
    mainWindow = createWindow()
    mainWindow.on('closed', () => (mainWindow = null))
  }
})
