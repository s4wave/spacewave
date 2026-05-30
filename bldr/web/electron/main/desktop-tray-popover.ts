import electron from 'electron'

import type { DesktopTrayState } from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import type { DesktopRuntimeState } from '../desktop-runtime/desktop-runtime.pb.js'
import {
  buildDesktopTrayPanelDescriptor,
  type DesktopTrayPanelDescriptor,
  type DesktopTrayPanelRow,
  type DesktopTrayPanelSection,
  type DesktopTrayPanelSeverity,
} from './desktop-tray-panel-descriptor.js'

interface DesktopTrayPopoverControllerOpts {
  appName?: string
  actionHandler: (entryId: string) => Promise<void>
  runtimeState?: () => DesktopRuntimeState
}

const popoverWidth = 390
const popoverHeight = 560
const popoverMargin = 8
const navigationCardLimit = 6
const actionScheme = 'spacewave-tray-action:'
const commandScheme = 'spacewave-tray-command:'

// DesktopTrayPopoverController owns the opt-in rich Electron tray panel.
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
    return this.show(tray, state)
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

  public close(): void {
    if (!this.window || this.window.isDestroyed()) {
      this.window = undefined
      return
    }
    this.window.close()
    this.window = undefined
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
    win.webContents.on('before-input-event', (event, input) => {
      if (input.key !== 'Escape') {
        return
      }
      event.preventDefault()
      this.close()
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
    if (url.startsWith(commandScheme)) {
      event.preventDefault()
      if (url.slice(commandScheme.length) === 'close') {
        this.close()
      }
      return
    }
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
    state: DesktopTrayState,
  ): Promise<void> {
    const descriptor = buildDesktopTrayPanelDescriptor({
      appName: this.opts.appName || 'Spacewave',
      state,
      runtimeState: this.opts.runtimeState?.(),
    })
    await win.loadURL(
      `data:text/html;charset=utf-8,${encodeURIComponent(
        renderDesktopTrayPanelHtml(descriptor),
      )}`,
    )
  }

  private disable(err: unknown): void {
    console.error(
      'desktop tray panel disabled; using native menu fallback',
      err,
    )
    this.disabled = true
    this.close()
  }
}

export function renderDesktopTrayPanelHtml(
  descriptor: DesktopTrayPanelDescriptor,
): string {
  const tabs = renderableTabs(descriptor)
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
:root {
  color-scheme: light dark;
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", sans-serif;
  background: #f5f6f8;
  color: #1d232d;
}
* {
  box-sizing: border-box;
}
html,
body {
  min-width: 0;
  height: 100%;
  margin: 0;
  overflow: hidden;
  background: transparent;
}
body {
  display: flex;
}
.surface {
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  width: 100%;
  height: 100%;
  padding: 12px;
  background: #f5f6f8;
  color: #1d232d;
}
.header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 4px 2px 10px;
}
.title {
  min-width: 0;
  overflow: hidden;
  font-size: 15px;
  font-weight: 720;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subtitle {
  min-width: 0;
  margin-top: 2px;
  overflow: hidden;
  color: #617082;
  font-size: 12px;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.status-pill {
  min-width: 82px;
  padding: 5px 8px;
  border: 1px solid #c8d0da;
  border-radius: 7px;
  background: #ffffff;
  color: #344253;
  font-size: 11px;
  font-weight: 700;
  text-align: center;
}
.severity-info .status-pill,
.severity-warning .status-pill,
.severity-critical .status-pill {
  color: #1d232d;
}
.severity-info .status-pill {
  border-color: #9ccbed;
  background: #e8f5ff;
}
.severity-warning .status-pill {
  border-color: #d8ad40;
  background: #fff5d7;
}
.severity-critical .status-pill {
  border-color: #d48686;
  background: #ffe7e7;
}
.tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 4px;
  height: 31px;
  padding: 3px;
  border: 1px solid #d6dce4;
  border-radius: 7px;
  background: #ebeef3;
}
.tab {
  min-width: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: #5c6a7c;
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
}
.tab[aria-selected="true"] {
  background: #ffffff;
  color: #1d232d;
  box-shadow: 0 1px 2px rgb(20 25 31 / 12%);
}
.tab:disabled {
  color: #9aa4b2;
}
.content {
  min-height: 0;
  padding-top: 10px;
  overflow: hidden auto;
}
.content[data-tabs="collapsed"] {
  padding-top: 0;
}
.panel {
  display: none;
}
.panel[data-active="true"] {
  display: block;
}
.cards {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 7px;
  margin-bottom: 10px;
}
.card,
.nav-card,
.empty,
.section {
  border: 1px solid #d8dee7;
  border-radius: 8px;
  background: #ffffff;
}
.card {
  min-width: 0;
  min-height: 76px;
  padding: 9px;
}
.card-label,
.section-title {
  color: #617082;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
}
.card-value {
  margin-top: 5px;
  color: #1d232d;
  font-size: 18px;
  font-weight: 760;
  line-height: 1.05;
}
.card-detail {
  min-width: 0;
  margin-top: 5px;
  overflow: hidden;
  color: #526174;
  font-size: 11px;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card.severity-warning {
  border-color: #d8ad40;
}
.card.severity-critical {
  border-color: #d48686;
}
.primary-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
  margin-bottom: 10px;
}
.primary-actions .row {
  min-height: 36px;
}
.nav-cards {
  display: grid;
  gap: 7px;
}
.nav-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
  min-height: 52px;
  padding: 9px 10px;
  color: inherit;
  text-align: left;
  text-decoration: none;
}
.nav-card.action {
  cursor: default;
}
.nav-card.action:hover,
.nav-card.action:focus,
.nav-card.action:focus-visible {
  border-color: #b7cce8;
  background: #edf4ff;
  outline: none;
}
.nav-card.action:focus-visible {
  box-shadow: 0 0 0 2px rgb(90 142 208 / 26%);
}
.nav-card.action[aria-disabled="true"] {
  color: #98a2af;
  pointer-events: none;
}
.nav-card.disabled-action {
  color: #98a2af;
}
.nav-card.active {
  border-color: #87c6aa;
  background: #f0fbf6;
}
.nav-card[aria-current="page"] {
  border-color: #70b795;
}
.nav-card-overflow {
  padding: 4px 2px;
  color: #717f91;
  font-size: 11px;
  font-weight: 650;
  text-align: center;
}
.section {
  margin-bottom: 8px;
  overflow: hidden;
}
.section-title {
  display: flex;
  align-items: center;
  height: 27px;
  padding: 0 10px;
  border-bottom: 1px solid #edf0f4;
  background: #fafbfc;
}
.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-height: 40px;
  width: 100%;
  padding: 7px 10px;
  border: 0;
  border-bottom: 1px solid #edf0f4;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  text-decoration: none;
}
.row:last-child {
  border-bottom: 0;
}
.row.action {
  cursor: default;
}
.row.action:hover,
.row.action:focus,
.row.action:focus-visible {
  background: #edf4ff;
  outline: none;
}
.row.action:focus-visible {
  box-shadow: inset 0 0 0 2px rgb(90 142 208 / 26%);
}
.row.action[aria-disabled="true"] {
  color: #98a2af;
  pointer-events: none;
}
.row.disabled-action {
  color: #98a2af;
}
.row[aria-current="page"] {
  background: #f0fbf6;
}
.row.active .label::before,
.nav-card.active .label::before {
  content: "";
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 6px;
  border-radius: 99px;
  background: #2aa876;
  vertical-align: 1px;
}
.label,
.detail {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.label {
  font-size: 12px;
  font-weight: 680;
  line-height: 1.25;
}
.detail {
  margin-top: 2px;
  color: #617082;
  font-size: 11px;
  line-height: 1.25;
}
.status {
  min-width: 0;
  max-width: 96px;
  overflow: hidden;
  color: #647386;
  font-size: 11px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row.action .status,
.nav-card.action .status {
  max-width: 118px;
  padding: 2px 5px;
  border: 1px solid #d5dbe4;
  border-radius: 5px;
  background: #f7f9fb;
  color: #536172;
  font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  font-size: 10px;
  line-height: 1.1;
}
.row.disabled-action .status,
.nav-card.disabled-action .status {
  opacity: 0.75;
}
.row.severity-info .status {
  color: #2573a7;
}
.row.severity-warning .status {
  color: #9b6b00;
}
.row.severity-critical .status {
  color: #b12f35;
}
.empty {
  padding: 18px 10px;
  color: #717f91;
  font-size: 12px;
  text-align: center;
}
@media (prefers-color-scheme: dark) {
  :root {
    background: #17191d;
    color: #f1f3f6;
  }
  .surface {
    background: #17191d;
    color: #f1f3f6;
  }
  .subtitle,
  .card-label,
  .section-title,
  .detail,
  .status {
    color: #a5afbd;
  }
  .status-pill,
  .tab[aria-selected="true"],
  .card,
  .nav-card,
  .empty,
  .section {
    border-color: #333944;
    background: #20242b;
    color: #f1f3f6;
  }
  .tabs {
    border-color: #333944;
    background: #1d2026;
  }
  .tab {
    color: #a5afbd;
  }
  .section-title {
    border-color: #2a3039;
    background: #242932;
  }
  .row {
    border-color: #2a3039;
  }
  .row.action:hover,
  .row.action:focus,
  .row.action:focus-visible,
  .nav-card.action:hover,
  .nav-card.action:focus,
  .nav-card.action:focus-visible {
    background: #26354a;
  }
  .row[aria-current="page"] {
    background: #1f342b;
  }
  .nav-card.active {
    border-color: #3b7458;
    background: #1f342b;
  }
  .row.action .status,
  .nav-card.action .status {
    border-color: #3a424e;
    background: #171b21;
    color: #c3ccd8;
  }
  .card-value,
  .title {
    color: #f1f3f6;
  }
  .card-detail {
    color: #b2bbc8;
  }
  .severity-info .status-pill {
    border-color: #356b91;
    background: #1d3447;
  }
  .severity-warning .status-pill {
    border-color: #8b6c20;
    background: #3d3118;
  }
  .severity-critical .status-pill {
    border-color: #8b4548;
    background: #402222;
  }
}
</style>
</head>
<body class="${severityClass(descriptor.severity)}">
  <main class="surface">
    <section class="header">
      <div>
        <div class="title">${escapeHtml(descriptor.title)}</div>
        <div class="subtitle">${escapeHtml(descriptor.subtitle)}</div>
      </div>
      <div class="status-pill">${escapeHtml(descriptor.icon.variant)}</div>
    </section>
    ${renderTabs(tabs)}
    ${renderPanels(descriptor, tabs)}
  </main>
  ${renderPanelScript()}
</body>
</html>`
}

function renderableTabs(
  descriptor: DesktopTrayPanelDescriptor,
): DesktopTrayPanelDescriptor['tabs'] {
  return descriptor.tabs.filter((tab) => tab.id === 'overview' || tab.enabled)
}

function renderTabs(tabs: DesktopTrayPanelDescriptor['tabs']): string {
  if (tabs.length <= 1) {
    return ''
  }
  return `<nav class="tabs" aria-label="Tray panel views">
    ${tabs
      .map(
        (tab) =>
          `<button class="tab" type="button" data-tab="${tab.id}" aria-selected="${
            tab.id === 'overview' ? 'true' : 'false'
          }" ${tab.enabled ? '' : 'disabled'}>${escapeHtml(tab.label)}</button>`,
      )
      .join('')}
  </nav>`
}

function renderPanels(
  descriptor: DesktopTrayPanelDescriptor,
  tabs: DesktopTrayPanelDescriptor['tabs'],
): string {
  const enabled = new Set(tabs.map((tab) => tab.id))
  return `<section class="content" data-tabs="${
    tabs.length > 1 ? 'visible' : 'collapsed'
  }">
      <div class="panel" data-panel="overview" data-active="true">
        ${renderCards(descriptor)}
        ${renderPrimaryActions(descriptor.primaryActions)}
        ${renderAttention(descriptor.attentionRows)}
        ${renderSections(descriptor)}
      </div>
      ${
        enabled.has('sessions') ?
          `<div class="panel" data-panel="sessions">
        ${renderRowsPanel('Sessions', descriptor.sessionRows, 'No sessions')}
      </div>`
        : ''
      }
      ${
        enabled.has('spaces') ?
          `<div class="panel" data-panel="spaces">
        ${renderRowsPanel('Spaces', descriptor.spaceRows, 'No spaces')}
      </div>`
        : ''
      }
    </section>`
}

function renderCards(descriptor: DesktopTrayPanelDescriptor): string {
  return `<section class="cards">
    ${descriptor.cards
      .map(
        (card) => `<article class="card ${severityClass(card.severity)}">
          <div class="card-label">${escapeHtml(card.label)}</div>
          <div class="card-value">${escapeHtml(card.value)}</div>
          <div class="card-detail">${escapeHtml(card.detail)}</div>
        </article>`,
      )
      .join('')}
  </section>`
}

function renderPrimaryActions(rows: DesktopTrayPanelRow[]): string {
  if (!rows.length) {
    return ''
  }
  return `<section class="primary-actions">${rows.map(renderRow).join('')}</section>`
}

function renderAttention(rows: DesktopTrayPanelRow[]): string {
  if (!rows.length) {
    return ''
  }
  return `<section class="section">
    <div class="section-title">Attention</div>
    ${rows.map(renderRow).join('')}
  </section>`
}

function renderSections(descriptor: DesktopTrayPanelDescriptor): string {
  const primaryIDs = new Set(descriptor.primaryActions.map((row) => row.id))
  return descriptor.sections
    .filter(
      (section) => section.title !== 'Sessions' && section.title !== 'Spaces',
    )
    .map((section) => {
      const rows =
        section.title === 'Overview' ?
          section.rows.filter((row) => !primaryIDs.has(row.id))
        : section.rows
      return rows.length ? renderSection({ ...section, rows }) : ''
    })
    .join('')
}

function renderSection(section: DesktopTrayPanelSection): string {
  return `<section class="section" data-section="${escapeHtml(section.id)}">
    <div class="section-title">${escapeHtml(section.title)}</div>
    ${section.rows.map(renderRow).join('')}
  </section>`
}

function renderRowsPanel(
  title: string,
  rows: DesktopTrayPanelRow[],
  emptyLabel: string,
): string {
  const visible = rows.filter((row) => !row.empty)
  if (!visible.length) {
    return `<div class="empty">${escapeHtml(emptyLabel)}</div>`
  }
  const shown = visible.slice(0, navigationCardLimit)
  const remaining = visible.length - shown.length
  return `<section class="nav-cards" data-card-panel="${escapeHtml(
    title.toLowerCase(),
  )}">
    ${shown.map(renderNavigationCard).join('')}
    ${remaining > 0 ? renderNavigationOverflow(title, remaining) : ''}
  </section>`
}

function renderRow(row: DesktopTrayPanelRow): string {
  if (!row.action) {
    return `<div class="${rowClass(row, '')}"${rowStateAttributes(
      row,
    )}>${renderRowContents(row)}</div>`
  }
  return `<a class="${rowClass(row, 'action')}" href="${actionHref(
    row.action.id,
  )}" role="button" tabindex="0" data-action-id="${escapeHtml(
    row.action.id,
  )}" aria-disabled="${row.enabled ? 'false' : 'true'}"${rowStateAttributes(
    row,
  )}>${renderRowContents(row)}</a>`
}

function renderNavigationCard(row: DesktopTrayPanelRow): string {
  if (!row.action) {
    return `<div class="${navigationCardClass(row, '')}"${rowStateAttributes(
      row,
    )}>${renderRowContents(row)}</div>`
  }
  return `<a class="${navigationCardClass(row, 'action')}" href="${actionHref(
    row.action.id,
  )}" role="button" tabindex="0" data-action-id="${escapeHtml(
    row.action.id,
  )}" aria-disabled="${row.enabled ? 'false' : 'true'}"${rowStateAttributes(
    row,
  )}>${renderRowContents(row)}</a>`
}

function renderNavigationOverflow(title: string, count: number): string {
  const noun =
    title === 'Sessions' ? 'session'
    : title === 'Spaces' ? 'space'
    : 'item'
  return `<div class="nav-card-overflow">+${count} more ${noun}${
    count === 1 ? '' : 's'
  }</div>`
}

function renderRowContents(row: DesktopTrayPanelRow): string {
  const detail = row.detail || ''
  const status = row.statusText || ''
  return `<div>
    <div class="label">${escapeHtml(row.label)}</div>
    ${detail ? `<div class="detail">${escapeHtml(detail)}</div>` : ''}
  </div>${status ? `<div class="status">${escapeHtml(status)}</div>` : ''}`
}

function rowClass(row: DesktopTrayPanelRow, extra: string): string {
  return [
    'row',
    extra,
    row.active ? 'active' : '',
    row.kind === 'action' && !row.enabled ? 'disabled-action' : '',
    severityClass(row.severity),
    row.empty ? 'empty-row' : '',
  ]
    .filter(Boolean)
    .join(' ')
}

function navigationCardClass(row: DesktopTrayPanelRow, extra: string): string {
  return [
    'nav-card',
    extra,
    row.active ? 'active' : '',
    row.kind === 'action' && !row.enabled ? 'disabled-action' : '',
    severityClass(row.severity),
  ]
    .filter(Boolean)
    .join(' ')
}

function rowStateAttributes(row: DesktopTrayPanelRow): string {
  return [
    row.active ? 'aria-current="page"' : '',
    row.kind === 'action' && !row.enabled ? 'aria-disabled="true"' : '',
  ]
    .filter(Boolean)
    .map((attr) => ` ${attr}`)
    .join('')
}

function actionHref(entryId: string): string {
  return `${actionScheme}${encodeURIComponent(entryId)}`
}

function renderPanelScript(): string {
  return `<script>
