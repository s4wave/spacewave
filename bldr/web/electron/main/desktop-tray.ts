import os from 'os'
import electron from 'electron'

import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTrayState,
  type DesktopTrayEntry,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import type { ElectronInit } from '../../plugin/electron/electron.pb.js'
import type { DesktopRuntimeState as DesktopRuntimeStateMessage } from '../desktop-runtime/desktop-runtime.pb.js'
import type { DesktopRuntimeResource } from './desktop-runtime.js'
import { DesktopTrayPopoverController } from './desktop-tray-popover.js'
import { buildDesktopTrayEntriesFromRuntimeState } from './desktop-tray-runtime-projection.js'

interface DesktopTrayControllerOpts {
  init: ElectronInit
  resource: DesktopTrayResource
}

interface DesktopTrayResource {
  WatchDesktopState: DesktopRuntimeResource['WatchDesktopState']
  OpenOrFocusMainWindow: DesktopRuntimeResource['OpenOrFocusMainWindow']
  QuitDesktopRuntime: DesktopRuntimeResource['QuitDesktopRuntime']
  getState: DesktopRuntimeResource['getState']
  desktopTrayResource: Pick<
    DesktopRuntimeResource['desktopTrayResource'],
    'WatchDesktopTray' | 'getState'
  >
}

// DesktopTrayController owns the native desktop status icon for the app lifetime.
export class DesktopTrayController {
  private tray?: Electron.Tray
  private popover?: DesktopTrayPopoverController
  private currentState?: DesktopRuntimeStateMessage
  private currentTrayState?: DesktopTrayState
  private watchStarted = false

  constructor(private readonly opts: DesktopTrayControllerOpts) {}

