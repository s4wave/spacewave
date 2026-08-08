import { Window } from 'happy-dom'
import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import {
  CommandFocusContext,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'

import { useSpaceKeybindingOverrides } from './useSpaceKeybindingOverrides.js'

if (typeof document === 'undefined') {
  const happyDomWindow = new Window({ url: 'http://localhost/' })

  Object.defineProperties(globalThis, {
    window: { value: happyDomWindow, configurable: true },
    document: { value: happyDomWindow.document, configurable: true },
    HTMLElement: { value: happyDomWindow.HTMLElement, configurable: true },
    HTMLButtonElement: {
      value: happyDomWindow.HTMLButtonElement,
      configurable: true,
    },
    HTMLInputElement: {
      value: happyDomWindow.HTMLInputElement,
      configurable: true,
    },
    Event: { value: happyDomWindow.Event, configurable: true },
    KeyboardEvent: { value: happyDomWindow.KeyboardEvent, configurable: true },
    MouseEvent: { value: happyDomWindow.MouseEvent, configurable: true },
    CustomEvent: { value: happyDomWindow.CustomEvent, configurable: true },
    navigator: { value: happyDomWindow.navigator, configurable: true },
  })
}

const hookState = {
  spaceContext: null as unknown,
}

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => hookState.spaceContext,
  },
}))

function comboBinding(id: string, combo: string): CommandBinding {
  return {
    id,
    binding: { case: 'combo', value: { combo } },
    when: CommandFocusContext.GLOBAL,
    surface: CommandSurface.WEB,
  }
}

function lastSettingsOp(applyWorldOp: { mock: { calls: unknown[][] } }) {
  const opData = applyWorldOp.mock.calls.at(-1)?.[1]
  if (!(opData instanceof Uint8Array)) throw new Error('missing op data')
  return SetSpaceSettingsOp.fromBinary(opData)
}

function SpaceOverridesProbe() {
  const overrides = useSpaceKeybindingOverrides(
    CommandSurface.WEB,
    new Set(['spacewave.palette', 'spacewave.viewer']),
  )
  const commandIds = Object.keys(overrides.overrideSet.overrides).sort()
  return (
    <section>
      <div data-testid="available">
        {overrides.available ? 'true' : 'false'}
      </div>
      <div data-testid="read-only">{overrides.readOnly ? 'true' : 'false'}</div>
      <div data-testid="layer-scope">{overrides.layer?.scope ?? 'null'}</div>
      <div data-testid="layer-label">{overrides.layer?.label ?? 'null'}</div>
      <div data-testid="commands">{commandIds.join(',')}</div>
      <button
        type="button"
        onClick={() =>
          overrides.setCommandBindings('spacewave.palette', [
            comboBinding('palette-space-replace', 'Ctrl+K'),
          ])
        }
      >
        replace palette
      </button>
      <button
        type="button"
        onClick={() =>
          overrides.clearCommandBindingId(
            'spacewave.palette',
            'palette-default',
          )
        }
      >
        clear default
      </button>
      <button
        type="button"
        onClick={() => overrides.resetCommand('spacewave.palette')}
      >
        reset palette
      </button>
      <button type="button" onClick={() => overrides.resetLayer()}>
        reset layer
      </button>
    </section>
  )
}

