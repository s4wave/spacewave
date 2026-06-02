import os from 'os'
import path from 'path'
import http, {
  type IncomingMessage,
  type Server,
  type ServerResponse,
} from 'http'
import electron, { dialog, ipcMain, nativeTheme, shell } from 'electron'
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
import { WebRuntimeHostClient } from '../../runtime/runtime_srpc.pb.js'
import type {
  DesktopRuntimeState,
  OpenOrFocusMainWindowRequest,
} from '../desktop-runtime/desktop-runtime.pb.js'
import { APP_SCHEME, appRequestHandler } from './protocol.js'
import { ServiceWorkerHostClient } from '../../runtime/sw/sw_srpc.pb.js'
import { proxyFetch } from '../../fetch/fetch.js'
import { messagePortMainToMessagePort } from './ipc.js'
import {
  buildPipeName,
  connectToPipe,
} from '@go/github.com/aperturerobotics/util/pipesock/pipesock.js'
import {
  DesktopPresencePolicy,
  ExternalLinks,
  type ElectronInit,
} from '../../plugin/electron/electron.pb.js'
import {
  buildDesktopCLIInstallDetector,
  buildDesktopCLIInstallProbe,
  buildManagedCLIReleaseResolver,
  readManagedCLIReleaseBinary,
} from './desktop-cli-install-node.js'
import { DesktopRuntimeResource } from './desktop-runtime.js'
import {
  buildDesktopTrayEntriesFromRuntimeState,
  desktopRuntimeCLISettingsRoute,
  iconStateForRuntimeHealth,
} from './desktop-tray-runtime-projection.js'
import { DesktopTrayController } from './desktop-tray.js'

export const isMac = os.platform() === 'darwin'
// BLDR_DEBUG is set if this is a debug build.
declare const BLDR_DEBUG: boolean | undefined
export const isDebug = BLDR_DEBUG ?? false
const proxyFetchHeaderTimeoutMs = 30_000
const logRendererEvents =
  isDebug && process.env.BLDR_ELECTRON_LOG_RENDERER === '1'
const e2eControlPortEnv = 'BLDR_ELECTRON_E2E_CONTROL_PORT'

// BldrElectronApp manages the main process for an Electron app.
export class BldrElectronApp {
  // app contains the reference to the bldr electron app
  public readonly app: Electron.App
  // webRuntime is the web runtime instance.
  public readonly webRuntime: WebRuntime
  // webRuntimeHostOpenStreamCtr contains the OpenStreamFn for the WebRuntimeHost.
  // this is the Go runtime that is managing the Bldr Electron instance.
  public readonly webRuntimeHostOpenStreamCtr: OpenStreamCtr
  // webRuntimeHostClient contacts the Go WebRuntimeHost via the runtime socket.
  public readonly webRuntimeHostClient: SRPCClient
  // webRuntimeHostServiceClient is the RPC wrapper for webRuntimeHostClient.
  public readonly webRuntimeHostServiceClient: WebRuntimeHostClient
  // serviceWorkerHostClient contacts the ServiceWorkerHost via the webRuntime
  public readonly serviceWorkerHostClient: SRPCClient
  // serviceWorkerHostClient is the ServiceWorkerHost RPC wrapper for serviceWorkerHostClient.
  public readonly serviceWorkerHostServiceClient: ServiceWorkerHostClient
  // electronInit contains initialization config from Go runtime.
  private readonly electronInit: ElectronInit
  // desktopRuntimeResource exposes Electron main desktop-shell lifecycle.
  public readonly desktopRuntimeResource: DesktopRuntimeResource
  // desktopTrayController owns the process-lifetime native status icon.
  private desktopTrayController?: DesktopTrayController
  // e2eControlServer exposes test-only desktop runtime controls on loopback.
  private e2eControlServer?: Server

  // browserWindows contains the list of created browser windows.
  private browserWindows: Record<string, electron.BrowserWindow> = {}
  private routeWindowCounter = 0
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
    this.webRuntimeHostClient = new SRPCClient(
      this.webRuntimeHostOpenStreamCtr.openStreamFunc,
    )
    this.webRuntimeHostServiceClient = new WebRuntimeHostClient(
      this.webRuntimeHostClient,
    )

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

