import { EventEmitter } from 'events'
import { createHandler } from 'starpc'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { Client as ResourceClient } from '../../../sdk/resource/client.js'
import { newResourceMux } from '../../../sdk/resource/server/mux.js'
import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  type DesktopTrayState,
  type WatchDesktopTrayResponse,
  type DesktopTrayEntry,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import {
  DesktopTrayActionHandlerServiceDefinition,
  DesktopTrayResourceServiceClient,
  type DesktopTrayActionHandlerService,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray_srpc.pb.js'
import {
  DesktopRuntimeActionKind,
  DesktopRuntimeHealth,
  DesktopRuntimeSeverity,
  type DesktopRuntimeActionItem,
  type DesktopRuntimeActivityItem,
  type DesktopRuntimeAttentionItem,
  type DesktopRuntimeNavigationItem,
  DesktopRuntimeState,
  type WatchDesktopStateResponse,
} from '../desktop-runtime/desktop-runtime.pb.js'
import { DesktopRuntimeResource } from './desktop-runtime.js'
import {
  buildDesktopTrayEntriesFromRuntimeState,
  iconStateForRuntimeHealth,
} from './desktop-tray-runtime-projection.js'

const platformState = { value: 'linux' }
const menuTemplates: Electron.MenuItemConstructorOptions[][] = []
const trayInstances: MockTray[] = []
const browserWindows: MockBrowserWindow[] = []
const notifications: MockNotification[] = []
const mockGlobalShortcut = {
  register: vi.fn((_shortcut: string, _handler: () => void) => true),
  unregister: vi.fn(),
}
const mockClipboard = {
  writeText: vi.fn(),
}
const mockShell = {
  showItemInFolder: vi.fn(),
}
const mockScreen = {
  getDisplayMatching: vi.fn(() => ({
    workArea: { x: 0, y: 0, width: 1440, height: 900 },
  })),
}
const mockResource = {
  WatchDesktopState: vi.fn(),
  getState: vi.fn(() => defaultRuntimeState()),
  OpenOrFocusMainWindow: vi.fn(() => Promise.resolve({})),
  QuitDesktopRuntime: vi.fn(() => Promise.resolve({})),
  desktopTrayResource: {
    WatchDesktopTray: vi.fn(),
    InvokeDesktopTrayEntry: vi.fn(() => Promise.resolve({})),
    getState: vi.fn(() => defaultTrayState()),
  },
}
const browserWindowState = {
  shouldThrow: false,
}
let emitState: (state: DesktopRuntimeState) => void = () => {}
let emitTrayState: (state: DesktopTrayState) => void = () => {}

class MockNativeImage {
  public readonly setTemplateImage = vi.fn()
}

class MockTray extends EventEmitter {
  public readonly setToolTip = vi.fn()
  public readonly setContextMenu = vi.fn()
  public readonly setImage = vi.fn()
  public readonly setTitle = vi.fn()
  public readonly destroy = vi.fn()
  public readonly getBounds = vi.fn(() => ({
    x: 100,
    y: 24,
    width: 24,
    height: 24,
  }))

  constructor(public readonly image: MockNativeImage) {
    super()
    trayInstances.push(this)
  }
}

class MockNotification {
  public static readonly isSupported = vi.fn(() => true)
  public readonly show = vi.fn()

  constructor(public readonly opts: Electron.NotificationConstructorOptions) {
    notifications.push(this)
  }
}

class MockBrowserWindow extends EventEmitter {
  public readonly webContents = new EventEmitter()
  public readonly loadURL = vi.fn((_url: string) => Promise.resolve())
  public readonly capturePage = vi.fn(() =>
    Promise.resolve({ toPNG: () => Buffer.from('popover-png') }),
  )
  public readonly setBounds = vi.fn()
  public readonly show = vi.fn()
  public readonly close = vi.fn(() => {
    this.destroyed = true
    this.emit('closed')
  })
  public readonly isDestroyed = vi.fn(() => this.destroyed)
  private destroyed = false

  constructor(public readonly opts: Electron.BrowserWindowConstructorOptions) {
    super()
    if (browserWindowState.shouldThrow) {
      throw new Error('popover unavailable')
    }
    browserWindows.push(this)
  }
}

vi.mock('os', () => ({
  default: {
    platform: () => platformState.value,
  },
}))

vi.mock('electron', () => {
  const nativeImage = {
    createEmpty: vi.fn(() => new MockNativeImage()),
    createFromDataURL: vi.fn(() => new MockNativeImage()),
    createFromPath: vi.fn(() => new MockNativeImage()),
  }
  const Menu = {
    buildFromTemplate: vi.fn(
      (template: Electron.MenuItemConstructorOptions[]) => {
        menuTemplates.push(template)
        return { template }
      },
    ),
  }
  return {
    default: {
      Tray: MockTray,
      BrowserWindow: MockBrowserWindow,
      Menu,
      Notification: MockNotification,
      clipboard: mockClipboard,
      globalShortcut: mockGlobalShortcut,
      nativeImage,
      screen: mockScreen,
      shell: mockShell,
    },
    Tray: MockTray,
    BrowserWindow: MockBrowserWindow,
    Menu,
    Notification: MockNotification,
    clipboard: mockClipboard,
    globalShortcut: mockGlobalShortcut,
    nativeImage,
    screen: mockScreen,
    shell: mockShell,
  }
})

describe('DesktopTrayController', () => {
  beforeEach(() => {
    platformState.value = 'linux'
    menuTemplates.length = 0
    trayInstances.length = 0
    browserWindows.length = 0
    notifications.length = 0
    browserWindowState.shouldThrow = false
    delete process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER
    delete process.env.BLDR_ELECTRON_DESKTOP_TRAY_DYNAMIC_ICON
    delete process.env.BLDR_ELECTRON_DESKTOP_TRAY_NOTIFICATIONS
    delete process.env.BLDR_ELECTRON_DESKTOP_TRAY_TOGGLE_SHORTCUT
    vi.clearAllMocks()
    const state = defaultRuntimeState()
    setMockRuntimeState(state)
    const stream = new TestStateStream()
    const trayStream = new TestTrayStateStream()
    emitState = (state: DesktopRuntimeState) => {
      stream.emit({ state })
      const trayState = trayStateFromRuntimeState(state)
      mockResource.desktopTrayResource.getState.mockReturnValue(trayState)
      trayStream.emit({ state: trayState })
    }
    emitTrayState = (state: DesktopTrayState) => {
      mockResource.desktopTrayResource.getState.mockReturnValue(state)
      trayStream.emit({ state })
    }
    mockResource.WatchDesktopState.mockReturnValue(stream)
    mockResource.desktopTrayResource.WatchDesktopTray.mockReturnValue(
      trayStream,
    )
  })

  it('keeps one native tray item for the process lifetime', async () => {
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave', trayIconPath: '/icons/tray.png' },
      resource: mockResource,
    })

    controller.init()
    controller.init()

    expect(trayInstances).toHaveLength(1)
    expect(trayInstances[0]?.setToolTip).toHaveBeenCalledWith(
      'Spacewave: Running',
    )
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
    expect(menuTemplates[0]).toContainEqual({
      label: 'Spacewave: Running',
      enabled: false,
    })
    expect(
      mockResource.desktopTrayResource.WatchDesktopTray,
    ).toHaveBeenCalledTimes(1)
    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()
  })

  it('rebuilds the native menu from desktop runtime state updates', async () => {
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await Promise.resolve()
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)

    const disconnected = {
      ...defaultRuntimeState(),
      statusText: 'Disconnected',
    }
    setMockRuntimeState(disconnected)
    emitState(disconnected)
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(menuTemplates[1]).toContainEqual({
      label: 'Spacewave: Disconnected',
      enabled: false,
    })
  })

  it('consumes the Electron-scoped DesktopTrayResource as the native menu source', async () => {
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    emitTrayState(defaultTrayState())
    await Promise.resolve()
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)

    emitTrayState({
      statusText: 'Tray Source',
      iconState: DesktopTrayIconState.ACTIVE,
      entries: [
        {
          id: 'tray-source',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'From DesktopTray',
        },
      ],
    })
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(menuTemplates[1]).toContainEqual({
      label: 'From DesktopTray',
      enabled: false,
    })
    expect(trayInstances[0]?.setToolTip).toHaveBeenLastCalledWith(
      'Spacewave: Tray Source',
    )
  })

  it('does not rebuild the native menu for duplicate tray snapshots', async () => {
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    const state: DesktopTrayState = {
      statusText: 'Stable',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'title',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Spacewave: Stable',
        },
        {
          id: 'open',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Open Spacewave',
          enabled: true,
          action: { kind: DesktopTrayActionKind.OPEN_ROUTE },
        },
      ],
    }

    emitTrayState(state)
    await Promise.resolve()
    emitTrayState({
      ...state,
      entries: state.entries?.map((entry) => ({ ...entry })),
    })
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(menuTemplates).toHaveLength(2)
    expect(templateLabels(menuTemplates[1])).toEqual([
      'Spacewave: Stable',
      'Open Spacewave',
    ])
  })

  it('records the tray resource to native menu path', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource,
    })
    controller.init()
    await flushPromises()

    const abort = new AbortController()
    const resourceClient = new ResourceClient(
      resource.resourceServer,
      abort.signal,
    )
    const rootRef = await resourceClient.accessRootResource()
    const tray = new DesktopTrayResourceServiceClient(rootRef.client)

    await tray.RegisterDesktopTrayEntry({
      entry: {
        id: 'status-runtime',
        kind: DesktopTrayEntryKind.STATUS,
        label: 'CLI reachable - 1 CLI client connected',
        order: 0,
      },
    })
    await tray.RegisterDesktopTrayEntry({
      entry: {
        id: 'navigation-session-1',
        kind: DesktopTrayEntryKind.ACTION,
        label: 'coolguy@spacewave.app - Cloud - Ready',
        order: 1,
        enabled: true,
        action: { kind: DesktopTrayActionKind.OPEN_ROUTE, route: '/u/1/' },
      },
    })
    await tray.RegisterDesktopTrayEntry({
      entry: {
        id: 'navigation-drive',
        kind: DesktopTrayEntryKind.ACTION,
        label: 'Drive - Open',
        order: 2,
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.OPEN_ROUTE,
          route: '/u/1/so/drive',
        },
      },
    })
    await flushPromises()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(4)
    expect(templateLabels(menuTemplates[3])).toContain(
      'CLI reachable - 1 CLI client connected',
    )
    expect(templateLabels(menuTemplates[3])).toContain(
      'coolguy@spacewave.app - Cloud - Ready',
    )
    expect(templateLabels(menuTemplates[3])).toContain('Drive - Open')

    rootRef.release()
    resourceClient.dispose()
    abort.abort()
  })

  it('renders healthy menu sections in daemon-console order', async () => {
    const state = {
      ...defaultRuntimeState(),
      statusText: 'Running',
      listener: {
        label: 'CLI reachable',
        detail: '1 CLI client connected',
        socketPath: '/tmp/spacewave.sock',
      },
      sessions: [
        {
          label: 'coolguy@spacewave.app',
          detail: 'Cloud',
          statusText: 'Ready',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      spaces: [
        {
          label: 'Project Alpha',
          detail: 'coolguy@spacewave.app',
          statusText: 'Shared',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      activity: [
        {
          label: 'Uploading changes',
          detail: '2 sync items',
        },
      ] satisfies DesktopRuntimeActivityItem[],
      actions: [
        {
          label: 'Copy diagnostics',
          detail: 'CLI socket',
          enabled: true,
        },
      ] satisfies DesktopRuntimeActionItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()

    expect(templateLabels(menuTemplates[0])).toEqual([
      'Spacewave: Running',
      '---',
      'Open Spacewave',
      'New Window',
      '---',
      'Status',
      'CLI reachable - 1 CLI client connected',
      '---',
      'Sessions',
      'coolguy@spacewave.app - Cloud - Ready',
      '---',
      'Spaces',
      'Project Alpha - coolguy@spacewave.app - Shared',
      '---',
      'Activity',
      'Uploading changes - 2 sync items',
      '---',
      'Quick Actions',
      'Copy Socket Path',
      'Copy Diagnostics',
      'Copy diagnostics - CLI socket',
      '---',
      'Settings...',
      'About Spacewave',
      '---',
      'Quit',
    ])
  })

  it('routes menu rows through the desktop runtime resource', async () => {
    const state = {
      ...defaultRuntimeState(),
      sessions: [
        {
          label: 'coolguy@spacewave.app',
          route: '/u/2/',
          active: true,
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      spaces: [
        {
          label: 'Project Alpha',
          route: '/u/2/so/project-alpha',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      actions: [
        {
          kind: DesktopRuntimeActionKind.OPEN_ROUTE,
          label: 'Open dashboard',
          route: '/u/2/',
          enabled: true,
        },
      ] satisfies DesktopRuntimeActionItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickMenuItem('Open Spacewave')
    await clickMenuItem('New Window')
    await clickMenuItem('coolguy@spacewave.app')
    await clickMenuItem('Project Alpha')
    await clickMenuItem('Open dashboard')
    await clickMenuItem('Settings...')
    await clickMenuItem('About Spacewave')

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(1, {})
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(2, {
      route: '/',
    })
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(3, {
      route: '/u/2/',
    })
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(4, {
      route: '/u/2/so/project-alpha',
    })
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(5, {
      route: '/u/2/',
    })
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(6, {
      route: '/u/2/settings/cli',
    })
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(7, {
      route: '/about',
    })
  })

  it('dispatches copy and reveal quick actions through native shell APIs', async () => {
    const state = {
      ...defaultRuntimeState(),
      listener: {
        label: 'CLI reachable',
        detail: '1 CLI client connected',
        socketPath: '/tmp/spacewave.sock',
      },
      actions: [
        {
          kind: DesktopRuntimeActionKind.COPY_TEXT,
          label: 'Copy custom diagnostics',
          value: 'custom diagnostics',
          enabled: true,
        },
        {
          kind: DesktopRuntimeActionKind.REVEAL_PATH,
          label: 'Reveal State Root',
          value: '/Users/cjs/Library/Application Support/Spacewave',
          enabled: true,
        },
      ] satisfies DesktopRuntimeActionItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickMenuItem('Copy Socket Path')
    await clickMenuItem('Copy Diagnostics')
    await clickMenuItem('Copy custom diagnostics')
    await clickMenuItem('Reveal State Root')

    expect(mockClipboard.writeText).toHaveBeenNthCalledWith(
      1,
      '/tmp/spacewave.sock',
    )
    expect(mockClipboard.writeText).toHaveBeenNthCalledWith(
      2,
      'Spacewave: Running\nCLI reachable - 1 CLI client connected\nSocket: /tmp/spacewave.sock',
    )
    expect(mockClipboard.writeText).toHaveBeenNthCalledWith(
      3,
      'custom diagnostics',
    )
    expect(mockShell.showItemInFolder).toHaveBeenCalledWith(
      '/Users/cjs/Library/Application Support/Spacewave',
    )
  })

  it('keeps unsupported recovery actions deferred', async () => {
    const state = {
      ...defaultRuntimeState(),
      actions: [
        {
          kind: DesktopRuntimeActionKind.UNSPECIFIED,
          label: 'Restart Runtime',
          enabled: true,
        },
      ] satisfies DesktopRuntimeActionItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    const item = menuTemplates[0]?.find(
      (entry) => entry.label === 'Restart Runtime',
    )

    expect(item).toMatchObject({ enabled: false })
    expect(item?.click).toBeUndefined()
  })

  it('collapses attention mode to the highest-priority item', async () => {
    const state = {
      ...defaultRuntimeState(),
      statusText: 'Needs attention',
      attentionItems: [
        {
          severity: DesktopRuntimeSeverity.INFO,
          label: 'Update ready',
          detail: '1.2.3',
        },
        {
          severity: DesktopRuntimeSeverity.CRITICAL,
          label: 'Sign in required',
          detail: 'coolguy@spacewave.app',
        },
      ] satisfies DesktopRuntimeAttentionItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()

    expect(templateLabels(menuTemplates[0])).toEqual([
      'Spacewave: Needs attention',
      'Sign in required',
      'coolguy@spacewave.app',
      '---',
      'Open Spacewave',
      '---',
      'Quit',
    ])
  })

  it('uses native icon state fallbacks for status variants', async () => {
    platformState.value = 'darwin'
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    emitState({
      ...defaultRuntimeState(),
      statusText: 'Syncing',
      health: DesktopRuntimeHealth.ACTIVE,
    })
    await Promise.resolve()
    emitState({
      ...defaultRuntimeState(),
      statusText: 'Needs attention',
      health: DesktopRuntimeHealth.NEEDS_ATTENTION,
    })
    await Promise.resolve()
    emitState({
      ...defaultRuntimeState(),
      statusText: 'Disconnected',
      health: DesktopRuntimeHealth.DISCONNECTED,
    })
    await Promise.resolve()
    emitState({
      ...defaultRuntimeState(),
      statusText: 'Quitting',
      health: DesktopRuntimeHealth.QUITTING,
    })
    await Promise.resolve()

    expect(trayInstances[0]?.setTitle).toHaveBeenNthCalledWith(1, '')
    expect(trayInstances[0]?.setTitle).toHaveBeenNthCalledWith(2, '*')
    expect(trayInstances[0]?.setTitle).toHaveBeenNthCalledWith(3, '!')
    expect(trayInstances[0]?.setTitle).toHaveBeenNthCalledWith(4, 'x')
    expect(trayInstances[0]?.setTitle).toHaveBeenNthCalledWith(5, '...')
    expect(trayInstances[0]?.setToolTip).toHaveBeenLastCalledWith(
      'Spacewave: Quitting',
    )
  })

  it('routes menu and tray commands through the desktop runtime resource', async () => {
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickMenuItem('Open Spacewave')
    await clickMenuItem('Quit')
    trayInstances[0]?.emit('click')
    await flushAsyncEvents()

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(2)
    expect(mockResource.QuitDesktopRuntime).toHaveBeenCalledTimes(1)
  })

  it.each(['darwin', 'win32', 'linux'])(
    'uses the native menu fallback when the dev popover is disabled on %s',
    async (platform) => {
      platformState.value = platform
      const { DesktopTrayController } = await import('./desktop-tray.js')
      const controller = new DesktopTrayController({
        init: { appName: 'Spacewave' },
        resource: mockResource,
      })
      controller.init()

      trayInstances[0]?.emit('click')
      await Promise.resolve()

      expect(browserWindows).toHaveLength(0)
      expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
      expect(templateLabels(menuTemplates[0])).toContain('Open Spacewave')
      expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(1)
    },
  )

  it('dispatches attached-handler tray entries through the tray resource', async () => {
    emitTrayState({
      statusText: 'Running',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'copy-diagnostics',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Copy Diagnostics',
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.ATTACHED_HANDLER,
            value: 'diagnostics',
          },
        },
      ],
    })
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickMenuItem('Copy Diagnostics')

    expect(
      mockResource.desktopTrayResource.InvokeDesktopTrayEntry,
    ).toHaveBeenCalledWith({
      entryId: 'copy-diagnostics',
    })
  })

  it('invokes Electron-scoped attached action handler resources', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const abort = new AbortController()
    const resourceClient = new ResourceClient(
      resource.resourceServer,
      abort.signal,
    )
    const rootRef = await resourceClient.accessRootResource()
    const tray = new DesktopTrayResourceServiceClient(rootRef.client)
    const handler: DesktopTrayActionHandlerService = {
      HandleDesktopTrayAction: vi.fn(async () => ({})),
    }
    const handlerMux = newResourceMux(
      createHandler(DesktopTrayActionHandlerServiceDefinition, handler),
    )
    const attached = await resourceClient.attachRawInvoker(
      'tray-action',
      handlerMux.lookupMethod.bind(handlerMux),
    )

    await tray.RegisterDesktopTrayEntry({
      attachedActionResourceId: attached.resourceId,
      entry: {
        id: 'copy-diagnostics',
        kind: DesktopTrayEntryKind.ACTION,
        label: 'Copy Diagnostics',
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.ATTACHED_HANDLER,
          value: 'diagnostics',
        },
      },
    })
    await tray.InvokeDesktopTrayEntry({ entryId: 'copy-diagnostics' })

    expect(handler.HandleDesktopTrayAction).toHaveBeenCalledWith({
      entryId: 'copy-diagnostics',
      action: {
        kind: DesktopTrayActionKind.ATTACHED_HANDLER,
        value: 'diagnostics',
      },
    })

    attached.cleanup()
    rootRef.release()
    resourceClient.dispose()
    abort.abort()
  })

  it('preserves the entry-backed menu contract for ordering and lifecycle actions', async () => {
    const state = {
      ...defaultRuntimeState(),
      listener: {
        label: 'CLI reachable',
        detail: '1 CLI client connected',
        socketPath: '/tmp/spacewave.sock',
      },
      sessions: [
        {
          label: 'coolguy@spacewave.app',
          detail: 'Cloud',
          statusText: 'Ready',
          route: '/u/1/',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      spaces: [
        {
          label: 'Drive',
          detail: 'Open',
          route: '/u/1/so/drive',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    expect(templateLabels(menuTemplates[0])).toEqual([
      'Spacewave: Running',
      '---',
      'Open Spacewave',
      'New Window',
      '---',
      'Status',
      'CLI reachable - 1 CLI client connected',
      '---',
      'Sessions',
      'coolguy@spacewave.app - Cloud - Ready',
      '---',
      'Spaces',
      'Drive - Open',
      '---',
      'Quick Actions',
      'Copy Socket Path',
      'Copy Diagnostics',
      '---',
      'Settings...',
      'About Spacewave',
      '---',
      'Quit',
    ])

    await clickMenuItem('Open Spacewave')
    await clickMenuItem('coolguy@spacewave.app - Cloud - Ready')
    await clickMenuItem('Drive - Open')
    await clickMenuItem('Quit')

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(1, {})
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(2, {
      route: '/u/1/',
    })
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(3, {
      route: '/u/1/so/drive',
    })
    expect(mockResource.QuitDesktopRuntime).toHaveBeenCalledTimes(1)
  })

  it('renders DesktopTrayEntry paths as native submenus', async () => {
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    const buildMenuTemplate = (
      controller as unknown as {
        buildMenuTemplate: (
          entries: DesktopTrayEntry[],
        ) => Electron.MenuItemConstructorOptions[]
      }
    ).buildMenuTemplate.bind(controller)

    const template = buildMenuTemplate([
      {
        id: 'project-alpha',
        kind: DesktopTrayEntryKind.ACTION,
        path: ['Spaces'],
        label: 'Project Alpha',
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.OPEN_ROUTE,
          route: '/spaces/project-alpha',
        },
      },
      {
        id: 'copy-socket',
        kind: DesktopTrayEntryKind.ACTION,
        path: ['Diagnostics', 'CLI'],
        label: 'Copy Socket',
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.COPY_TEXT,
          value: '/tmp/spacewave.sock',
        },
      },
    ])

    expect(templateLabels(template)).toEqual(['Spaces', 'Diagnostics'])
    expect(
      templateLabels(
        template[0]?.submenu as Electron.MenuItemConstructorOptions[],
      ),
    ).toEqual(['Project Alpha'])
    const diagnostics = template[1]
      ?.submenu as Electron.MenuItemConstructorOptions[]
    expect(templateLabels(diagnostics)).toEqual(['CLI'])
    expect(
      templateLabels(
        diagnostics[0]?.submenu as Electron.MenuItemConstructorOptions[],
      ),
    ).toEqual(['Copy Socket'])
  })

  it('shows the dev popover from DesktopTrayEntry state while keeping native menu fallback', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const state = {
      ...defaultRuntimeState(),
      statusText: 'Syncing',
      health: DesktopRuntimeHealth.ACTIVE,
      listener: {
        label: 'CLI reachable',
        detail: '1 CLI client connected',
        socketPath: '/tmp/spacewave.sock',
      },
      sessions: [
        {
          label: 'coolguy@spacewave.app',
          detail: 'Cloud',
          statusText: 'Ready',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    trayInstances[0]?.emit('click')
    await flushAsyncEvents()
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
    expect(browserWindows).toHaveLength(1)
    expect(browserWindows[0]?.show).toHaveBeenCalledTimes(1)
    expect(mockResource.OpenOrFocusMainWindow).not.toHaveBeenCalled()
    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()
    expect(latestPopoverHtml()).toContain('Syncing')
    expect(latestPopoverHtml()).toContain('CLI reachable')
    expect(latestPopoverHtml()).toContain('coolguy@spacewave.app')

    emitState({
      ...state,
      statusText: 'Needs attention',
      health: DesktopRuntimeHealth.NEEDS_ATTENTION,
      attentionItems: [
        {
          label: 'Sign in required',
          detail: 'coolguy@spacewave.app',
          severity: DesktopRuntimeSeverity.CRITICAL,
        },
      ],
    })
    await Promise.resolve()
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(latestPopoverHtml()).toContain('Needs attention')
    expect(latestPopoverHtml()).toContain('Sign in required')
  })

  it('routes enabled popover actions through the same tray action paths', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const trayState: DesktopTrayState = {
      statusText: 'Running',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'active-route',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Current Space',
          statusText: 'Cmd+1',
          active: true,
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.OPEN_ROUTE,
            route: '/spaces/current',
          },
        },
        {
          id: 'diagnostics',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Copy Diagnostics',
          statusText: 'Cmd+C',
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.COPY_TEXT,
            value: 'diagnostics text',
          },
        },
        {
          id: 'disabled',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Disabled Action',
          statusText: 'Cmd+D',
          enabled: false,
          action: {
            kind: DesktopTrayActionKind.OPEN_ROUTE,
            route: '/disabled',
          },
        },
      ],
    }
    emitTrayState(trayState)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    trayInstances[0]?.emit('click')
    await flushPromises()
    await Promise.resolve()

    const html = latestPopoverHtml()
    expect(html).toContain('spacewave-tray-action:diagnostics')
    expect(html).toContain('aria-current="page"')
    expect(html).toContain('Cmd+1')
    expect(html).toContain('Cmd+C')
    expect(html).toContain(
      'class="row disabled-action" aria-disabled="true"',
    )
    expect(html).toContain('Cmd+D')
    expect(html).toContain('.row.action:hover,')
    expect(html).toContain('.row.action:focus-visible')
    expect(html).toContain('.row.action .status,')
    expect(html).not.toContain('spacewave-tray-action:disabled')

    const event = { preventDefault: vi.fn() }
    browserWindows[0]?.webContents.emit(
      'will-navigate',
      event,
      'spacewave-tray-action:diagnostics',
    )
    await Promise.resolve()

    expect(event.preventDefault).toHaveBeenCalledTimes(1)
    expect(mockClipboard.writeText).toHaveBeenCalledWith('diagnostics text')
  })

  it('exposes opt-in popover screenshot capture for e2e evidence', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    const shown = await controller.showPopoverForE2E()

    const png = await controller.capturePopoverPNGForE2E()

    expect(shown).toBe(true)
    expect(browserWindows).toHaveLength(1)
    expect(browserWindows[0]?.show).toHaveBeenCalledTimes(1)
    expect(browserWindows[0]?.capturePage).toHaveBeenCalledTimes(1)
    expect(png?.toString()).toBe('popover-png')
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
  })

  it('falls back to the singleton window when the dev popover cannot attach', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    browserWindowState.shouldThrow = true
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    trayInstances[0]?.emit('click')
    await flushAsyncEvents()

    expect(browserWindows).toHaveLength(0)
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(1)
  })

  it.each(['darwin', 'win32', 'linux'])(
    'keeps the native menu fallback when the dev popover cannot attach on %s',
    async (platform) => {
      platformState.value = platform
      process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
      browserWindowState.shouldThrow = true
      const { DesktopTrayController } = await import('./desktop-tray.js')
      const controller = new DesktopTrayController({
        init: { appName: 'Spacewave' },
        resource: mockResource,
      })
      controller.init()

      trayInstances[0]?.emit('click')
      await flushPromises()

      expect(browserWindows).toHaveLength(0)
      expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
      expect(templateLabels(menuTemplates[0])).toContain('Open Spacewave')
      expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(1)
    },
  )

  it('keeps the native menu fallback when the dev popover render fails after attach', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    trayInstances[0]?.emit('click')
    await flushPromises()
    expect(browserWindows).toHaveLength(1)

    browserWindows[0]?.loadURL.mockRejectedValueOnce(
      new Error('popover render unavailable'),
    )
    emitTrayState({
      statusText: 'Needs attention',
      iconState: DesktopTrayIconState.ATTENTION,
      entries: [
        {
          id: 'attention',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Spacewave: Needs attention',
        },
      ],
    })
    await flushPromises()

    expect(browserWindows[0]?.close).toHaveBeenCalledTimes(1)
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)

    trayInstances[0]?.emit('click')
    await Promise.resolve()

    expect(browserWindows).toHaveLength(1)
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(1)
  })

  it('keeps the native menu fallback when a dev popover action dispatch fails', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    emitTrayState({
      statusText: 'Running',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'install-update',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Install Update',
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.ATTACHED_HANDLER,
          },
        },
      ],
    })
    mockResource.desktopTrayResource.InvokeDesktopTrayEntry.mockRejectedValueOnce(
      new Error('handler unavailable'),
    )
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    trayInstances[0]?.emit('click')
    await flushPromises()

    const event = { preventDefault: vi.fn() }
    browserWindows[0]?.webContents.emit(
      'will-navigate',
      event,
      'spacewave-tray-action:install-update',
    )
    await flushAsyncEvents()

    expect(event.preventDefault).toHaveBeenCalledTimes(1)
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)

    trayInstances[0]?.emit('click')
    await Promise.resolve()

    expect(browserWindows).toHaveLength(1)
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(1)
  })

  it('renders the rich panel descriptor without subscribing to runtime state', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const state = {
      ...defaultRuntimeState(),
      statusText: 'Syncing',
      health: DesktopRuntimeHealth.ACTIVE,
      sessions: [
        {
          label: 'coolguy@spacewave.app',
          detail: 'Cloud',
          statusText: 'Ready',
          route: '/u/1/',
          active: true,
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      spaces: [
        {
          label: 'Project Alpha With A Very Long Label',
          detail: 'Shared',
          route: '/u/1/so/project-alpha',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      activity: [
        {
          label: 'Uploading changes',
          detail: '2 sync items',
        },
      ] satisfies DesktopRuntimeActivityItem[],
      update: {
        ready: true,
        version: '1.2.3',
        label: 'Ready',
        detail: 'Version 1.2.3',
      },
    }
    setMockRuntimeState(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()
    trayInstances[0]?.emit('click')
    await flushPromises()

    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()
    expect(latestPopoverUrl()).toMatch(/^data:text\/html;charset=utf-8,/)
    const html = latestPopoverHtml()
    expect(html).toContain('data-tab="overview"')
    expect(html).toContain('data-tab="sessions"')
    expect(html).toContain('data-tabs="visible"')
    expect(html).toContain('data-panel="sessions"')
    expect(html).toContain('data-panel="spaces"')
    expect(html).toContain('Project Alpha With A Very Long Label')
    expect(html).toContain('.status {\n  min-width: 0;\n  max-width: 96px;')
    expect(html).toContain('spacewave-tray-action:navigation-')
    expect(html).toContain('spacewave-tray-action:apply-update')
    expect(html).toContain(
      'class="row action severity-info" href="spacewave-tray-action:apply-update"',
    )
    expect(html).toContain('ArrowDown')
    expect(html).toContain('lastFocusByPanel')
    expect(html).toContain('const actions = (scope = activePanel())')
    expect(html).toContain('focusPanelAction(id)')
  })

  it('collapses sparse popover tabs without changing native fallback rows', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()
    trayInstances[0]?.emit('click')
    await flushPromises()

    const html = latestPopoverHtml()
    expect(html).toContain('data-tabs="collapsed"')
    expect(html).not.toContain('class="tabs"')
    expect(html).not.toContain('data-tab="sessions"')
    expect(html).not.toContain('data-tab="spaces"')
    expect(html).not.toContain('data-panel="sessions"')
    expect(html).not.toContain('data-panel="spaces"')
    expect(html).toContain('No active sessions')
    expect(templateLabels(menuTemplates[0])).toContain('Sessions')
    expect(templateLabels(menuTemplates[0])).toContain('No sessions')
    expect(templateLabels(menuTemplates[0])).toContain('Spaces')
    expect(templateLabels(menuTemplates[0])).toContain('No spaces')
    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()
  })

  it('renders bounded session and space cards with route actions and long labels', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const longSpaceLabel =
      'Space One With An Extremely Long Name That Should Truncate In The Card'
    const sessionRows: DesktopTrayEntry[] = Array.from(
      { length: 7 },
      (_, index) => ({
        id: `session-${index + 1}`,
        kind: DesktopTrayEntryKind.ACTION,
        label:
          index === 6 ? 'Session 7 Hidden By Bound' : `Session ${index + 1}`,
        detail: index === 0 ? 'Cloud' : 'Local',
        statusText: index === 0 ? 'Ready' : 'Idle',
        active: index === 0,
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.OPEN_ROUTE,
          route: `/u/${index + 1}/`,
        },
      }),
    )
    const spaceRows: DesktopTrayEntry[] = Array.from(
      { length: 7 },
      (_, index) => ({
        id: `space-${index + 1}`,
        kind: DesktopTrayEntryKind.ACTION,
        label:
          index === 0 ? longSpaceLabel
          : index === 6 ? 'Space 7 Hidden By Bound'
          : `Space ${index + 1}`,
        detail: 'Shared',
        statusText: index === 0 ? 'Active' : 'Ready',
        active: index === 0,
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.OPEN_ROUTE,
          route: `/u/1/so/space-${index + 1}`,
        },
      }),
    )
    mockResource.desktopTrayResource.getState.mockReturnValue({
      statusText: 'Running',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'title',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Spacewave: Running',
        },
        {
          id: 'Sessions-section',
          kind: DesktopTrayEntryKind.SECTION,
          label: 'Sessions',
        },
        ...sessionRows,
        {
          id: 'Spaces-section',
          kind: DesktopTrayEntryKind.SECTION,
          label: 'Spaces',
        },
        ...spaceRows,
      ],
    })
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()
    trayInstances[0]?.emit('click')
    await flushPromises()

    const html = latestPopoverHtml()
    expect(html).toContain('data-card-panel="sessions"')
    expect(html).toContain('data-card-panel="spaces"')
    expect(html).toContain(
      'class="nav-card action active" href="spacewave-tray-action:session-1"',
    )
    expect(html).toContain('data-action-id="space-1"')
    expect(html).toContain(longSpaceLabel)
    expect(html).toContain('Ready')
    expect(html).toContain('+1 more session')
    expect(html).toContain('+1 more space')
    expect(html).not.toContain('Session 7 Hidden By Bound')
    expect(html).not.toContain('Space 7 Hidden By Bound')
    expect(templateLabels(menuTemplates[0])).toContain(longSpaceLabel)
    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()
  })

  it('keeps the panel host independent of renderer window lifetime and runtime polling', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER = '1'
    const initialState: DesktopTrayState = {
      statusText: 'Initial host status',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'title',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Spacewave: Initial host status',
        },
        {
          id: 'host-row',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Initial renderer window row',
        },
      ],
    }
    const updatedState: DesktopTrayState = {
      statusText: 'Recovered host status',
      iconState: DesktopTrayIconState.ACTIVE,
      entries: [
        {
          id: 'title',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Spacewave: Recovered host status',
        },
        {
          id: 'host-row',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Recovered watched tray row',
        },
      ],
    }
    mockResource.desktopTrayResource.getState.mockReturnValue(initialState)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()
    trayInstances[0]?.emit('click')
    await flushPromises()

    expect(browserWindows).toHaveLength(1)
    expect(latestPopoverHtml()).toContain('Initial renderer window row')
    expect(mockResource.getState).toHaveBeenCalledTimes(1)
    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()

    const firstWindow = browserWindows[0]
    firstWindow?.close()
    emitTrayState(updatedState)
    await flushPromises()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(firstWindow?.loadURL).toHaveBeenCalledTimes(1)
    expect(mockResource.getState).toHaveBeenCalledTimes(1)

    trayInstances[0]?.emit('click')
    await flushPromises()

    expect(browserWindows).toHaveLength(2)
    expect(latestPopoverHtml()).toContain('Recovered watched tray row')
    expect(mockResource.getState).toHaveBeenCalledTimes(2)
    expect(
      mockResource.desktopTrayResource.WatchDesktopTray,
    ).toHaveBeenCalledTimes(1)
    expect(mockResource.WatchDesktopState).not.toHaveBeenCalled()
  })

  it('uses opt-in dynamic macOS tray icon variants with title fallback', async () => {
    platformState.value = 'darwin'
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_DYNAMIC_ICON = '1'
    const [electron, { DesktopTrayController }] = await Promise.all([
      import('electron'),
      import('./desktop-tray.js'),
    ])
    const controller = new DesktopTrayController({
      init: {
        appName: 'Spacewave',
        macosTemplateTrayIconPath: '/icons/tray-template.png',
      },
      resource: mockResource,
    })
    controller.init()

    emitState({
      ...defaultRuntimeState(),
      statusText: 'Syncing',
      health: DesktopRuntimeHealth.ACTIVE,
    })
    await flushPromises()
    emitState({
      ...defaultRuntimeState(),
      statusText: 'Needs attention',
      health: DesktopRuntimeHealth.NEEDS_ATTENTION,
    })
    await flushPromises()

    expect(electron.nativeImage.createFromDataURL).toHaveBeenCalled()
    expect(trayInstances[0]?.setImage).toHaveBeenCalledTimes(3)
    expect(trayInstances[0]?.setTitle).toHaveBeenLastCalledWith('!')
    expect(trayInstances[0]?.setToolTip).toHaveBeenLastCalledWith(
      'Spacewave: Needs attention',
    )
  })

  it('keeps shortcut and notification policies opt-in and Electron-owned', async () => {
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_TOGGLE_SHORTCUT =
      'CommandOrControl+Shift+S'
    process.env.BLDR_ELECTRON_DESKTOP_TRAY_NOTIFICATIONS = '1'
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    expect(mockGlobalShortcut.register).toHaveBeenCalledWith(
      'CommandOrControl+Shift+S',
      expect.any(Function),
    )

    emitTrayState({
      statusText: 'Update ready',
      iconState: DesktopTrayIconState.ATTENTION,
      entries: [
        {
          id: 'apply-update',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Install Update',
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.ATTACHED_HANDLER,
            value: '1.2.3',
          },
        },
      ],
    })
    await flushPromises()
    emitTrayState({
      statusText: 'Update ready',
      iconState: DesktopTrayIconState.ATTENTION,
      entries: [
        {
          id: 'apply-update',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Install Update',
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.ATTACHED_HANDLER,
            value: '1.2.3',
          },
        },
      ],
    })
    await flushPromises()

    expect(notifications).toHaveLength(1)
    expect(notifications[0]?.opts).toMatchObject({
      title: 'Spacewave update ready',
      body: 'Version 1.2.3',
      silent: true,
    })

    controller.dispose()

    expect(mockGlobalShortcut.unregister).toHaveBeenCalledWith(
      'CommandOrControl+Shift+S',
    )
    expect(trayInstances[0]?.destroy).toHaveBeenCalledTimes(1)
  })

  it('uses the macOS template icon when configured', async () => {
    platformState.value = 'darwin'
    const [electron, { DesktopTrayController }] = await Promise.all([
      import('electron'),
      import('./desktop-tray.js'),
    ])
    const controller = new DesktopTrayController({
      init: {
        appName: 'Spacewave',
        trayIconPath: '/icons/tray.png',
        macosTemplateTrayIconPath: '/icons/tray-template.png',
      },
      resource: mockResource,
    })

    controller.init()

    expect(electron.nativeImage.createFromPath).toHaveBeenCalledWith(
      '/icons/tray-template.png',
    )
    expect(trayInstances[0]?.image.setTemplateImage).toHaveBeenCalledWith(true)
  })

  it('uses an empty fallback icon when no platform icon is configured', async () => {
    const [electron, { DesktopTrayController }] = await Promise.all([
      import('electron'),
      import('./desktop-tray.js'),
    ])
    const controller = new DesktopTrayController({
      init: {},
      resource: mockResource,
    })

    controller.init()

    expect(electron.nativeImage.createEmpty).toHaveBeenCalledTimes(1)
    expect(trayInstances).toHaveLength(1)
  })
})

