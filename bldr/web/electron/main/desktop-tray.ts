import os from 'os'
import electron from 'electron'

import type { ElectronInit } from '../../plugin/electron/electron.pb.js'
import {
  DesktopRuntimeState,
  type DesktopRuntimeState as DesktopRuntimeStateMessage,
} from '../desktop-runtime/desktop-runtime.pb.js'
import type { DesktopRuntimeResource } from './desktop-runtime.js'

interface DesktopTrayControllerOpts {
  init: ElectronInit
  resource: DesktopTrayResource
}

interface DesktopTrayResource {
  WatchDesktopState: DesktopRuntimeResource['WatchDesktopState']
  OpenOrFocusMainWindow: DesktopRuntimeResource['OpenOrFocusMainWindow']
  QuitDesktopRuntime: DesktopRuntimeResource['QuitDesktopRuntime']
  getState: DesktopRuntimeResource['getState']
}

// DesktopTrayController owns the native desktop status icon for the app lifetime.
export class DesktopTrayController {
  private tray?: Electron.Tray
  private currentState?: DesktopRuntimeStateMessage
  private stateWatchStarted = false

  constructor(private readonly opts: DesktopTrayControllerOpts) {}

  public init(): void {
    if (this.tray) {
      return
    }
    this.tray = new electron.Tray(this.buildIcon())
    this.tray.setToolTip(this.opts.init.appName || 'Spacewave')
    this.rebuildMenu(this.opts.resource.getState())
    this.startStateWatch()
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

  private startStateWatch(): void {
    if (this.stateWatchStarted) {
      return
    }
    this.stateWatchStarted = true
    void this.watchDesktopState()
  }

  private async watchDesktopState(): Promise<void> {
    try {
      for await (const resp of this.opts.resource.WatchDesktopState({})) {
        this.rebuildMenu(resp.state ?? this.opts.resource.getState())
      }
    } catch (err) {
      console.error('desktop tray state stream ended', err)
    }
  }

  private rebuildMenu(state: DesktopRuntimeStateMessage): void {
    if (this.currentState && DesktopRuntimeState.equals(this.currentState, state)) {
      return
    }
    this.currentState = cloneDesktopRuntimeState(state)
    this.tray?.setContextMenu(this.buildMenu(state))
  }

  private buildMenu(state: DesktopRuntimeStateMessage): Electron.Menu {
    return electron.Menu.buildFromTemplate([
      {
        label: 'Open Spacewave',
        click: () => {
          void this.openOrFocusMainWindow()
        },
      },
      { type: 'separator' },
      {
        label: `Status: ${state.statusText || 'Running'}`,
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

function cloneDesktopRuntimeState(
  state: DesktopRuntimeStateMessage,
): DesktopRuntimeStateMessage {
  return {
    ...state,
    listener: state.listener ? { ...state.listener } : undefined,
    sessions: state.sessions?.map((item) => ({ ...item })),
    spaces: state.spaces?.map((item) => ({ ...item })),
    activity: state.activity?.map((item) => ({ ...item })),
    update: state.update ? { ...state.update } : undefined,
    attentionItems: state.attentionItems?.map((item) => ({ ...item })),
    actions: state.actions?.map((item) => ({ ...item })),
  }
}

export type { DesktopTrayControllerOpts, DesktopTrayResource }
