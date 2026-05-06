import os from 'os'
import electron from 'electron'

import type { ElectronInit } from '../../plugin/electron/electron.pb.js'
import type { DesktopRuntimeResource } from './desktop-runtime.js'

interface DesktopTrayControllerOpts {
  init: ElectronInit
  resource: DesktopTrayResource
}

interface DesktopTrayResource {
  OpenOrFocusMainWindow: DesktopRuntimeResource['OpenOrFocusMainWindow']
  QuitDesktopRuntime: DesktopRuntimeResource['QuitDesktopRuntime']
  getState: DesktopRuntimeResource['getState']
}

// DesktopTrayController owns the native desktop status icon for the app lifetime.
export class DesktopTrayController {
  private tray?: Electron.Tray

  constructor(private readonly opts: DesktopTrayControllerOpts) {}

  public init(): void {
    if (this.tray) {
      return
    }
    this.tray = new electron.Tray(this.buildIcon())
    this.tray.setToolTip(this.opts.init.appName || 'Spacewave')
    this.tray.setContextMenu(this.buildMenu())
    this.tray.on('click', () => {
      void this.openOrFocusMainWindow()
    })
  }

  private buildIcon(): Electron.NativeImage {
    const iconPath = this.getIconPath()
    const image =
      iconPath ?
        electron.nativeImage.createFromPath(iconPath)
      : electron.nativeImage.createEmpty()
    if (
      os.platform() === 'darwin' &&
      iconPath === this.opts.init.macosTemplateTrayIconPath
    ) {
      image.setTemplateImage(true)
    }
    return image
  }

  private getIconPath(): string {
    if (
      os.platform() === 'darwin' &&
      this.opts.init.macosTemplateTrayIconPath
    ) {
      return this.opts.init.macosTemplateTrayIconPath
    }
    return this.opts.init.trayIconPath || ''
  }

  private buildMenu(): Electron.Menu {
    return electron.Menu.buildFromTemplate([
      {
        label: 'Open Spacewave',
        click: () => {
          void this.openOrFocusMainWindow()
        },
      },
      { type: 'separator' },
      {
        label: `Status: ${this.opts.resource.getState().statusText || 'Running'}`,
        enabled: false,
      },
      { type: 'separator' },
      {
        label: 'Quit',
        click: () => {
          void this.quitDesktopRuntime()
        },
      },
    ])
  }

  private async openOrFocusMainWindow(): Promise<void> {
    await this.opts.resource.OpenOrFocusMainWindow({})
  }

  private async quitDesktopRuntime(): Promise<void> {
    await this.opts.resource.QuitDesktopRuntime({})
  }
}

export type { DesktopTrayControllerOpts, DesktopTrayResource }