(() => {
  const tabs = Array.from(document.querySelectorAll('[data-tab]'));
  const panels = Array.from(document.querySelectorAll('[data-panel]'));
  const lastFocusByPanel = new Map();
  const isVisible = (item) => item.getClientRects().length > 0;
  const activePanel = () => panels.find((panel) => panel.dataset.active === 'true') || document;
  const actions = (scope = activePanel()) => Array.from(scope.querySelectorAll('[data-action-id]:not([aria-disabled="true"])')).filter(isVisible);
  const focusPanelAction = (id) => {
    const panel = activePanel();
    const previous = lastFocusByPanel.get(id);
    const target = previous && isVisible(previous) ? previous : actions(panel)[0];
    if (target) target.focus();
  };
  function selectTab(id) {
    for (const tab of tabs) tab.setAttribute('aria-selected', String(tab.dataset.tab === id));
    for (const panel of panels) panel.dataset.active = String(panel.dataset.panel === id);
    focusPanelAction(id);
  }
  for (const tab of tabs) {
    tab.addEventListener('click', () => selectTab(tab.dataset.tab));
  }
  document.addEventListener('focusin', (event) => {
    if (!event.target?.dataset?.actionId) return;
    const panel = event.target.closest('[data-panel]');
    if (panel?.dataset?.panel) lastFocusByPanel.set(panel.dataset.panel, event.target);
  });
  document.addEventListener('keydown', (event) => {
    const items = actions();
    const index = items.indexOf(document.activeElement);
    if (event.key === 'Escape') {
      event.preventDefault();
      window.location.href = '${commandScheme}close';
      return;
    }
    if (!items.length) return;
    if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
      event.preventDefault();
      items[(index + 1 + items.length) % items.length].focus();
      return;
    }
    if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
      event.preventDefault();
      items[(index - 1 + items.length) % items.length].focus();
      return;
    }
    if (event.key === 'Home') {
      event.preventDefault();
      items[0].focus();
      return;
    }
    if (event.key === 'End') {
      event.preventDefault();
      items[items.length - 1].focus();
      return;
    }
    if ((event.key === 'Enter' || event.key === ' ') && document.activeElement?.dataset?.actionId) {
      event.preventDefault();
      document.activeElement.click();
    }
  });
  focusPanelAction('overview');
})();
</script>`
}

function severityClass(severity: DesktopTrayPanelSeverity): string {
  switch (severity) {
    case 'critical':
      return 'severity-critical'
    case 'info':
      return 'severity-info'
    case 'warning':
      return 'severity-warning'
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
