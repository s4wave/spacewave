import {
  DesktopTrayIconState,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'

export type DesktopTrayIconVariant =
  | 'active'
  | 'attention'
  | 'disconnected'
  | 'healthy'
  | 'quitting'

export interface DesktopTrayIconModel {
  appName: string
  statusText: string
  tooltip: string
  state: DesktopTrayIconState
  variant: DesktopTrayIconVariant
  fallbackTitle: string
  dynamicIconEnabled: boolean
  renderKey: string
}

export interface BuildDesktopTrayIconModelOpts {
  appName?: string
  state: DesktopTrayState
  dynamicIconEnabled?: boolean
}

export function buildDesktopTrayIconModel(
  opts: BuildDesktopTrayIconModelOpts,
): DesktopTrayIconModel {
  const appName = opts.appName || 'Spacewave'
  const statusText = opts.state.statusText || 'Running'
  const state = opts.state.iconState ?? DesktopTrayIconState.NORMAL
  const dynamicIconEnabled = opts.dynamicIconEnabled ?? false
  const variant = iconVariant(state)
  return {
    appName,
    statusText,
    tooltip: `${appName}: ${statusText}`,
    state,
    variant,
    fallbackTitle: trayTitleForState(state, dynamicIconEnabled),
    dynamicIconEnabled,
    renderKey: `${dynamicIconEnabled ? 'dynamic' : 'static'}:${variant}`,
  }
}

export function renderMacOSTrayIconDataURL(
  model: DesktopTrayIconModel,
): string {
  const overlay = renderOverlay(model.variant)
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 18 18">
<rect width="18" height="18" fill="none"/>
<path fill="black" d="M3 5.8c0-1.55 1.25-2.8 2.8-2.8h6.4C13.75 3 15 4.25 15 5.8v1.1h-2.2V5.8c0-.34-.26-.6-.6-.6H5.8c-.34 0-.6.26-.6.6v.75c0 .3.22.55.52.59l6.89.98A2.78 2.78 0 0 1 15 10.88v1.32c0 1.55-1.25 2.8-2.8 2.8H5.8A2.8 2.8 0 0 1 3 12.2v-1.1h2.2v1.1c0 .34.26.6.6.6h6.4c.34 0 .6-.26.6-.6v-1.03a.6.6 0 0 0-.52-.59l-6.89-.98A2.78 2.78 0 0 1 3 6.85V5.8Z"/>
${overlay}
</svg>`
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}

function iconVariant(
  state: DesktopTrayIconState | undefined,
): DesktopTrayIconVariant {
  switch (state) {
    case DesktopTrayIconState.ACTIVE:
      return 'active'
    case DesktopTrayIconState.ATTENTION:
      return 'attention'
    case DesktopTrayIconState.DISCONNECTED:
      return 'disconnected'
    case DesktopTrayIconState.QUITTING:
      return 'quitting'
    default:
      return 'healthy'
  }
}

function trayTitleForState(
  state: DesktopTrayIconState,
  dynamicIconEnabled: boolean,
): string {
  switch (state) {
    case DesktopTrayIconState.ACTIVE:
      return dynamicIconEnabled ? '' : '*'
    case DesktopTrayIconState.ATTENTION:
      return '!'
    case DesktopTrayIconState.DISCONNECTED:
      return 'x'
    case DesktopTrayIconState.QUITTING:
      return dynamicIconEnabled ? '...' : '...'
    default:
      return ''
  }
}

function renderOverlay(variant: DesktopTrayIconVariant): string {
  switch (variant) {
    case 'active':
      return '<circle cx="14.1" cy="4.1" r="2.1" fill="black"/>'
    case 'attention':
      return '<path fill="black" d="M13.8 2.2 17 7.8h-6.4l3.2-5.6Zm-.8 6.8h1.6v6H13V9Z"/>'
    case 'disconnected':
      return '<path fill="black" d="M2.25 3.65 3.65 2.25l12.1 12.1-1.4 1.4-12.1-12.1Z"/>'
    case 'quitting':
      return '<circle cx="6" cy="15" r="1.1" fill="black"/><circle cx="9" cy="15" r="1.1" fill="black"/><circle cx="12" cy="15" r="1.1" fill="black"/>'
    default:
      return ''
  }
}
