import electron from 'electron'

import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayEntry,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'

interface DesktopTrayPopoverControllerOpts {
  appName?: string
  actionHandler: (entryId: string) => Promise<void>
}

const popoverWidth = 360
const popoverHeight = 520
const popoverMargin = 8
const actionScheme = 'spacewave-tray-action:'

// DesktopTrayPopoverController owns the dev-only custom tray popover prototype.
export class DesktopTrayPopoverController {
  private window?: Electron.BrowserWindow
  private disabled = false

  constructor(private readonly opts: DesktopTrayPopoverControllerOpts) {}

  public update(state: DesktopTrayState): void {
    if (!this.window || this.window.isDestroyed()) {
      return
    }
    void this.render(this.window, state).catch((err: unknown) => {
      this.disable(err)
    })
  }

  public async toggle(
    tray: Electron.Tray | undefined,
    state: DesktopTrayState,
  ): Promise<boolean> {
    if (this.disabled || !tray) {
      return false
    }
    if (this.window && !this.window.isDestroyed()) {
      this.close()
      return true
    }
    try {
      const win = this.createWindow(tray)
      this.window = win
      await this.render(win, state)
      win.show()
      return true
    } catch (err) {
      this.disable(err)
      return false
    }
  }

  public async show(
    tray: Electron.Tray | undefined,
    state: DesktopTrayState,
  ): Promise<boolean> {
    if (this.disabled || !tray) {
      return false
    }
    if (this.window && !this.window.isDestroyed()) {
      await this.render(this.window, state)
      this.window.show()
      return true
    }
    try {
      const win = this.createWindow(tray)
      this.window = win
      await this.render(win, state)
      win.show()
      return true
    } catch (err) {
      this.disable(err)
      return false
    }
  }

  public async capturePNG(): Promise<Buffer | undefined> {
    if (!this.window || this.window.isDestroyed()) {
      return undefined
    }
    const image = await this.window.capturePage()
    return image.toPNG()
  }

  private createWindow(tray: Electron.Tray): Electron.BrowserWindow {
    const win = new electron.BrowserWindow({
      width: popoverWidth,
      height: popoverHeight,
      show: false,
      frame: false,
      resizable: false,
      movable: false,
      minimizable: false,
      maximizable: false,
      fullscreenable: false,
      skipTaskbar: true,
      alwaysOnTop: true,
      title: `${this.opts.appName || 'Spacewave'} Status`,
      webPreferences: {
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: true,
      },
    })
    win.setBounds(this.getWindowBounds(tray))
    win.webContents.on('will-navigate', (event, url) => {
      void this.handleNavigation(event, url).catch((err: unknown) => {
        this.disable(err)
        if (!win.isDestroyed()) {
          win.close()
        }
      })
    })
    win.on('blur', () => {
      this.close()
    })
    win.on('closed', () => {
      if (this.window === win) {
        this.window = undefined
      }
    })
    return win
  }

  private async handleNavigation(
    event: Electron.Event,
    url: string,
  ): Promise<void> {
    if (!url.startsWith(actionScheme)) {
      return
    }
    event.preventDefault()
    const entryId = decodeURIComponent(url.slice(actionScheme.length))
    await this.opts.actionHandler(entryId)
    this.close()
  }

  private getWindowBounds(tray: Electron.Tray): Electron.Rectangle {
    const trayBounds = tray.getBounds()
    const display = electron.screen.getDisplayMatching(trayBounds)
    const workArea = display.workArea
    const x = clamp(
      Math.round(trayBounds.x + trayBounds.width / 2 - popoverWidth / 2),
      workArea.x,
      workArea.x + workArea.width - popoverWidth,
    )
    const opensDown =
      trayBounds.y < workArea.y + Math.floor(workArea.height / 2)
    const y = opensDown
      ? trayBounds.y + trayBounds.height + popoverMargin
      : trayBounds.y - popoverHeight - popoverMargin
    return {
      x,
      y: clamp(y, workArea.y, workArea.y + workArea.height - popoverHeight),
      width: popoverWidth,
      height: popoverHeight,
    }
  }

