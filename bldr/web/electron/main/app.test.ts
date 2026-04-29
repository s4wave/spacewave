import { EventEmitter } from 'events'
import { beforeEach, describe, expect, it, vi } from 'vitest'

Reflect.set(globalThis, 'BLDR_DEBUG', false)

const browserWindows: MockBrowserWindow[] = []
const webRuntimeInstances: MockWebRuntime[] = []
const mockElectronApp = {
  getAppPath() {
    return '/app'
  },
  on: vi.fn(),
  quit: vi.fn(),
  setName: vi.fn(),
}

class MockWebContents extends EventEmitter {
  private currentUrl = ''

  public readonly openDevTools = vi.fn()
  public readonly setWindowOpenHandler = vi.fn()

  public getURL() {
    return this.currentUrl
  }

  public setURL(url: string) {
    this.currentUrl = url
  }
}

class MockBrowserWindow extends EventEmitter {
  public readonly webContents = new MockWebContents()
  public readonly show = vi.fn()
  public readonly isDestroyed = vi.fn(() => false)
  public readonly opts: object
  public readonly loadURL = vi.fn((url: string) => {
    this.webContents.setURL(url)
  })

  constructor(opts: object = {}) {
    super()
    this.opts = opts
  }
}

class MockWebRuntime {
  public readonly removeConnection = vi.fn()
  public readonly invalidateClient = vi.fn((clientId: string) => {
    this.removeConnection(clientId)
  })
  public readonly openServiceWorkerHostStream = vi.fn()
  public readonly getWebRuntimeServer = vi.fn()
  public readonly handleClient = vi.fn()

  constructor(public readonly webRuntimeId: string) {
    webRuntimeInstances.push(this)
  }
}

vi.mock('electron', () => {
  const Menu = {
    buildFromTemplate: vi.fn(() => ({})),
    setApplicationMenu: vi.fn(),
  }
  const protocol = {
    handle: vi.fn(),
    registerSchemesAsPrivileged: vi.fn(),
  }
  const shell = {
    openExternal: vi.fn(),
  }
  const ipcMain = {
    on: vi.fn(),
  }
  const nativeTheme = {
    themeSource: 'system',
  }
  class BrowserWindow extends MockBrowserWindow {
    constructor(opts: object = {}) {
      super(opts)
      browserWindows.push(this)
    }
  }
  return {
    default: {
      app: mockElectronApp,
      BrowserWindow,
      Menu,
      protocol,
      shell,
      ipcMain,
      nativeTheme,
    },
    app: mockElectronApp,
    BrowserWindow,
    Menu,
    protocol,
    shell,
    ipcMain,
    nativeTheme,
  }
})

vi.mock('starpc', () => ({
  Client: class {},
  OpenStreamCtr: class {
    public readonly openStreamFunc = vi.fn()
    public readonly set = vi.fn()
  },
  StreamConn: class {
    constructor() {}
    public readonly buildOpenStreamFunc = vi.fn()
  },
}))

vi.mock('../../bldr/web-runtime.js', () => ({
  WebRuntime: MockWebRuntime,
}))

vi.mock('../../runtime/sw/sw_srpc.pb.js', () => ({
  ServiceWorkerHostClient: class {},
}))

vi.mock('../../fetch/fetch.js', () => ({
  proxyFetch: vi.fn(),
}))

vi.mock('./ipc.js', () => ({
  messagePortMainToMessagePort: vi.fn(),
}))

vi.mock('@go/github.com/aperturerobotics/util/pipesock/pipesock.js', () => ({
  buildPipeName: vi.fn(() => '/tmp/mock-pipe'),
  connectToPipe: vi.fn(() => ({
    on: vi.fn(),
  })),
}))

