import { EventEmitter } from 'events'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const platformState = { value: 'linux' }
const menuTemplates: Electron.MenuItemConstructorOptions[][] = []
const trayInstances: MockTray[] = []
const mockResource = {
  getState: vi.fn(() => ({
    mainWindowOpen: false,
    quitting: false,
    statusText: 'Running',
  })),
  OpenOrFocusMainWindow: vi.fn(async () => ({})),
  QuitDesktopRuntime: vi.fn(async () => ({})),
}

class MockNativeImage {
  public readonly setTemplateImage = vi.fn()
}

class MockTray extends EventEmitter {
  public readonly setToolTip = vi.fn()
  public readonly setContextMenu = vi.fn()

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
      nativeImage,
    },
    Tray: MockTray,
    Menu,
    nativeImage,
  }
})

describe('DesktopTrayController', () => {
  beforeEach(() => {
    platformState.value = 'linux'
    menuTemplates.length = 0
    trayInstances.length = 0
    vi.clearAllMocks()
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
      label: 'Status: Running',
      enabled: false,
    })
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