async function clickMenuItem(label: string): Promise<void> {
  const item = menuTemplates[0]?.find((entry) => entry.label === label)
  if (!item?.click) {
    throw new Error(`${label} menu item not found`)
  }
  Reflect.apply(item.click, undefined, [])
  await Promise.resolve()
}

async function flushPromises(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

async function flushAsyncEvents(): Promise<void> {
  await flushPromises()
  await flushPromises()
}

function templateLabels(
  template: Electron.MenuItemConstructorOptions[] | undefined,
): string[] {
  return (template ?? []).map((item) => {
    if (item.type === 'separator') {
      return '---'
    }
    return String(item.label)
  })
}

function latestPopoverUrl(): string {
  return String(browserWindows.at(-1)?.loadURL.mock.calls.at(-1)?.[0] ?? '')
}

function latestPopoverHtml(): string {
  const url = latestPopoverUrl()
  if (!url) {
    return ''
  }
  return decodeURIComponent(
    String(url).replace('data:text/html;charset=utf-8,', ''),
  )
}

function defaultRuntimeState(): DesktopRuntimeState {
  return {
    mainWindowOpen: false,
    quitting: false,
    statusText: 'Running',
  }
}

function defaultTrayState(): DesktopTrayState {
  return trayStateFromRuntimeState(defaultRuntimeState())
}

function setMockRuntimeState(state: DesktopRuntimeState): void {
  mockResource.getState.mockReturnValue(state)
  mockResource.desktopTrayResource.getState.mockReturnValue(
    trayStateFromRuntimeState(state),
  )
}

function trayStateFromRuntimeState(
  state: DesktopRuntimeState,
): DesktopTrayState {
  return {
    entries: buildDesktopTrayEntriesFromRuntimeState(state),
    statusText: state.statusText || 'Running',
    iconState: iconStateForRuntimeHealth(state.health),
  }
}

class TestStateStream implements AsyncIterable<WatchDesktopStateResponse> {
  private queue: WatchDesktopStateResponse[] = []
  private resolveNext?: (
    value: IteratorResult<WatchDesktopStateResponse>,
  ) => void

  public emit(response: WatchDesktopStateResponse): void {
    if (this.resolveNext) {
      const resolve = this.resolveNext
      this.resolveNext = undefined
      resolve({ value: response, done: false })
      return
    }
    this.queue.push(response)
  }

  public [Symbol.asyncIterator](): AsyncIterator<WatchDesktopStateResponse> {
    return {
      next: () => this.next(),
      return: () => Promise.resolve({ value: undefined, done: true }),
    }
  }

  private next(): Promise<IteratorResult<WatchDesktopStateResponse>> {
    const response = this.queue.shift()
    if (response) {
      return Promise.resolve({ value: response, done: false })
    }
    return new Promise((resolve) => {
      this.resolveNext = resolve
    })
  }
}

class TestTrayStateStream implements AsyncIterable<WatchDesktopTrayResponse> {
  private queue: WatchDesktopTrayResponse[] = []
  private resolveNext?: (
    value: IteratorResult<WatchDesktopTrayResponse>,
  ) => void

  public emit(response: WatchDesktopTrayResponse): void {
    if (this.resolveNext) {
      const resolve = this.resolveNext
      this.resolveNext = undefined
      resolve({ value: response, done: false })
      return
    }
    this.queue.push(response)
  }

  public [Symbol.asyncIterator](): AsyncIterator<WatchDesktopTrayResponse> {
    return {
      next: () => this.next(),
      return: () => Promise.resolve({ value: undefined, done: true }),
    }
  }

  private next(): Promise<IteratorResult<WatchDesktopTrayResponse>> {
    const response = this.queue.shift()
    if (response) {
      return Promise.resolve({ value: response, done: false })
    }
    return new Promise((resolve) => {
      this.resolveNext = resolve
    })
  }
}