describe('useSpaceKeybindingOverrides', () => {
  beforeEach(() => {
    hookState.spaceContext = null
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('exposes an unavailable read-only Space layer and refuses writes when there is no Space context', () => {
    const view = render(<SpaceOverridesProbe />)

    expect(view.getByTestId('available').textContent).toBe('false')
    expect(view.getByTestId('read-only').textContent).toBe('true')
    expect(view.getByTestId('layer-scope').textContent).toBe('null')
    expect(view.getByTestId('commands').textContent).toBe('')

    fireEvent.click(view.getByText('replace palette'))
    fireEvent.click(view.getByText('reset layer'))

    expect(view.getByTestId('commands').textContent).toBe('')
  })

  it('publishes a Space layer and writes keybinding overrides without dropping index_path or plugin_ids', async () => {
    const applyWorldOp = vi.fn(() => Promise.resolve())
    hookState.spaceContext = {
      spaceState: {
        settings: {
          indexPath: '/files',
          pluginIds: ['spacewave-app', 'spacewave-terminal'],
          keybindingOverrides: {
            version: 1,
            overrides: [
              {
                commandId: 'spacewave.palette',
                bindings: [comboBinding('palette-existing', 'Ctrl+P')],
              },
              {
                commandId: 'spacewave.viewer',
                bindings: [comboBinding('viewer-existing', 'Ctrl+V')],
              },
            ],
          },
        },
      },
      spaceWorld: { applyWorldOp },
      spaceWorldResource: {
        value: { applyWorldOp },
        loading: false,
        error: null,
      },
    }

    const view = render(<SpaceOverridesProbe />)

    expect(view.getByTestId('available').textContent).toBe('true')
    expect(view.getByTestId('read-only').textContent).toBe('false')
    expect(view.getByTestId('layer-scope').textContent).toBe('space')
    expect(view.getByTestId('layer-label').textContent).toBe('Space')
    expect(view.getByTestId('commands').textContent).toBe(
      'spacewave.palette,spacewave.viewer',
    )

    await waitFor(() => expect(applyWorldOp).toHaveBeenCalledTimes(1))
    applyWorldOp.mockClear()
    fireEvent.click(view.getByText('replace palette'))

    await waitFor(() => expect(applyWorldOp).toHaveBeenCalledTimes(1))
    expect(applyWorldOp).toHaveBeenLastCalledWith(
      SET_SPACE_SETTINGS_OP_ID,
      expect.any(Uint8Array),
      '',
    )
    let op = lastSettingsOp(applyWorldOp)
    expect(op.settings?.indexPath).toBe('/files')
    expect(op.settings?.pluginIds).toEqual([
      'spacewave-app',
      'spacewave-terminal',
    ])
    expect(op.settings?.keybindingOverrides?.webOverrides).toEqual([
      expect.objectContaining({
        commandId: 'spacewave.palette',
        replaceBindings: true,
        bindings: [comboBinding('palette-space-replace', 'Ctrl+K')],
      }),
      expect.objectContaining({
        commandId: 'spacewave.viewer',
        bindings: [comboBinding('viewer-existing', 'Ctrl+V')],
      }),
    ])

    fireEvent.click(view.getByText('clear default'))

    await waitFor(() => expect(applyWorldOp).toHaveBeenCalledTimes(2))
    op = lastSettingsOp(applyWorldOp)
    expect(op.settings?.indexPath).toBe('/files')
    expect(op.settings?.pluginIds).toEqual([
      'spacewave-app',
      'spacewave-terminal',
    ])
    expect(op.settings?.keybindingOverrides?.webOverrides).toEqual([
      expect.objectContaining({
        commandId: 'spacewave.palette',
        clearedBindingIds: ['palette-default'],
      }),
      expect.objectContaining({ commandId: 'spacewave.viewer' }),
    ])

    fireEvent.click(view.getByText('reset palette'))

    await waitFor(() => expect(applyWorldOp).toHaveBeenCalledTimes(3))
    op = lastSettingsOp(applyWorldOp)
    expect(op.settings?.keybindingOverrides?.webOverrides).toEqual([
      expect.objectContaining({ commandId: 'spacewave.viewer' }),
    ])

    fireEvent.click(view.getByText('reset layer'))

    await waitFor(() => expect(applyWorldOp).toHaveBeenCalledTimes(4))
    op = lastSettingsOp(applyWorldOp)
    expect(op.settings?.indexPath).toBe('/files')
    expect(op.settings?.pluginIds).toEqual([
      'spacewave-app',
      'spacewave-terminal',
    ])
    expect(op.settings?.keybindingOverrides).toEqual(
      expect.objectContaining({
        version: 2,
        webSettings: {},
      }),
    )
    expect(op.settings?.keybindingOverrides?.webOverrides ?? []).toEqual([])
  })

  it('keeps a Space layer read-only when sharing state does not allow management', () => {
    const applyWorldOp = vi.fn(() => Promise.resolve())
    hookState.spaceContext = {
      spaceState: {
        settings: {
          indexPath: '/files',
          pluginIds: ['spacewave-app'],
          keybindingOverrides: {
            version: 1,
            overrides: [
              {
                commandId: 'spacewave.palette',
                bindings: [comboBinding('palette-existing', 'Ctrl+P')],
              },
            ],
          },
        },
      },
      spaceWorld: { applyWorldOp },
      spaceWorldResource: {
        value: { applyWorldOp },
        loading: false,
        error: null,
      },
      spaceSharingState: { canManage: false },
    }

    const view = render(<SpaceOverridesProbe />)

    expect(view.getByTestId('available').textContent).toBe('true')
    expect(view.getByTestId('read-only').textContent).toBe('true')
    expect(view.getByTestId('commands').textContent).toBe('spacewave.palette')

    fireEvent.click(view.getByText('replace palette'))
    expect(applyWorldOp).not.toHaveBeenCalled()
  })
})
