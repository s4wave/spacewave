import { EventEmitter } from 'events'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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

const platformState = { value: 'linux' }
const menuTemplates: Electron.MenuItemConstructorOptions[][] = []
const trayInstances: MockTray[] = []
const browserWindows: MockBrowserWindow[] = []
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
}
const browserWindowState = {
  shouldThrow: false,
}
let emitState: (state: DesktopRuntimeState) => void = () => {}

class MockNativeImage {
  public readonly setTemplateImage = vi.fn()
}

class MockTray extends EventEmitter {
  public readonly setToolTip = vi.fn()
  public readonly setContextMenu = vi.fn()
  public readonly setTitle = vi.fn()
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

class MockBrowserWindow extends EventEmitter {
  public readonly loadURL = vi.fn((_url: string) => Promise.resolve())
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
      clipboard: mockClipboard,
      nativeImage,
      screen: mockScreen,
      shell: mockShell,
    },
    Tray: MockTray,
    BrowserWindow: MockBrowserWindow,
    Menu,
    clipboard: mockClipboard,
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
    browserWindowState.shouldThrow = false
    delete process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER
    vi.clearAllMocks()
    mockResource.getState.mockReturnValue(defaultRuntimeState())
    const stream = new TestStateStream()
    emitState = (state: DesktopRuntimeState) => stream.emit({ state })
    mockResource.WatchDesktopState.mockReturnValue(stream)
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
    expect(mockResource.WatchDesktopState).toHaveBeenCalledTimes(1)
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
    mockResource.getState.mockReturnValue(disconnected)
    emitState(disconnected)
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(menuTemplates[1]).toContainEqual({
      label: 'Spacewave: Disconnected',
      enabled: false,
    })
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
          label: 'christian@aperture.us',
          detail: 'Cloud',
          statusText: 'Ready',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
      spaces: [
        {
          label: 'Project Alpha',
          detail: 'christian@aperture.us',
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
    mockResource.getState.mockReturnValue(state)
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
      'christian@aperture.us - Cloud - Ready',
      '---',
      'Spaces',
      'Project Alpha - christian@aperture.us - Shared',
      '---',
      'Activity',
      'Uploading changes - 2 sync items',
      '---',
      'Quick Actions',
      'Copy CLI Socket',
      'Copy Diagnostics',
      'Copy diagnostics - CLI socket',
      '---',
      'Settings...',
      'About Spacewave',
      '---',
      'Quit',
    ])
  })

  it('routes menu rows through the singleton app window', async () => {
    const state = {
      ...defaultRuntimeState(),
      sessions: [
        {
          label: 'christian@aperture.us',
          route: '/u/2/',
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
    mockResource.getState.mockReturnValue(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickMenuItem('Open Spacewave')
    await clickMenuItem('New Window')
    await clickMenuItem('christian@aperture.us')
    await clickMenuItem('Project Alpha')
    await clickMenuItem('Open dashboard')
    await clickMenuItem('Settings...')
    await clickMenuItem('About Spacewave')

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(1, {})
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenNthCalledWith(2, {})
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
      route: '/settings',
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
    mockResource.getState.mockReturnValue(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickMenuItem('Copy CLI Socket')
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
    mockResource.getState.mockReturnValue(state)
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
          detail: 'christian@aperture.us',
        },
      ] satisfies DesktopRuntimeAttentionItem[],
    }
    mockResource.getState.mockReturnValue(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })

    controller.init()

    expect(templateLabels(menuTemplates[0])).toEqual([
      'Spacewave: Needs attention',
      'Sign in required',
      'christian@aperture.us',
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
    await Promise.resolve()

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(2)
    expect(mockResource.QuitDesktopRuntime).toHaveBeenCalledTimes(1)
  })

  it('shows the dev popover from desktop runtime state while keeping native menu fallback', async () => {
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
          label: 'christian@aperture.us',
          detail: 'Cloud',
          statusText: 'Ready',
        },
      ] satisfies DesktopRuntimeNavigationItem[],
    }
    mockResource.getState.mockReturnValue(state)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    trayInstances[0]?.emit('click')
    await Promise.resolve()
    await Promise.resolve()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
    expect(browserWindows).toHaveLength(1)
    expect(browserWindows[0]?.show).toHaveBeenCalledTimes(1)
    expect(mockResource.OpenOrFocusMainWindow).not.toHaveBeenCalled()
    expect(latestPopoverHtml()).toContain('Syncing')
    expect(latestPopoverHtml()).toContain('CLI reachable')
    expect(latestPopoverHtml()).toContain('christian@aperture.us')

    emitState({
      ...state,
      statusText: 'Needs attention',
      health: DesktopRuntimeHealth.NEEDS_ATTENTION,
      attentionItems: [
        {
          label: 'Sign in required',
          detail: 'christian@aperture.us',
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
    await Promise.resolve()

    expect(browserWindows).toHaveLength(0)
    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(1)
    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(1)
  })

  it('uses the macOS template icon when configured', async () => {
    platformState.value = 'darwin'
    const electron = await import('electron')
    const { DesktopTrayController } = await import('./desktop-tray.js')
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
    const electron = await import('electron')
    const { DesktopTrayController } = await import('./desktop-tray.js')
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

function latestPopoverHtml(): string {
  const url = browserWindows.at(-1)?.loadURL.mock.calls.at(-1)?.[0]
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
