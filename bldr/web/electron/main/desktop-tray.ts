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
import type { DesktopRuntimeResource } from './desktop-runtime.js'
import { DesktopTrayPopoverController } from './desktop-tray-popover.js'

interface DesktopTrayControllerOpts {
  init: ElectronInit
  resource: DesktopTrayResource
}

interface DesktopTrayResource {
  OpenOrFocusMainWindow: DesktopRuntimeResource['OpenOrFocusMainWindow']
  QuitDesktopRuntime: DesktopRuntimeResource['QuitDesktopRuntime']
  desktopTrayResource: Pick<
    DesktopRuntimeResource['desktopTrayResource'],
    'WatchDesktopTray' | 'InvokeDesktopTrayEntry' | 'getState'
  >
}

// DesktopTrayController owns the native desktop status icon for the app lifetime.
export class DesktopTrayController {
  private tray?: Electron.Tray
  private popover?: DesktopTrayPopoverController
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
        actionHandler: (entryId) => this.handlePopoverAction(entryId),
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
    this.popover?.update(state)
  }

  private async handleTrayClick(): Promise<void> {
    if (this.popover) {
      const handled = await this.popover.toggle(
        this.tray,
        this.currentTrayState ??
          this.opts.resource.desktopTrayResource.getState(),
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
        return {
          label: entry.label,
          click: () => {
            void this.invokeTrayEntry(entry)
          },
        }
      case DesktopTrayActionKind.NEW_WINDOW:
        return {
          label: entry.label,
          click: () => {
            void this.invokeTrayEntry(entry)
          },
        }
      case DesktopTrayActionKind.COPY_TEXT:
        if (!action.value) {
          return disabledItem(entry.label || '')
        }
        return {
          label: entry.label,
          click: () => {
            void this.invokeTrayEntry(entry)
          },
        }
      case DesktopTrayActionKind.REVEAL_PATH:
        if (!action.value) {
          return disabledItem(entry.label || '')
        }
        return {
          label: entry.label,
          click: () => {
            void this.invokeTrayEntry(entry)
          },
        }
      case DesktopTrayActionKind.QUIT:
        return {
          label: entry.label,
          click: () => {
            void this.invokeTrayEntry(entry)
          },
        }
      case DesktopTrayActionKind.ATTACHED_HANDLER:
        return {
          label: entry.label,
          click: () => {
            void this.invokeTrayEntry(entry)
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

  private async handlePopoverAction(entryId: string): Promise<void> {
    const entry = (this.currentTrayState?.entries ?? []).find(
      (entry) => entry.id === entryId,
    )
    if (!entry) {
      return
    }
    await this.invokeTrayEntry(entry)
  }

  private async invokeTrayEntry(entry: DesktopTrayEntry): Promise<void> {
    const action = entry.action
    if (
      entry.kind !== DesktopTrayEntryKind.ACTION ||
      !(entry.enabled ?? false) ||
      !action
    ) {
      return
    }
    switch (action.kind) {
      case DesktopTrayActionKind.OPEN_ROUTE:
        await this.openRouteOrFocus(action.route)
        return
      case DesktopTrayActionKind.NEW_WINDOW:
        await this.openRoute(action.route || '/')
        return
      case DesktopTrayActionKind.COPY_TEXT:
        if (action.value) {
          this.copyText(action.value)
        }
        return
      case DesktopTrayActionKind.REVEAL_PATH:
        if (action.value) {
          this.revealPath(action.value)
        }
        return
      case DesktopTrayActionKind.QUIT:
        await this.quitDesktopRuntime()
        return
      case DesktopTrayActionKind.ATTACHED_HANDLER:
        await this.invokeAttachedTrayEntry(entry.id)
        return
      default:
        return
    }
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

  private async invokeAttachedTrayEntry(entryId?: string): Promise<void> {
    await this.opts.resource.desktopTrayResource.InvokeDesktopTrayEntry({
      entryId,
    })
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

export type { DesktopTrayControllerOpts, DesktopTrayResource }
