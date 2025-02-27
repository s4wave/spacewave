import os from 'os'
import path from 'path'
import electron, { ipcMain, nativeTheme } from 'electron'
import { Client as SRPCClient, OpenStreamCtr, StreamConn } from 'starpc'
import type { Message } from '@aptre/protobuf-es-lite'

import { WebRuntime } from '../../bldr/web-runtime.js'
import {
  CreateWebDocumentRequest,
  CreateWebDocumentResponse,
  RemoveWebDocumentRequest,
  RemoveWebDocumentResponse,
  WebRuntimeClientInit,
} from '../../runtime/runtime.pb.js'
import { APP_SCHEME, appRequestHandler } from './protocol.js'
import { ServiceWorkerHostClient } from '../../runtime/sw/sw_srpc.pb.js'
import { proxyFetch } from '../../fetch/fetch.js'
import { messagePortMainToMessagePort } from './ipc.js'
import {
  buildPipeName,
  connectToPipe,
} from '../../../util/pipesock/pipesock.js'

export const isMac = os.platform() === 'darwin'
// BLDR_DEBUG is set if this is a debug build.
declare const BLDR_DEBUG: boolean | undefined
export const isDebug = BLDR_DEBUG ?? false

// BldrElectronApp manages the main process for an Electron app.
export class BldrElectronApp {
  // app contains the reference to the bldr electron app
  public readonly app: Electron.App
  // webRuntime is the web runtime instance.
  public readonly webRuntime: WebRuntime
  // webRuntimeHostOpenStreamCtr contains the OpenStreamFn for the WebRuntimeHost.
  // this is the Go runtime that is managing the Bldr Electron instance.
  public readonly webRuntimeHostOpenStreamCtr: OpenStreamCtr
  // serviceWorkerHostClient contacts the ServiceWorkerHost via the webRuntime
  public readonly serviceWorkerHostClient: SRPCClient
  // serviceWorkerHostClient is the ServiceWorkerHost RPC wrapper for serviceWorkerHostClient.
  public readonly serviceWorkerHostServiceClient: ServiceWorkerHostClient

  // browserWindows contains the list of created browser windows.
  private browserWindows: Record<string, electron.BrowserWindow> = {}

  // distPath is the path to the electron app dist files.
  public get distPath() {
    return this.app.getAppPath()
  }

  constructor(app: Electron.App, webRuntimeID: string) {
    this.app = app

    // openStreamCtr will contain the runtime open stream func.
    this.webRuntimeHostOpenStreamCtr = new OpenStreamCtr(undefined)

    this.webRuntime = new WebRuntime(
      webRuntimeID,
      this.webRuntimeHostOpenStreamCtr.openStreamFunc,
      this.createWebDocument.bind(this),
      this.removeWebDocument.bind(this),
    )

    // swHostClient contacts the ServiceWorkerHost via the webRuntime.
    this.serviceWorkerHostClient = new SRPCClient(() =>
      this.webRuntime.openServiceWorkerHostStream(webRuntimeID),
    )

    // swHost is the RPC client for the ServiceWorkerHost.
    this.serviceWorkerHostServiceClient = new ServiceWorkerHostClient(
      this.serviceWorkerHostClient,
    )
  }

  // init initializes the app
  public init() {
    const app = this.app

    app.on('ready', this.onAppReady.bind(this))

    // dark mode
    nativeTheme.themeSource = 'dark' // TODO: allow overriding this

    /*
    app.on('window-all-closed', () => {
      // TODO: notify web runtime that all windows were closed
      app.quit()
    })
    */
  }

  // serviceWorkerFetch performs a request as if it was sent from the ServiceWorker.
  public serviceWorkerFetch(
    req: GlobalRequest,
    clientId?: string,
  ): Promise<GlobalResponse> {
    return proxyFetch(
      this.serviceWorkerHostServiceClient,
      req,
      clientId ?? 'electron:main',
    )
  }

