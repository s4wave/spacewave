import electron from 'electron'

import {
  DesktopRuntimeHealth,
  type DesktopRuntimeActionItem,
  type DesktopRuntimeActivityItem,
  type DesktopRuntimeAttentionItem,
  type DesktopRuntimeListenerStatus,
  type DesktopRuntimeNavigationItem,
  type DesktopRuntimeState as DesktopRuntimeStateMessage,
} from '../desktop-runtime/desktop-runtime.pb.js'

interface DesktopTrayPopoverControllerOpts {
  appName?: string
}

const popoverWidth = 360
const popoverHeight = 520
const popoverMargin = 8

// DesktopTrayPopoverController owns the dev-only custom tray popover prototype.
export class DesktopTrayPopoverController {
  private window?: Electron.BrowserWindow
  private disabled = false
  private state?: DesktopRuntimeStateMessage

  constructor(private readonly opts: DesktopTrayPopoverControllerOpts) {}

  public update(state: DesktopRuntimeStateMessage): void {
    this.state = state
    if (!this.window || this.window.isDestroyed()) {
      return
    }
    void this.render(this.window, state).catch((err: unknown) => {
      this.disable(err)
    })
  }

  public async toggle(
    tray: Electron.Tray | undefined,
    state: DesktopRuntimeStateMessage,
  ): Promise<boolean> {
    if (this.disabled || !tray) {
      return false
    }
    this.state = state
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
    const y =
      opensDown ?
        trayBounds.y + trayBounds.height + popoverMargin
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
    state: DesktopRuntimeStateMessage,
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
  state: DesktopRuntimeStateMessage,
): string {
  const health = healthLabel(state.health)
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
  padding: 18px;
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
  background: ${healthColor(state.health)};
  color: #101216;
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}
.section {
  padding: 14px 0;
  border-bottom: 1px solid #2c3038;
}
.section:last-child {
  border-bottom: 0;
}
.section-title {
  margin-bottom: 8px;
  color: #d6d8dd;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}
.row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10px;
  padding: 6px 0;
}
.label {
  font-size: 13px;
  font-weight: 650;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.detail {
  color: #a9adb7;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.status {
  align-self: start;
  color: #cfd2d8;
  font-size: 12px;
  white-space: nowrap;
}
.empty {
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
      <div class="pill">${escapeHtml(health)}</div>
    </section>
    ${renderListenerSection(state.listener)}
    ${renderAttentionSection(state.attentionItems)}
    ${renderNavigationSection('Sessions', state.sessions, 'No sessions')}
    ${renderNavigationSection('Spaces', state.spaces, 'No spaces')}
    ${renderActivitySection(state.activity)}
    ${renderActionSection(state.actions)}
  </main>
</body>
</html>`
}

function renderListenerSection(
  listener: DesktopRuntimeListenerStatus | undefined,
): string {
  if (!listener) {
    return renderSection('Status', '<div class="empty">Runtime starting</div>')
  }
  const socket =
    listener.socketPath ?
      `<div class="detail">${escapeHtml(listener.socketPath)}</div>`
    : ''
  return renderSection(
    'Status',
    `<div class="row"><div><div class="label">${escapeHtml(
      listener.label || 'CLI listener',
    )}</div><div class="detail">${escapeHtml(listener.detail || '')}</div>${socket}</div></div>`,
  )
}

function renderAttentionSection(
  items: DesktopRuntimeAttentionItem[] | undefined,
): string {
  if (!items?.length) {
    return ''
  }
  return renderSection(
    'Attention',
    items
      .map((item) =>
        renderRow(
          item.label || 'Needs attention',
          item.detail,
          severityLabel(item.severity),
        ),
      )
      .join(''),
  )
}

function renderNavigationSection(
  title: string,
  items: DesktopRuntimeNavigationItem[] | undefined,
  empty: string,
): string {
  if (!items?.length) {
    return renderSection(title, `<div class="empty">${escapeHtml(empty)}</div>`)
  }
  return renderSection(
    title,
    items
      .map((item) =>
        renderRow(item.label || 'Untitled', item.detail, item.statusText),
      )
      .join(''),
  )
}

function renderActivitySection(
  items: DesktopRuntimeActivityItem[] | undefined,
): string {
  if (!items?.length) {
    return ''
  }
  return renderSection(
    'Activity',
    items
      .map((item) => renderRow(item.label || 'Activity', item.detail))
      .join(''),
  )
}

function renderActionSection(
  items: DesktopRuntimeActionItem[] | undefined,
): string {
  if (!items?.length) {
    return ''
  }
  return renderSection(
    'Quick Actions',
    items
      .map((item) =>
        renderRow(
          item.label || 'Action',
          item.detail,
          item.enabled ? '' : 'Unavailable',
        ),
      )
      .join(''),
  )
}

function renderSection(title: string, body: string): string {
  return `<section class="section"><div class="section-title">${escapeHtml(
    title,
  )}</div>${body}</section>`
}

function renderRow(label: string, detail?: string, status?: string): string {
  return `<div class="row"><div><div class="label">${escapeHtml(
    label,
  )}</div>${detail ? `<div class="detail">${escapeHtml(detail)}</div>` : ''}</div>${
    status ? `<div class="status">${escapeHtml(status)}</div>` : ''
  }</div>`
}

function healthLabel(health: DesktopRuntimeHealth | undefined): string {
  switch (health) {
    case DesktopRuntimeHealth.ACTIVE:
      return 'Active'
    case DesktopRuntimeHealth.NEEDS_ATTENTION:
      return 'Attention'
    case DesktopRuntimeHealth.DISCONNECTED:
      return 'Offline'
    case DesktopRuntimeHealth.QUITTING:
      return 'Quitting'
    case DesktopRuntimeHealth.STARTING:
      return 'Starting'
    default:
      return 'Running'
  }
}

function healthColor(health: DesktopRuntimeHealth | undefined): string {
  switch (health) {
    case DesktopRuntimeHealth.ACTIVE:
      return '#69d2e7'
    case DesktopRuntimeHealth.NEEDS_ATTENTION:
      return '#f6c453'
    case DesktopRuntimeHealth.DISCONNECTED:
      return '#e26868'
    case DesktopRuntimeHealth.QUITTING:
      return '#aeb4c0'
    case DesktopRuntimeHealth.STARTING:
      return '#95d47a'
    default:
      return '#95d47a'
  }
}

function severityLabel(severity: number | undefined): string {
  if (!severity) {
    return ''
  }
  return (
    severity >= 3 ? 'Critical'
    : severity >= 2 ? 'Warning'
    : 'Info'
  )
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