  private async render(
    win: Electron.BrowserWindow,
    state: DesktopTrayState,
  ): Promise<void> {
    await win.loadURL(
      `data:text/html;charset=utf-8,${encodeURIComponent(
        renderDesktopTrayPopoverHtml(this.opts.appName || 'Spacewave', state),
      )}`,
    )
  }

  private close(): void {
    if (!this.window || this.window.isDestroyed()) {
      this.window = undefined
      return
    }
    this.window.close()
    this.window = undefined
  }

  private disable(err: unknown): void {
    console.error(
      'desktop tray popover disabled; using native menu fallback',
      err,
    )
    this.disabled = true
    this.close()
  }
}

function renderDesktopTrayPopoverHtml(
  appName: string,
  state: DesktopTrayState,
): string {
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
:root {
  color-scheme: dark;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  background: #181a1f;
  color: #f2f3f5;
}
* {
  box-sizing: border-box;
}
body {
  margin: 0;
  background: #181a1f;
}
.surface {
  min-height: 100vh;
  padding: 16px;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 14px;
  border-bottom: 1px solid #343841;
}
.title {
  font-size: 17px;
  font-weight: 700;
}
.subtle {
  color: #a9adb7;
  font-size: 12px;
}
.pill {
  border-radius: 999px;
  padding: 4px 9px;
  background: ${iconStateColor(state.iconState)};
  color: #101216;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}
.entries {
  padding-top: 10px;
}
.section-title,
.path-title {
  padding: 10px 0 4px;
  color: #d6d8dd;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}
.path-title {
  color: #a9adb7;
  font-size: 11px;
}
.separator {
  height: 1px;
  margin: 10px 0;
  background: #2c3038;
}
.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  width: 100%;
  min-height: 34px;
  padding: 7px 8px;
  border-radius: 6px;
  color: inherit;
  text-align: left;
  text-decoration: none;
}
.row.action {
  cursor: default;
}
.row.action:hover {
  background: #242831;
}
.row.inactive {
  color: #8d929d;
}
.label {
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.detail {
  min-width: 0;
  overflow: hidden;
  color: #a9adb7;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.status {
  align-self: start;
  color: #cfd2d8;
  font-size: 12px;
  white-space: nowrap;
}
.severity-info .status {
  color: #9fd7ff;
}
.severity-warning .status {
  color: #f6c453;
}
.severity-critical .status {
  color: #ff9f9f;
}
.active .label::before {
  content: "";
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 6px;
  border-radius: 999px;
  background: #95d47a;
  vertical-align: 1px;
}
.empty {
  padding: 16px 0;
  color: #8d929d;
  font-size: 12px;
}
</style>
</head>
<body>
  <main class="surface">
    <section class="header">
      <div>
        <div class="title">${escapeHtml(appName)}</div>
        <div class="subtle">${escapeHtml(state.statusText || 'Running')}</div>
      </div>
      <div class="pill">${escapeHtml(iconStateLabel(state.iconState))}</div>
    </section>
    <section class="entries">
      ${renderEntries(state.entries ?? [])}
    </section>
  </main>
</body>
</html>`
}

function renderEntries(entries: DesktopTrayEntry[]): string {
  if (!entries.length) {
    return '<div class="empty">Runtime starting</div>'
  }
  let currentPath = ''
  return entries
    .map((entry) => {
      const nextPath = (entry.path ?? [])
        .filter((part) => part !== '')
        .join(' / ')
      const pathHeader =
        nextPath && nextPath !== currentPath
          ? `<div class="path-title">${escapeHtml(nextPath)}</div>`
          : ''
      currentPath = nextPath
      return pathHeader + renderEntry(entry)
    })
    .join('')
}

function renderEntry(entry: DesktopTrayEntry): string {
  switch (entry.kind) {
    case DesktopTrayEntryKind.SEPARATOR:
      return '<div class="separator"></div>'
    case DesktopTrayEntryKind.SECTION:
    case DesktopTrayEntryKind.SUBMENU:
      return `<div class="section-title">${escapeHtml(entry.label || '')}</div>`
    case DesktopTrayEntryKind.ACTION:
      return renderActionEntry(entry)
    default:
      return renderStaticEntry(entry)
  }
}

function renderActionEntry(entry: DesktopTrayEntry): string {
  if (!canInvokeEntry(entry)) {
    return renderStaticEntry(entry, 'Unavailable')
  }
  return `<a class="${rowClass(entry, 'action')}" href="${actionHref(entry.id)}">${renderRowContents(
    entry,
    actionStatus(entry),
  )}</a>`
}

function canInvokeEntry(entry: DesktopTrayEntry): boolean {
  if (!(entry.enabled ?? false) || !entry.action) {
    return false
  }
  switch (entry.action.kind) {
    case DesktopTrayActionKind.OPEN_ROUTE:
    case DesktopTrayActionKind.NEW_WINDOW:
    case DesktopTrayActionKind.QUIT:
    case DesktopTrayActionKind.ATTACHED_HANDLER:
      return true
    case DesktopTrayActionKind.COPY_TEXT:
    case DesktopTrayActionKind.REVEAL_PATH:
      return !!entry.action.value
    default:
      return false
  }
}

function renderStaticEntry(entry: DesktopTrayEntry, status?: string): string {
  return `<div class="${rowClass(entry, 'inactive')}">${renderRowContents(
    entry,
    status || entry.statusText,
  )}</div>`
}

function renderRowContents(entry: DesktopTrayEntry, status?: string): string {
  return `<div><div class="label">${escapeHtml(entry.label || '')}</div>${
    entry.detail ? `<div class="detail">${escapeHtml(entry.detail)}</div>` : ''
  }</div>${status ? `<div class="status">${escapeHtml(status)}</div>` : ''}`
}

function rowClass(entry: DesktopTrayEntry, extra: string): string {
  return [
    'row',
    extra,
    entry.active ? 'active' : '',
    severityClass(entry.severity),
  ]
    .filter((part) => part !== '')
    .join(' ')
}

function actionStatus(entry: DesktopTrayEntry): string {
  if (entry.statusText) {
    return entry.statusText
  }
  if (entry.action?.kind === DesktopTrayActionKind.COPY_TEXT) {
    return 'Copy'
  }
  if (entry.action?.kind === DesktopTrayActionKind.REVEAL_PATH) {
    return 'Reveal'
  }
  return ''
}

function actionHref(entryId: string | undefined): string {
  return `${actionScheme}${encodeURIComponent(entryId || '')}`
}

function iconStateLabel(iconState: DesktopTrayIconState | undefined): string {
  switch (iconState) {
    case DesktopTrayIconState.ACTIVE:
      return 'Active'
    case DesktopTrayIconState.ATTENTION:
      return 'Attention'
    case DesktopTrayIconState.DISCONNECTED:
      return 'Offline'
    case DesktopTrayIconState.QUITTING:
      return 'Quitting'
    default:
      return 'Running'
  }
}

function iconStateColor(iconState: DesktopTrayIconState | undefined): string {
  switch (iconState) {
    case DesktopTrayIconState.ACTIVE:
      return '#69d2e7'
    case DesktopTrayIconState.ATTENTION:
      return '#f6c453'
    case DesktopTrayIconState.DISCONNECTED:
      return '#e26868'
    case DesktopTrayIconState.QUITTING:
      return '#aeb4c0'
    default:
      return '#95d47a'
  }
}

function severityClass(severity: DesktopTraySeverity | undefined): string {
  switch (severity) {
    case DesktopTraySeverity.INFO:
      return 'severity-info'
    case DesktopTraySeverity.WARNING:
      return 'severity-warning'
    case DesktopTraySeverity.CRITICAL:
      return 'severity-critical'
    default:
      return ''
  }
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}
