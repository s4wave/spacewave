import os from 'os'
import electron from 'electron'

import type { ElectronInit } from '../../plugin/electron/electron.pb.js'
import {
  DesktopRuntimeHealth,
  DesktopRuntimeActionKind,
  DesktopRuntimeSeverity,
  DesktopRuntimeState,
  type DesktopRuntimeActionItem,
  type DesktopRuntimeActivityItem,
  type DesktopRuntimeAttentionItem,
  type DesktopRuntimeState as DesktopRuntimeStateMessage,
  type DesktopRuntimeNavigationItem,
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
    this.updateIconState(state)
    this.tray?.setContextMenu(this.buildMenu(state))
  }

  private buildMenu(state: DesktopRuntimeStateMessage): Electron.Menu {
    if (state.attentionItems?.length) {
      return electron.Menu.buildFromTemplate(this.buildAttentionTemplate(state))
    }
    return electron.Menu.buildFromTemplate(this.buildHealthyTemplate(state))
  }

  private buildHealthyTemplate(
    state: DesktopRuntimeStateMessage,
  ): Electron.MenuItemConstructorOptions[] {
    return [
      disabledItem(`Spacewave: ${state.statusText || 'Running'}`),
      { type: 'separator' },
      {
        label: 'Open Spacewave',
        click: () => {
          void this.openOrFocusMainWindow()
        },
      },
      {
        label: 'New Window',
        click: () => {
          void this.openOrFocusMainWindow()
        },
      },
      { type: 'separator' },
      ...this.buildStatusSection(state),
      ...this.buildNavigationSection('Sessions', state.sessions, 'No sessions'),
      ...this.buildNavigationSection('Spaces', state.spaces, 'No spaces'),
      ...this.buildActivitySection(state.activity),
      ...this.buildActionSection(state),
      { type: 'separator' },
      {
        label: 'Settings...',
        click: () => {
          void this.openRoute('/settings')
        },
      },
      {
        label: 'About Spacewave',
        click: () => {
          void this.openRoute('/about')
        },
      },
      { type: 'separator' },
      {
        label: 'Quit',
        click: () => {
          void this.quitDesktopRuntime()
        },
      },
    ]
  }

  private buildAttentionTemplate(
    state: DesktopRuntimeStateMessage,
  ): Electron.MenuItemConstructorOptions[] {
    const item = selectPrimaryAttentionItem(state.attentionItems)
    return [
      disabledItem(`Spacewave: ${state.statusText || 'Needs attention'}`),
      disabledItem(item?.label || 'Needs attention'),
      ...(item?.detail ? [disabledItem(item.detail)] : []),
      { type: 'separator' },
      {
        label: 'Open Spacewave',
        click: () => {
          void this.openOrFocusMainWindow()
        },
      },
      { type: 'separator' },
      {
        label: 'Quit',
        click: () => {
          void this.quitDesktopRuntime()
        },
      },
    ]
  }

  private buildStatusSection(
    state: DesktopRuntimeStateMessage,
  ): Electron.MenuItemConstructorOptions[] {
    const listener = state.listener
    const rows = [
      disabledItem('Status'),
      disabledItem(
        compactLabel([
          listener?.label || 'Runtime',
          listener?.detail || state.statusText || 'Running',
        ]),
      ),
    ]
    if (state.update?.label) {
      rows.push(
        disabledItem(
          compactLabel(['Update', state.update.label, state.update.detail]),
        ),
      )
    }
    return rows
  }

  private buildNavigationSection(
    title: string,
    items: DesktopRuntimeNavigationItem[] | undefined,
    emptyLabel: string,
  ): Electron.MenuItemConstructorOptions[] {
    return [
      { type: 'separator' },
      disabledItem(title),
      ...nonEmpty(items).map((item) =>
        this.buildNavigationItem(item),
      ),
      ...(items?.length ? [] : [disabledItem(emptyLabel)]),
    ]
  }

  private buildActivitySection(
    items: DesktopRuntimeActivityItem[] | undefined,
  ): Electron.MenuItemConstructorOptions[] {
    return [
      { type: 'separator' },
      disabledItem('Activity'),
      ...nonEmpty(items).map((item) =>
        disabledItem(compactLabel([item.label, item.detail])),
      ),
      ...(items?.length ? [] : [disabledItem('No recent activity')]),
    ]
  }

  private buildActionSection(
    state: DesktopRuntimeStateMessage,
  ): Electron.MenuItemConstructorOptions[] {
    const items = [
      ...this.buildSyntheticActions(state),
      ...nonEmpty(state.actions),
    ]
    return [
      { type: 'separator' },
      disabledItem('Quick Actions'),
      ...items.map((item) => this.buildActionItem(item)),
      ...(items.length ? [] : [disabledItem('No quick actions')]),
    ]
  }

  private buildNavigationItem(
    item: DesktopRuntimeNavigationItem,
  ): Electron.MenuItemConstructorOptions {
    const label = compactLabel([item.label, item.detail, item.statusText])
    if (!item.route) {
      return disabledItem(label)
    }
    return {
      label,
      click: () => {
        void this.openRoute(item.route)
      },
    }
  }

  private buildSyntheticActions(
    state: DesktopRuntimeStateMessage,
  ): DesktopRuntimeActionItem[] {
    const socketPath = state.listener?.socketPath
    if (!socketPath) {
      return []
    }
    return [
      {
        id: 'copy-cli-socket',
        kind: DesktopRuntimeActionKind.COPY_TEXT,
        label: 'Copy CLI Socket',
        value: socketPath,
        enabled: true,
      },
      {
        id: 'copy-diagnostics',
        kind: DesktopRuntimeActionKind.COPY_TEXT,
        label: 'Copy Diagnostics',
        value: buildDiagnosticText(state),
        enabled: true,
      },
    ]
  }

  private buildActionItem(
    item: DesktopRuntimeActionItem,
  ): Electron.MenuItemConstructorOptions {
    const label = compactLabel([item.label, item.detail])
    if (!item.enabled) {
      return disabledItem(label)
    }
    switch (item.kind) {
      case DesktopRuntimeActionKind.OPEN_ROUTE:
      case DesktopRuntimeActionKind.NEW_WINDOW:
        return {
          label,
          click: () => {
            void this.openRoute(item.route)
          },
        }
      case DesktopRuntimeActionKind.COPY_TEXT:
        if (!item.value) {
          return disabledItem(label)
        }
        return {
          label,
          click: () => {
            this.copyText(item.value)
          },
        }
      case DesktopRuntimeActionKind.REVEAL_PATH:
        if (!item.value) {
          return disabledItem(label)
        }
        return {
          label,
          click: () => {
            this.revealPath(item.value)
          },
        }
      case DesktopRuntimeActionKind.QUIT:
        return {
          label,
          click: () => {
            void this.quitDesktopRuntime()
          },
        }
      default:
        return disabledItem(label)
    }
  }

  private updateIconState(state: DesktopRuntimeStateMessage): void {
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

function compactLabel(parts: Array<string | undefined>): string {
  return parts.filter((part) => part && part.trim() !== '').join(' - ')
}

function nonEmpty<T>(items: T[] | undefined): T[] {
  return items ?? []
}

function selectPrimaryAttentionItem(
  items: DesktopRuntimeAttentionItem[] | undefined,
): DesktopRuntimeAttentionItem | undefined {
  return [...nonEmpty(items)].sort((a, b) => {
    const severity = severityPriority(b.severity) - severityPriority(a.severity)
    if (severity !== 0) {
      return severity
    }
    return a.label.localeCompare(b.label)
  })[0]
}

function severityPriority(severity: DesktopRuntimeSeverity | undefined): number {
  return severity ?? DesktopRuntimeSeverity.INFO
}

function trayTitleForState(state: DesktopRuntimeStateMessage): string {
  switch (state.health) {
    case DesktopRuntimeHealth.ACTIVE:
      return '*'
    case DesktopRuntimeHealth.NEEDS_ATTENTION:
      return '!'
    case DesktopRuntimeHealth.DISCONNECTED:
      return 'x'
    case DesktopRuntimeHealth.QUITTING:
      return '...'
    case DesktopRuntimeHealth.STARTING:
      return '~'
    default:
      return ''
  }
}

function buildDiagnosticText(state: DesktopRuntimeStateMessage): string {
  return [
    `Spacewave: ${state.statusText || 'Running'}`,
    compactLabel([
      state.listener?.label || 'Runtime',
      state.listener?.detail,
    ]),
    state.listener?.socketPath ? `Socket: ${state.listener.socketPath}` : '',
  ]
    .filter((line) => line !== '')
    .join('\n')
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