describe('BldrElectronApp', () => {
  beforeEach(() => {
    Reflect.set(globalThis, 'BLDR_DEBUG', false)
    browserWindows.length = 0
    webRuntimeInstances.length = 0
    vi.clearAllMocks()
    vi.resetModules()
  })

  it('invalidates the document runtime client on main-frame reload', async () => {
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      {},
    ])
    await createWebDocument(app, 'electron-init')

    const runtime = webRuntimeInstances[0]
    const win = browserWindows[0]
    expect(runtime).toBeDefined()
    expect(win).toBeDefined()

    win.webContents.emit('did-start-navigation', {
      isMainFrame: true,
      isSameDocument: false,
      url: 'app://index.html?webDocumentId=electron-init',
    })
    win.webContents.emit('did-start-navigation', {
      isMainFrame: true,
      isSameDocument: false,
      url: 'app://index.html?webDocumentId=electron-init',
    })

    expect(runtime.removeConnection).toHaveBeenCalledTimes(1)
    expect(runtime.removeConnection).toHaveBeenCalledWith('electron-init')
  })

  it('ignores same-document navigation and invalidates on renderer loss', async () => {
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      {},
    ])
    await createWebDocument(app, 'electron-init')

    const runtime = webRuntimeInstances[0]
    const win = browserWindows[0]

    win.webContents.emit('did-start-navigation', {
      isMainFrame: true,
      isSameDocument: true,
      url: 'app://index.html?webDocumentId=electron-init#/feed',
    })
    expect(runtime.removeConnection).not.toHaveBeenCalled()

    win.webContents.emit('render-process-gone', {}, { reason: 'crashed' })
    expect(runtime.removeConnection).toHaveBeenCalledTimes(1)
    expect(runtime.removeConnection).toHaveBeenCalledWith('electron-init')
  })

  it('drops BrowserWindow ownership when the window closes', async () => {
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      {},
    ])
    await createWebDocument(app, 'electron-init')

    const win = browserWindows[0]
    expect(getBrowserWindow(app, 'electron-init')).toBe(win)

    win.emit('closed')

    expect(getBrowserWindow(app, 'electron-init')).toBeUndefined()
  })

  it('does not open DevTools from release config alone', async () => {
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      { devTools: true },
    ])
    await createWebDocument(app, 'electron-init')

    expect(browserWindows[0]?.webContents.openDevTools).not.toHaveBeenCalled()
  })

  it('shows windows only after the renderer finishes loading', async () => {
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      {},
    ])
    await createWebDocument(app, 'electron-init')

    const win = browserWindows[0]
    expect(win?.opts).toMatchObject({ show: false })
    expect(win?.show).not.toHaveBeenCalled()

    win?.webContents.emit('did-finish-load')
    expect(win?.show).toHaveBeenCalledTimes(1)
  })

  it('opens DevTools only when debug build enables them', async () => {
    Reflect.set(globalThis, 'BLDR_DEBUG', true)
    vi.resetModules()
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      { devTools: true },
    ])
    await createWebDocument(app, 'electron-init')

    expect(browserWindows[0]?.webContents.openDevTools).toHaveBeenCalledTimes(1)
  })

  it('quits the Electron main process when all windows are closed', async () => {
    const { BldrElectronApp } = await import('./app.js')
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      'runtime-1',
      {},
    ])
    Reflect.apply(Reflect.get(app, 'init'), app, [])

    const handler = getAppHandler('window-all-closed')
    handler()

    expect(mockElectronApp.quit).toHaveBeenCalledTimes(1)
  })
})

async function createWebDocument(app: object, id: string) {
  const create = Reflect.get(app, 'createWebDocument')
  if (typeof create !== 'function') {
    throw new Error('createWebDocument not found')
  }
  await Reflect.apply(create, app, [{ id }])
}

function getBrowserWindow(app: object, id: string) {
  const windows = Reflect.get(app, 'browserWindows')
  if (!windows || typeof windows !== 'object') {
    throw new Error('browserWindows not found')
  }
  return Reflect.get(windows, id)
}

function getAppHandler(event: string) {
  const match = mockElectronApp.on.mock.calls.find(([name]) => name === event)
  const handler = match?.[1]
  if (typeof handler !== 'function') {
    throw new Error(`${event} handler not found`)
  }
  return handler
}