  // onAppReady handles when the app becomes ready.
  private onAppReady() {
    // init the app protocol for fetching index.html and .js.map files
    electron.protocol.handle(APP_SCHEME, (req) =>
      appRequestHandler(this.serviceWorkerFetch.bind(this), req),
    )

    // setup the IPC socket to the WebRuntimeHost
    this.setupWebRuntimeHostSocket()
    // setup the web runtime client port
    this.setupWebRuntimeClientPort()

    // create the first window
    this.createWebDocument({ id: 'electron:init' })
  }

  private setupWebRuntimeClientPort() {
    ipcMain.on('BLDR_ELECTRON_CLIENT_OPEN', async (event, init: Uint8Array) => {
      const initMsg = WebRuntimeClientInit.fromBinary(init)
      const clientPort = event.ports[0]
      this.webRuntime.handleClient(
        initMsg,
        messagePortMainToMessagePort(clientPort),
      )
    })
  }

  // setupWebRuntimeHostSocket sets up the socket to the WebRuntimeHost.
  private setupWebRuntimeHostSocket() {
    // workdir is the directory we will look for the socket
    const runtimeUuid = this.webRuntime.webRuntimeId
    let workdir = this.distPath
    if (path.extname(workdir) === '.asar') {
      workdir = path.dirname(workdir)
    }

    // Build the IPC path using the pipesock utility
    const ipcPath = buildPipeName(workdir, runtimeUuid)

    // socketConn reads and writes to the socket.
    const socketConn = new StreamConn(this.webRuntime.getWebRuntimeServer(), {
      direction: 'inbound',
    })

    // Connect to the pipe and set up bidirectional communication
    const sock = connectToPipe(ipcPath, socketConn, (_connection) => {
      this.webRuntimeHostOpenStreamCtr.set(socketConn.buildOpenStreamFunc())
    })

    // Handle socket end (process exit)
    sock.on('end', () => {
      // assume we are exiting
      process.exit(0)
    })

    // Handle socket errors (process exit with error)
    sock.on('error', (err) => {
      console.error(err)
      // ...but also exit if this happens.
      process.exit(1)
    })
  }

  // createWindow creates a new browser window.
  private createWindow() {
    const preload = path.join(this.distPath, 'preload.mjs')
    const nwindow = new electron.BrowserWindow({
      // Only show the OS window frame on MacOS.
      frame: isMac,
      titleBarStyle: isMac ? 'hidden' : undefined,

      height: 680,
      width: 900,

      webPreferences: {
        sandbox: true,
        nodeIntegration: false,
        contextIsolation: true,
        preload,

        // Disable background throttling to avoid WebDocument timeouts.
        // We will still unload the WebViews when the document is hidden.
        backgroundThrottling: false,
      },
    })

    nwindow.webContents.openDevTools()
    nwindow.loadURL(`${APP_SCHEME}://index.html`)

    return nwindow
  }

  // runtimeCreateWebDocument is called by the WebRuntimeHost to create a new WebDocument.
  private async createWebDocument(
    req: Message<CreateWebDocumentRequest>,
  ): Promise<CreateWebDocumentResponse> {
    const id = req.id
    if (!id) {
      return { created: false }
    }
    const nwindow = this.createWindow()
    this.browserWindows[id] = nwindow
    nwindow.on('closed', () => {
      if (this.browserWindows[id] === nwindow) {
        delete this.browserWindows[id]
        this.webRuntime.removeConnection(id)
      }
    })
    return { created: true }
  }

  // runtimeRemoveWebDocument is called to remove a browser window.
  private async removeWebDocument(
    req: Message<RemoveWebDocumentRequest>,
  ): Promise<RemoveWebDocumentResponse> {
    const doc = req.id && this.browserWindows[req.id]
    if (!doc) {
      return { removed: false }
    }
    // NOTE: the close() might not work if !closable or interrupted
    // this behaves the same as if the user clicked the X
    doc.close()
    return { removed: true }
  }
}
