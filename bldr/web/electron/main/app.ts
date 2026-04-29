import os from 'os'
import path from 'path'
import electron, { ipcMain, nativeTheme, shell } from 'electron'
import { Client as SRPCClient, OpenStreamCtr, StreamConn } from 'starpc'
import type { Message } from '@aptre/protobuf-es-lite'

import { WebRuntime } from '../../bldr/web-runtime.js'
import { ServiceWorkerFetchTracker } from '../../bldr/service-worker-fetch-tracker.js'
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
} from '@go/github.com/aperturerobotics/util/pipesock/pipesock.js'
import {
  ExternalLinks,
  type ElectronInit,
} from '../../plugin/electron/electron.pb.js'

export const isMac = os.platform() === 'darwin'
// BLDR_DEBUG is set if this is a debug build.
declare const BLDR_DEBUG: boolean | undefined
export const isDebug = BLDR_DEBUG ?? false
const proxyFetchHeaderTimeoutMs = 30_000

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
  // electronInit contains initialization config from Go runtime.
  private readonly electronInit: ElectronInit

  // browserWindows contains the list of created browser windows.
  private browserWindows: Record<string, electron.BrowserWindow> = {}
  // fetchTracker aborts proxied fetches when their owning WebDocument closes.
  private readonly fetchTracker = new ServiceWorkerFetchTracker()

  // distPath is the path to the electron app dist files.
  public get distPath() {
    return this.app.getAppPath()
  }

  constructor(
    app: Electron.App,
    webRuntimeID: string,
    electronInit: ElectronInit,
  ) {
    this.app = app
    this.electronInit = electronInit

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
    const init = this.electronInit

    app.on('ready', this.onAppReady.bind(this))

    if (init.appName) {
      app.setName(init.appName)
    }

    if (init.themeSource) {
      nativeTheme.themeSource = init.themeSource as 'dark' | 'light' | 'system'
    }

    app.on('window-all-closed', () => {
      app.quit()
    })
  }

  // serviceWorkerFetch performs a request as if it was sent from the ServiceWorker.
  public serviceWorkerFetch(
    req: GlobalRequest,
    clientId?: string,
  ): Promise<GlobalResponse> {
    if (!clientId) {
      return proxyFetch(
        this.serviceWorkerHostServiceClient,
        req,
        'electron-main',
        {
          headerTimeoutMs: proxyFetchHeaderTimeoutMs,
        },
      )
    }

    const trackedFetch = this.fetchTracker.trackFetch(clientId)
    return proxyFetch(
      this.serviceWorkerHostServiceClient,
      req,
      clientId,
      {
        abortSignal: trackedFetch.abortController.signal,
        headerTimeoutMs: proxyFetchHeaderTimeoutMs,
      },
    ).finally(() => trackedFetch.release())
  }

  // onAppReady handles when the app becomes ready.
  private onAppReady() {
    // Set a custom application menu to prevent the default Electron/macOS
    // menu from intercepting keyboard shortcuts (e.g. Cmd+K) before they
    // reach the renderer's KeyboardManager.
    const menuTemplate: Electron.MenuItemConstructorOptions[] = [
      ...(isMac
        ? [{ role: 'appMenu' as const }]
        : []),
      { role: 'editMenu' as const },
      {
        label: 'View',
        submenu: [
          ...(isDebug
            ? [
                { role: 'toggleDevTools' as const },
                { type: 'separator' as const },
              ]
            : []),
          { role: 'resetZoom' as const },
          { role: 'zoomIn' as const },
          { role: 'zoomOut' as const },
          { type: 'separator' as const },
          { role: 'togglefullscreen' as const },
        ],
      },
      { role: 'windowMenu' as const },
    ]
    electron.Menu.setApplicationMenu(
      electron.Menu.buildFromTemplate(menuTemplate),
    )

    // init the app protocol for fetching index.html and .js.map files
    electron.protocol.handle(APP_SCHEME, (req) =>
      appRequestHandler(this.serviceWorkerFetch.bind(this), req),
    )

    // setup the IPC socket to the WebRuntimeHost
    this.setupWebRuntimeHostSocket()
    // setup the web runtime client port
    this.setupWebRuntimeClientPort()

    // create the first window
    this.createWebDocument({ id: 'electron-init' })
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
    const sock = connectToPipe(ipcPath, socketConn, () => {
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
  // hash is an optional URL hash to navigate to after loading (without the # prefix).
  private createWindow(webDocumentId?: string, hash?: string) {
    const init = this.electronInit
    const preload = path.join(this.distPath, 'preload.mjs')
    const nwindow = new electron.BrowserWindow({
      // Only show the OS window frame on MacOS.
      frame: isMac,
      titleBarStyle: isMac ? 'hidden' : undefined,

      title: init.windowTitle || init.appName || undefined,
      height: init.windowHeight || 680,
      width: init.windowWidth || 900,
      show: false,

      webPreferences: {
        sandbox: true,
        nodeIntegration: false,
        contextIsolation: true,
        preload,

        // Background throttling was re-enabled after fixing timeout-based lifecycle issues.
        // WebDocument uses visibility-aware reconnect with exponential backoff.
        // However, this could be set to false to prevent background throttling altogether.
        backgroundThrottling: true,
      },
    })

    if (isDebug && init.devTools) {
      nwindow.webContents.openDevTools()
    }
    nwindow.webContents.once('did-finish-load', () => {
      if (!nwindow.isDestroyed()) {
        nwindow.show()
      }
    })

    // Build URL with optional hash
    let url =
      webDocumentId ?
        `${APP_SCHEME}://index.html?webDocumentId=${encodeURIComponent(webDocumentId)}`
      : `${APP_SCHEME}://index.html`

    if (hash) {
      url += `#${hash}`
    }

    nwindow.loadURL(url)
    if (webDocumentId) {
      this.attachWebDocumentWindowLifecycle(webDocumentId, nwindow)
    }

    // Handle navigation to external URLs (clicked links)
    nwindow.webContents.on('will-navigate', (event, targetUrl) => {
      // Prevent navigation to the same URL (spurious reload).
      // This can happen during initial load when ServiceWorker isn't yet controlling.
      const currentUrl = nwindow.webContents.getURL()
      if (targetUrl === currentUrl) {
        event.preventDefault()
        return
      }

      if (!this.isInternalUrl(targetUrl)) {
        event.preventDefault()
        if (this.electronInit.externalLinks !== ExternalLinks.DENY) {
          shell.openExternal(targetUrl)
        }
        return
      }

      // SPA guard: the app only works at /index.html with hash routing.
      // If something tries to navigate to e.g. app://index.html/feed.xml,
      // block it and redirect back to the correct base URL.
      try {
        const parsed = new URL(targetUrl)
        if (parsed.pathname !== '/index.html') {
          event.preventDefault()
          const correctUrl =
            webDocumentId ?
              `${APP_SCHEME}://index.html?webDocumentId=${encodeURIComponent(webDocumentId)}`
            : `${APP_SCHEME}://index.html`
          nwindow.loadURL(correctUrl)
        }
      } catch {
        // Invalid URL, block navigation
        event.preventDefault()
      }
    })

    // Handle window.open() calls - only allow same-origin with different hash
    nwindow.webContents.setWindowOpenHandler(({ url: targetUrl }) => {
      // Handle external URLs
      if (!this.isInternalUrl(targetUrl)) {
        if (this.electronInit.externalLinks !== ExternalLinks.DENY) {
          shell.openExternal(targetUrl)
        }
        return { action: 'deny' }
      }

      try {
        const parsed = new URL(targetUrl)

        // Extract hash (remove leading #)
        const hash = parsed.hash ? parsed.hash.slice(1) : ''

        // Create popout window with preserved hash
        const popoutDocId = `popout-${Date.now()}`
        const popoutWindow = this.createWindow(popoutDocId, hash)
        this.browserWindows[popoutDocId] = popoutWindow
      } catch {
        // Invalid URL, deny
      }

      // Deny the default behavior, we handle it ourselves
      return { action: 'deny' }
    })

    return nwindow
  }

  // attachWebDocumentWindowLifecycle invalidates runtime clients for window teardown and reload.
  private attachWebDocumentWindowLifecycle(
    webDocumentId: string,
    nwindow: electron.BrowserWindow,
  ) {
    const state = { invalidated: false }
    const invalidate = (reason: string) => {
      if (state.invalidated) {
        return
      }
      state.invalidated = true
      const err = new Error(reason)
      this.abortWebDocumentFetches(webDocumentId, reason)
      this.webRuntime.invalidateClient(webDocumentId, err)
    }

    nwindow.webContents.on('did-start-navigation', (details) => {
      if (!details.isMainFrame || details.isSameDocument) {
        return
      }
      invalidate(`navigation started: ${details.url}`)
    })
    nwindow.webContents.on('render-process-gone', (_event, details) => {
      invalidate(`renderer gone: ${details.reason}`)
    })
    nwindow.on('closed', () => {
      invalidate(`window closed: ${webDocumentId}`)
      if (this.browserWindows[webDocumentId] === nwindow) {
        delete this.browserWindows[webDocumentId]
      }
    })
  }

  // abortWebDocumentFetches aborts in-flight proxied fetches for a WebDocument.
  private abortWebDocumentFetches(webDocumentId?: string, reason?: string) {
    if (!webDocumentId) {
      return
    }
    this.fetchTracker.abortClient(
      webDocumentId,
      new Error(reason ?? `web document closed: ${webDocumentId}`),
    )
  }

  // isInternalUrl checks if a URL is internal to the app.
  private isInternalUrl(url: string): boolean {
    try {
      const parsed = new URL(url)
      return parsed.protocol === `${APP_SCHEME}:`
    } catch {
      return false
    }
  }

  // runtimeCreateWebDocument is called by the WebRuntimeHost to create a new WebDocument.
  private async createWebDocument(
    req: Message<CreateWebDocumentRequest>,
  ): Promise<CreateWebDocumentResponse> {
    const id = req.id
    if (!id) {
      return { created: false }
    }
    const nwindow = this.createWindow(id)
    this.browserWindows[id] = nwindow
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
    this.abortWebDocumentFetches(req.id)
    // NOTE: the close() might not work if !closable or interrupted
    // this behaves the same as if the user clicked the X
    doc.close()
    return { removed: true }
  }
}
