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
  DesktopCLIInstallStatus,
  DesktopRuntimeActionKind,
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
const mockClipboard = {
  writeText: vi.fn(),
}
const mockShell = {
  showItemInFolder: vi.fn(),
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
let emitState: (state: DesktopRuntimeState) => void = () => {}
let emitTrayState: (state: DesktopTrayState) => void = () => {}
let emitTrayWatchResponse: (state: DesktopTrayState) => void = () => {}

class MockNativeImage {
  public readonly setTemplateImage = vi.fn()
}

class MockTray extends EventEmitter {
  public readonly setToolTip = vi.fn()
  public readonly setContextMenu = vi.fn()
  public readonly destroy = vi.fn()

  constructor(public readonly image: MockNativeImage) {
    super()
    trayInstances.push(this)
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
      Menu,
      clipboard: mockClipboard,
      nativeImage,
      shell: mockShell,
    },
    Tray: MockTray,
    Menu,
    clipboard: mockClipboard,
    nativeImage,
    shell: mockShell,
  }
})

describe('DesktopTrayController', () => {
  beforeEach(() => {
    platformState.value = 'linux'
    menuTemplates.length = 0
    trayInstances.length = 0
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
    emitTrayWatchResponse = (state: DesktopTrayState) => {
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
    expect(trayInstances[0]?.setToolTip).toHaveBeenCalledWith('Spacewave')
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
    expect(trayInstances[0]?.setToolTip).toHaveBeenLastCalledWith('Spacewave')
  })

  it('renders watched tray payloads even when the resource snapshot advances', async () => {
    const initial: DesktopTrayState = {
      statusText: 'Initial',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'initial',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Initial tray row',
        },
      ],
    }
    const watched: DesktopTrayState = {
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
    }
    const advanced: DesktopTrayState = {
      statusText: 'Advanced',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'advanced',
          kind: DesktopTrayEntryKind.STATUS,
          label: 'Advanced resource row',
        },
      ],
    }
    mockResource.desktopTrayResource.getState.mockReturnValue(initial)
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    mockResource.desktopTrayResource.getState.mockReturnValue(advanced)
    emitTrayWatchResponse(watched)
    await flushPromises()

    expect(trayInstances[0]?.setContextMenu).toHaveBeenCalledTimes(2)
    expect(templateLabels(menuTemplates[1])).toContain('Install Update')
    expect(templateLabels(menuTemplates[1])).not.toContain(
      'Advanced resource row',
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

  it('projects compact CLI install status and settings action without install actions', async () => {
    const state = {
      ...defaultRuntimeState(),
      cliInstall: {
        status:
          DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE,
        label: 'CLI update available',
        detail: 'spacewave-cli rev 9',
        route: '/u/2/settings/cli',
      },
      sessions: [
        {
          label: 'coolguy@spacewave.app',
          route: '/u/2/',
          active: true,
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

    expect(templateLabels(menuTemplates[0])).toContain(
      'Command line - CLI update available - spacewave-cli rev 9',
    )
    expect(templateLabels(menuTemplates[0])).toContain(
      'Command Line Settings - CLI update available',
    )
    expect(templateLabels(menuTemplates[0])).not.toContain('Install')
    expect(templateLabels(menuTemplates[0])).not.toContain('Update')

    await clickMenuItem('Command Line Settings - CLI update available')

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledWith({
      route: '/u/2/settings/cli',
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

  it('disables copy and reveal actions without values', async () => {
    emitTrayState({
      statusText: 'Running',
      iconState: DesktopTrayIconState.NORMAL,
      entries: [
        {
          id: 'copy-missing',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Copy Missing',
          enabled: true,
          action: { kind: DesktopTrayActionKind.COPY_TEXT },
        },
        {
          id: 'reveal-missing',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Reveal Missing',
          enabled: true,
          action: { kind: DesktopTrayActionKind.REVEAL_PATH },
        },
        {
          id: 'route',
          kind: DesktopTrayEntryKind.ACTION,
          label: 'Open Route',
          enabled: true,
          action: { kind: DesktopTrayActionKind.OPEN_ROUTE },
        },
      ],
    })
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    const copy = menuTemplates[0]?.find(
      (entry) => entry.label === 'Copy Missing',
    )
    const reveal = menuTemplates[0]?.find(
      (entry) => entry.label === 'Reveal Missing',
    )
    const route = menuTemplates[0]?.find(
      (entry) => entry.label === 'Open Route',
    )

    expect(copy).toMatchObject({ enabled: false })
    expect(copy?.click).toBeUndefined()
    expect(reveal).toMatchObject({ enabled: false })
    expect(reveal?.click).toBeUndefined()
    expect(route?.click).toBeDefined()
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
    await flushPromises()

    expect(mockResource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(2)
    expect(mockResource.QuitDesktopRuntime).toHaveBeenCalledTimes(1)
  })

  it.each(['darwin', 'win32', 'linux'])(
    'opens the main window from the tray click on %s',
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

  it('reuses explicit submenu entries when children target the same path', async () => {
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
        id: 'tools',
        kind: DesktopTrayEntryKind.SUBMENU,
        path: ['Diagnostics'],
        label: 'Tools',
      },
      {
        id: 'copy-socket',
        kind: DesktopTrayEntryKind.ACTION,
        path: ['Diagnostics', 'Tools'],
        label: 'Copy Socket',
        enabled: true,
        action: {
          kind: DesktopTrayActionKind.COPY_TEXT,
          value: '/tmp/spacewave.sock',
        },
      },
    ])

    expect(templateLabels(template)).toEqual(['Diagnostics'])
    const diagnostics = template[0]
      ?.submenu as Electron.MenuItemConstructorOptions[]
    expect(templateLabels(diagnostics)).toEqual(['Tools'])
    expect(
      templateLabels(
        diagnostics[0]?.submenu as Electron.MenuItemConstructorOptions[],
      ),
    ).toEqual(['Copy Socket'])
  })

  it('dispatches every user-critical action through the native menu', async () => {
    emitTrayState({
      statusText: 'Running',
      iconState: DesktopTrayIconState.NORMAL,
      entries: userCriticalActionEntries(),
    })
    const { DesktopTrayController } = await import('./desktop-tray.js')
    const controller = new DesktopTrayController({
      init: { appName: 'Spacewave' },
      resource: mockResource,
    })
    controller.init()

    await clickLatestMenuItem('Open Current Space')
    await clickLatestMenuItem('Open New Space Window')
    await clickLatestMenuItem('Copy Diagnostics')
    await clickLatestMenuItem('Reveal Logs')
    await clickLatestMenuItem('Restart Listener')
    await clickLatestMenuItem('Quit')
    await flushPromises()

    expect(mockResource.OpenOrFocusMainWindow.mock.calls).toEqual([
      [{ route: '/spaces/current' }],
      [{ route: '/spaces/new' }],
    ])
    expect(mockClipboard.writeText).toHaveBeenCalledWith('diagnostics text')
    expect(mockShell.showItemInFolder).toHaveBeenCalledWith(
      '/tmp/spacewave.log',
    )
    expect(
      mockResource.desktopTrayResource.InvokeDesktopTrayEntry,
    ).toHaveBeenCalledWith({ entryId: 'restart-listener' })
    expect(mockResource.QuitDesktopRuntime).toHaveBeenCalledTimes(1)
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

async function clickLatestMenuItem(label: string): Promise<void> {
  const item = menuTemplates.at(-1)?.find((entry) => entry.label === label)
  if (!item?.click) {
    throw new Error(`${label} menu item not found`)
  }
  Reflect.apply(item.click, undefined, [])
  await flushPromises()
}

async function flushPromises(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
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

function userCriticalActionEntries(): DesktopTrayEntry[] {
  return [
    {
      id: 'open-current-space',
      kind: DesktopTrayEntryKind.ACTION,
      label: 'Open Current Space',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.OPEN_ROUTE,
        route: '/spaces/current',
      },
    },
    {
      id: 'open-new-space-window',
      kind: DesktopTrayEntryKind.ACTION,
      label: 'Open New Space Window',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.NEW_WINDOW,
        route: '/spaces/new',
      },
    },
    {
      id: 'copy-diagnostics',
      kind: DesktopTrayEntryKind.ACTION,
      label: 'Copy Diagnostics',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.COPY_TEXT,
        value: 'diagnostics text',
      },
    },
    {
      id: 'reveal-logs',
      kind: DesktopTrayEntryKind.ACTION,
      label: 'Reveal Logs',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.REVEAL_PATH,
        value: '/tmp/spacewave.log',
      },
    },
    {
      id: 'restart-listener',
      kind: DesktopTrayEntryKind.ACTION,
      label: 'Restart Listener',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.ATTACHED_HANDLER,
      },
    },
    {
      id: 'quit',
      kind: DesktopTrayEntryKind.ACTION,
      label: 'Quit',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.QUIT,
      },
    },
  ]
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
