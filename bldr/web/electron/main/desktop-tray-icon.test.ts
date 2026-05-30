import { describe, expect, it } from 'vitest'

import {
  DesktopTrayIconState,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import {
  buildDesktopTrayIconModel,
  renderMacOSTrayIconDataURL,
} from './desktop-tray-icon.js'

describe('desktop tray icon model', () => {
  it.each([
    [DesktopTrayIconState.NORMAL, 'healthy', ''],
    [DesktopTrayIconState.ACTIVE, 'active', '*'],
    [DesktopTrayIconState.ATTENTION, 'attention', '!'],
    [DesktopTrayIconState.DISCONNECTED, 'disconnected', 'x'],
    [DesktopTrayIconState.QUITTING, 'quitting', '...'],
  ] as const)(
    'keeps static fallback title semantics for %s',
    (iconState, variant, title) => {
      const model = buildDesktopTrayIconModel({
        appName: 'Spacewave',
        state: state(iconState, 'Running'),
      })

      expect(model.variant).toBe(variant)
      expect(model.fallbackTitle).toBe(title)
      expect(model.tooltip).toBe('Spacewave: Running')
      expect(model.dynamicIconEnabled).toBe(false)
    },
  )

  it('suppresses active debug title when dynamic macOS icons are enabled', () => {
    const model = buildDesktopTrayIconModel({
      state: state(DesktopTrayIconState.ACTIVE, 'Syncing'),
      dynamicIconEnabled: true,
    })

    expect(model.variant).toBe('active')
    expect(model.fallbackTitle).toBe('')
    expect(model.tooltip).toBe('Spacewave: Syncing')
  })

  it('keeps long update-ready status text out of icon render identity', () => {
    const statusText =
      'Update ready for Project Alpha With An Extremely Long Name That Must Not Resize The Menu Bar'
    const model = buildDesktopTrayIconModel({
      state: state(DesktopTrayIconState.ATTENTION, statusText),
      dynamicIconEnabled: true,
    })

    expect(model.statusText).toBe(statusText)
    expect(model.tooltip).toBe(`Spacewave: ${statusText}`)
    expect(model.fallbackTitle).toBe('!')
    expect(model.renderKey).toBe('dynamic:attention')
    expect(decodeURIComponent(renderMacOSTrayIconDataURL(model))).not.toContain(
      statusText,
    )
  })

  it('renders deterministic template SVG data urls for macOS variants', () => {
    const active = buildDesktopTrayIconModel({
      state: state(DesktopTrayIconState.ACTIVE, 'Syncing'),
      dynamicIconEnabled: true,
    })
    const disconnected = buildDesktopTrayIconModel({
      state: state(DesktopTrayIconState.DISCONNECTED, 'Disconnected'),
      dynamicIconEnabled: true,
    })

    expect(renderMacOSTrayIconDataURL(active)).toContain('data:image/svg+xml')
    expect(renderMacOSTrayIconDataURL(active)).not.toBe(
      renderMacOSTrayIconDataURL(disconnected),
    )
    expect(
      decodeURIComponent(renderMacOSTrayIconDataURL(disconnected)),
    ).toContain('12.1-12.1')
  })
})

function state(
  iconState: DesktopTrayIconState,
  statusText: string,
): DesktopTrayState {
  return { iconState, statusText, entries: [] }
}