  public init(): void {
    if (this.tray) {
      return
    }
    this.tray = new electron.Tray(this.buildIcon())
    if (isDesktopTrayPopoverEnabled()) {
      this.popover = new DesktopTrayPopoverController({
        appName: this.opts.init.appName,
      })
    }
    this.rebuildMenu(this.opts.resource.desktopTrayResource.getState())
    this.startWatch()
    this.tray.on('click', () => {
      void this.handleTrayClick()
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

  private startWatch(): void {
    if (this.watchStarted) {
      return
    }
    this.watchStarted = true
    void this.watchDesktopTray()
    if (this.popover) {
      this.updatePopover(this.opts.resource.getState())
      void this.watchDesktopStateForPopover()
    }
  }

  private async watchDesktopTray(): Promise<void> {
    try {
      for await (const resp of this.opts.resource.desktopTrayResource.WatchDesktopTray(
        {},
      )) {
        this.rebuildMenu(
          resp.state ?? this.opts.resource.desktopTrayResource.getState(),
        )
      }
    } catch (err) {
      console.error('desktop tray state stream ended', err)
    }
  }

  private async watchDesktopStateForPopover(): Promise<void> {
    try {
      for await (const resp of this.opts.resource.WatchDesktopState({})) {
        this.updatePopover(resp.state ?? this.opts.resource.getState())
      }
    } catch (err) {
      console.error('desktop tray state stream ended', err)
    }
  }

  private rebuildMenu(state: DesktopTrayState): void {
    if (
      this.currentTrayState &&
      DesktopTrayState.equals(this.currentTrayState, state)
    ) {
      return
    }
    this.currentTrayState = cloneDesktopTrayState(state)
    this.updateIconState(state)
    this.tray?.setContextMenu(this.buildMenu(state))
  }

  private updatePopover(state: DesktopRuntimeStateMessage): void {
    this.currentState = cloneDesktopRuntimeState(state)
    this.popover?.update(state)
  }

  private async handleTrayClick(): Promise<void> {
    if (this.popover) {
      const handled = await this.popover.toggle(
        this.tray,
        this.currentState ?? this.opts.resource.getState(),
      )
      if (handled) {
        return
      }
    }
    await this.openOrFocusMainWindow()
  }

  private buildMenu(state: DesktopTrayState): Electron.Menu {
    return electron.Menu.buildFromTemplate(
      this.buildMenuTemplate(state.entries ?? []),
    )
  }

  private buildTrayEntries(
    state: DesktopRuntimeStateMessage,
  ): DesktopTrayEntry[] {
    return buildDesktopTrayEntriesFromRuntimeState(state)
  }
  private buildMenuTemplate(
    entries: DesktopTrayEntry[],
  ): Electron.MenuItemConstructorOptions[] {
    const root: Electron.MenuItemConstructorOptions[] = []
    const submenus = new Map<string, Electron.MenuItemConstructorOptions[]>()
    for (const entry of entries) {
      const level = this.getMenuLevel(root, submenus, entry.path ?? [])
      if (entry.kind === DesktopTrayEntryKind.SUBMENU) {
        this.getMenuLevel(level, submenus, [entry.label || ''])
        continue
      }
      level.push(this.buildMenuItem(entry))
    }
    return root
  }

  private getMenuLevel(
    root: Electron.MenuItemConstructorOptions[],
    submenus: Map<string, Electron.MenuItemConstructorOptions[]>,
    path: string[],
  ): Electron.MenuItemConstructorOptions[] {
    let level = root
    let key = ''
    for (const segment of path.filter((part) => part !== '')) {
      key = `${key}\0${segment}`
      let submenu = submenus.get(key)
      if (!submenu) {
        submenu = []
        submenus.set(key, submenu)
        level.push({ label: segment, submenu })
      }
      level = submenu
    }
    return level
  }

  private buildMenuItem(
    entry: DesktopTrayEntry,
  ): Electron.MenuItemConstructorOptions {
    switch (entry.kind) {
      case DesktopTrayEntryKind.SEPARATOR:
        return { type: 'separator' }
      case DesktopTrayEntryKind.ACTION:
        return this.buildActionMenuItem(entry)
      default:
        return disabledItem(entry.label || '')
    }
  }

  private buildActionMenuItem(
    entry: DesktopTrayEntry,
  ): Electron.MenuItemConstructorOptions {
    const action = entry.action
    if (!entry.enabled || !action) {
      return disabledItem(entry.label || '')
    }
    switch (action.kind) {
      case DesktopTrayActionKind.OPEN_ROUTE:
      case DesktopTrayActionKind.NEW_WINDOW:
        return {
          label: entry.label,
          click: () => {
            void this.openRouteOrFocus(action.route)
          },
        }
      case DesktopTrayActionKind.COPY_TEXT:
        if (!action.value) {
          return disabledItem(entry.label || '')
        }
        return {
          label: entry.label,
          click: () => {
            this.copyText(action.value || '')
          },
        }
      case DesktopTrayActionKind.REVEAL_PATH:
        if (!action.value) {
          return disabledItem(entry.label || '')
        }
        return {
          label: entry.label,
          click: () => {
            this.revealPath(action.value || '')
          },
        }
      case DesktopTrayActionKind.QUIT:
        return {
          label: entry.label,
          click: () => {
            void this.quitDesktopRuntime()
          },
        }
      default:
        return disabledItem(entry.label || '')
    }
  }

  private updateIconState(state: DesktopTrayState): void {
    this.tray?.setToolTip(
      `${this.opts.init.appName || 'Spacewave'}: ${state.statusText || 'Running'}`,
    )
    if (os.platform() === 'darwin') {
      this.tray?.setTitle(trayTitleForState(state))
    }
  }

  private async openOrFocusMainWindow(): Promise<void> {
    await this.opts.resource.OpenOrFocusMainWindow({})
  }

  private async openRoute(route?: string): Promise<void> {
    await this.opts.resource.OpenOrFocusMainWindow({ route })
  }

  private async openRouteOrFocus(route?: string): Promise<void> {
    if (route) {
      await this.openRoute(route)
      return
    }
    await this.openOrFocusMainWindow()
  }

  private copyText(text: string): void {
    electron.clipboard.writeText(text)
  }

  private revealPath(path: string): void {
    electron.shell.showItemInFolder(path)
  }

  private async quitDesktopRuntime(): Promise<void> {
    await this.opts.resource.QuitDesktopRuntime({})
  }
}

function disabledItem(label: string): Electron.MenuItemConstructorOptions {
  return { label, enabled: false }
}

function isDesktopTrayPopoverEnabled(): boolean {
  return process.env.BLDR_ELECTRON_DESKTOP_TRAY_POPOVER === '1'
}

function trayTitleForState(state: DesktopTrayState): string {
  switch (state.iconState) {
    case DesktopTrayIconState.ACTIVE:
      return '*'
    case DesktopTrayIconState.ATTENTION:
      return '!'
    case DesktopTrayIconState.DISCONNECTED:
      return 'x'
    case DesktopTrayIconState.QUITTING:
      return '...'
    default:
      return ''
  }
}

function cloneDesktopTrayState(state: DesktopTrayState): DesktopTrayState {
  return {
    ...state,
    entries: state.entries?.map((entry) => ({
      ...entry,
      path: [...(entry.path ?? [])],
      action: entry.action ? { ...entry.action } : undefined,
    })),
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