    const cliInstallProbe = buildDesktopCLIInstallProbe()
    const cliReleaseResolver = buildManagedCLIReleaseResolver(
      this.electronInit.managedCliRelease,
    )
    this.desktopRuntimeResource = new DesktopRuntimeResource({
      openOrFocusMainWindow: this.openOrFocusMainWindow.bind(this),
      quitDesktopRuntime: this.quitDesktopRuntime.bind(this),
      desktopCLIInstall: {
        detectCLIInstallState: buildDesktopCLIInstallDetector(
          cliReleaseResolver,
          cliInstallProbe,
        ),
        openCLISettings: () =>
          this.openOrFocusMainWindow({
            route:
              desktopRuntimeCLISettingsRoute(
                this.desktopRuntimeResource.getState(),
              ) || '/',
          }),
        readReleaseBinary: readManagedCLIReleaseBinary(cliReleaseResolver),
        probe: cliInstallProbe,
      },
    })
    this.webRuntime.registerServerExtension(
      this.desktopRuntimeResource.resourceServer,
    )
    void this.desktopRuntimeResource.desktopCLIInstallResource.recheck()
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

    if (app.requestSingleInstanceLock && !app.requestSingleInstanceLock()) {
      app.quit()
      return
    }

    app.on('second-instance', () => {
      void this.desktopRuntimeResource.OpenOrFocusMainWindow({})
    })
    app.on('activate', () => {
      void this.desktopRuntimeResource.OpenOrFocusMainWindow({})
    })
    app.on('before-quit', () => {
      this.desktopTrayController?.dispose()
      this.desktopRuntimeResource.setQuitting(true)
    })
    app.on('window-all-closed', this.onWindowAllClosed.bind(this))
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
    return proxyFetch(this.serviceWorkerHostServiceClient, req, clientId, {
      abortSignal: trackedFetch.abortController.signal,
      headerTimeoutMs: proxyFetchHeaderTimeoutMs,
    }).finally(() => trackedFetch.release())
  }

  // onAppReady handles when the app becomes ready.
  private onAppReady() {
    // Set a custom application menu to prevent the default Electron/macOS
    // menu from intercepting keyboard shortcuts (e.g. Cmd+K) before they
    // reach the renderer's KeyboardManager.
    const menuTemplate: Electron.MenuItemConstructorOptions[] = [
      ...(isMac ? [{ role: 'appMenu' as const }] : []),
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
    // setup native filesystem picker ipc
    this.setupNativeDirectoryPicker()
    // setup renderer desktop runtime lifecycle ipc
    this.setupDesktopRuntimeIpc()
    // setup test-only control surface for windowless Electron e2e assertions
    this.setupE2EControlServer()

    if (this.hasTrayBackgroundPresence()) {
      this.desktopTrayController = new DesktopTrayController({
        init: this.electronInit,
        resource: this.desktopRuntimeResource,
      })
      this.desktopTrayController.init()
    }

    // create the first window
    this.createWebDocument({ id: 'electron-init' })
  }

  private onWindowAllClosed() {
    if (this.hasTrayBackgroundPresence()) {
      return
    }
    this.app.quit()
  }

  private setupNativeDirectoryPicker() {
    ipcMain.handle('BLDR_ELECTRON_OPEN_DIRECTORY', async () => {
      const result = await dialog.showOpenDialog({
        properties: ['openDirectory', 'showHiddenFiles'],
      })
      if (result.canceled || result.filePaths.length === 0) {
        return null
      }
      return result.filePaths[0] ?? null
    })
  }

  private setupDesktopRuntimeIpc() {
    ipcMain.handle('BLDR_ELECTRON_QUIT_DESKTOP_RUNTIME', async () => {
      await this.desktopRuntimeResource.QuitDesktopRuntime({})
    })
  }

  private setupE2EControlServer() {
    const portRaw = process.env[e2eControlPortEnv]?.trim()
    if (!portRaw || this.e2eControlServer) {
      return
    }
    const port = Number(portRaw)
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      console.error(`${e2eControlPortEnv} must be a TCP port, got ${portRaw}`)
      return
    }

    this.e2eControlServer = http.createServer((req, res) => {
      void this.handleE2EControlRequest(req, res)
    })
    this.e2eControlServer.on('error', (err) => {
      console.error('electron e2e control server failed', err)
    })
    this.e2eControlServer.listen(port, '127.0.0.1')
    this.e2eControlServer.unref()
  }

  private async handleE2EControlRequest(
    req: IncomingMessage,
    res: ServerResponse,
  ) {
    try {
      const url = new URL(req.url ?? '/', 'http://127.0.0.1')
      if (req.method === 'GET' && url.pathname === '/desktop-state') {
        sendE2EJSON(res, 200, this.desktopRuntimeResource.getState())
        return
      }
      if (req.method === 'POST' && url.pathname === '/desktop-state') {
        const state = await readE2EJSON<DesktopRuntimeState>(req)
        await this.desktopRuntimeResource.SetDesktopState({ state })
        this.desktopRuntimeResource.desktopTrayResource.replaceStateForE2E({
          entries: buildDesktopTrayEntriesFromRuntimeState(state),
          iconState: iconStateForRuntimeHealth(state.health),
          statusText: state.statusText || 'Running',
        })
        sendE2EJSON(res, 200, { ok: true })
        return
      }
      if (req.method === 'DELETE' && url.pathname === '/desktop-state') {
        this.desktopRuntimeResource.resetProjectedDesktopStateForE2E()
        this.desktopRuntimeResource.desktopTrayResource.replaceStateForE2E(
          undefined,
        )
        sendE2EJSON(res, 200, { ok: true })
        return
      }
      if (req.method === 'GET' && url.pathname === '/tray-state') {
        sendE2EJSON(
          res,
          200,
          this.desktopRuntimeResource.desktopTrayResource.getState(),
        )
        return
      }
      if (req.method === 'POST' && url.pathname === '/native-theme') {
        const source = url.searchParams.get('source') ?? 'system'
        if (!isNativeThemeSource(source)) {
          sendE2EJSON(res, 400, { error: 'invalid native theme source' })
          return
        }
        nativeTheme.themeSource = source
        this.desktopTrayController?.setPopoverAppearanceForE2E(source)
        sendE2EJSON(res, 200, { ok: true, source })
        return
      }
      if (req.method === 'POST' && url.pathname === '/tray-popover/open') {
        const shown =
          (await this.desktopTrayController?.showPopoverForE2E()) ?? false
        sendE2EJSON(res, 200, { shown })
        return
      }
      if (req.method === 'POST' && url.pathname === '/tray-popover/close') {
        this.desktopTrayController?.closePopoverForE2E()
        sendE2EJSON(res, 200, { ok: true })
        return
      }
      if (req.method === 'GET' && url.pathname === '/tray-popover/inspection') {
        const inspection =
          await this.desktopTrayController?.inspectPopoverForE2E()
        if (!inspection) {
          sendE2EJSON(res, 404, { error: 'tray popover is not open' })
          return
        }
        sendE2EJSON(res, 200, inspection)
        return
      }
      if (req.method === 'GET' && url.pathname === '/tray-popover/screenshot') {
        const png = await this.desktopTrayController?.capturePopoverPNGForE2E()
        if (!png) {
          sendE2EJSON(res, 404, { error: 'tray popover is not open' })
          return
        }
        sendE2EBinary(res, 200, 'image/png', png)
        return
      }
      if (req.method === 'POST' && url.pathname === '/open-or-focus') {
        await this.desktopRuntimeResource.OpenOrFocusMainWindow({
          route: url.searchParams.get('route') || undefined,
        })
        sendE2EJSON(res, 200, { ok: true })
        return
      }
      if (req.method === 'POST' && url.pathname === '/activate') {
        this.app.emit('activate')
        sendE2EJSON(res, 200, { ok: true })
        return
      }
      if (req.method === 'POST' && url.pathname === '/quit') {
        await this.desktopRuntimeResource.QuitDesktopRuntime({})
        sendE2EJSON(res, 200, { ok: true })
        return
      }
      sendE2EJSON(res, 404, { error: 'not found' })
    } catch (err) {
      sendE2EJSON(res, 500, {
        error: err instanceof Error ? err.message : String(err),
      })
    }
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
    if (logRendererEvents) {
      const label = webDocumentId ?? 'main'
      nwindow.webContents.on('console-message', (event) => {
        const { level, message, sourceId, lineNumber } = event
        console.log(
          `[renderer:${label}:console:${level}] ${message} (${sourceId}:${lineNumber})`,
        )
      })
      nwindow.webContents.on('did-navigate-in-page', (_event, url) => {
        console.log(`[renderer:${label}:navigate-in-page] ${url}`)
      })
    }
    nwindow.webContents.once('did-finish-load', () => {
      if (!nwindow.isDestroyed()) {
        nwindow.show()
      }
    })

    // Build URL with optional hash
    nwindow.loadURL(this.buildWindowUrl(webDocumentId, hash))
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
          nwindow.loadURL(this.buildWindowUrl(webDocumentId))
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
      if (webDocumentId === 'electron-init') {
        this.desktopRuntimeResource.setMainWindowOpen(false)
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

  private async openOrFocusMainWindow(request?: OpenOrFocusMainWindowRequest) {
    const routeHash = this.normalizeRouteHash(request?.route)
    if (routeHash) {
      await this.createRouteWindow(routeHash)
      return
    }

    const nwindow = this.browserWindows['electron-init']
    if (!nwindow || nwindow.isDestroyed()) {
      await this.createWebDocument({ id: 'electron-init' })
      return
    }

    if (nwindow.isMinimized()) {
      nwindow.restore()
    }
    nwindow.show()
    nwindow.focus()
  }

  private async quitDesktopRuntime() {
    try {
      await this.webRuntimeHostServiceClient.RequestRuntimeQuit({})
    } catch (err) {
      console.error('failed to request host runtime quit', err)
      this.app.quit()
    }
  }

  private hasTrayBackgroundPresence(): boolean {
    return (
      this.electronInit.desktopPresencePolicy ===
      DesktopPresencePolicy.TRAY_BACKGROUND
    )
  }

  private async createRouteWindow(routeHash: string): Promise<void> {
    this.routeWindowCounter += 1
    await this.createWebDocument(
      { id: `electron-route-${this.routeWindowCounter}` },
      routeHash,
    )
  }

  private buildWindowUrl(webDocumentId?: string, hash?: string): string {
    let url = webDocumentId
      ? `${APP_SCHEME}://index.html?webDocumentId=${encodeURIComponent(webDocumentId)}`
      : `${APP_SCHEME}://index.html`

    if (hash) {
      url += `#${hash}`
    }
    return url
  }

  private normalizeRouteHash(route?: string): string {
    if (!route) {
      return ''
    }
    if (route.startsWith('#')) {
      return route.slice(1)
    }
    return route
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
    hash?: string,
  ): Promise<CreateWebDocumentResponse> {
    const id = req.id
    if (!id) {
      return { created: false }
    }
    const nwindow = this.createWindow(id, hash)
    this.browserWindows[id] = nwindow
    if (id === 'electron-init') {
      this.desktopRuntimeResource.setMainWindowOpen(true)
    }
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

function sendE2EJSON(res: ServerResponse, statusCode: number, value: unknown) {
  res.statusCode = statusCode
  res.setHeader('content-type', 'application/json')
  res.end(
    JSON.stringify(value, (_key, val) =>
      typeof val === 'bigint' ? val.toString() : val,
    ),
  )
}

async function readE2EJSON<T>(req: IncomingMessage): Promise<T> {
  const chunks: Buffer[] = []
  for await (const chunk of req) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk))
  }
  const raw = Buffer.concat(chunks).toString('utf8').trim()
  return raw ? (JSON.parse(raw) as T) : ({} as T)
}

function isNativeThemeSource(
  source: string,
): source is Electron.NativeTheme['themeSource'] {
  return source === 'system' || source === 'light' || source === 'dark'
}

function sendE2EBinary(
  res: ServerResponse,
  statusCode: number,
  contentType: string,
  value: Buffer,
) {
  res.statusCode = statusCode
  res.setHeader('content-type', contentType)
  res.end(value)
}
