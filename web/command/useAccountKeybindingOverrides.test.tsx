import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'
import {
  CommandFocusContext,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'

import { useAccountKeybindingOverrides } from './useAccountKeybindingOverrides.js'

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
  sessionResource: {
    value: null as unknown,
    loading: false,
    error: null as Error | null,
  },
  sessionInfo: { providerId: '', accountId: '' },
  accountResource: {
    value: null as unknown,
    loading: false,
    error: null as Error | null,
  },
  accountOverrides: {
    value: null as unknown,
    loading: false,
    error: null as Error | null,
  },
  useSessionInfo: vi.fn(),
  useMountAccount: vi.fn(),
  useStreamingResource: vi.fn(),
}

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => hookState.sessionResource,
  },
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => hookState.useSessionInfo(),
}))

vi.mock('@s4wave/web/hooks/useMountAccount.js', () => ({
  useMountAccount: (providerId: string, accountId: string, enabled: boolean) =>
    hookState.useMountAccount(providerId, accountId, enabled),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => hookState.useStreamingResource(),
}))

function comboBinding(id: string, combo: string): CommandBinding {
  return {
    id,
    binding: { case: 'combo', value: { combo } },
    when: CommandFocusContext.GLOBAL,
  }
}

function AccountOverridesProbe() {
  const overrides = useAccountKeybindingOverrides()
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
            comboBinding('palette-account', 'Ctrl+K'),
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
      <button
        type="button"
        onClick={() =>
          overrides.setSettings({
            leaderCombo: 'Alt+Space',
            whichKeyDelayMs: 125,
          })
        }
      >
        set discovery settings
      </button>
    </section>
  )
}

describe('useAccountKeybindingOverrides', () => {
  beforeEach(() => {
    hookState.sessionResource = { value: null, loading: false, error: null }
    hookState.sessionInfo = { providerId: '', accountId: '' }
    hookState.accountResource = { value: null, loading: false, error: null }
    hookState.accountOverrides = { value: null, loading: false, error: null }
    hookState.useSessionInfo.mockImplementation(() => hookState.sessionInfo)
    hookState.useMountAccount.mockImplementation(
      () => hookState.accountResource,
    )
    hookState.useStreamingResource.mockImplementation(
      () => hookState.accountOverrides,
    )
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('exposes an unavailable read-only account layer and refuses writes when no account can be mounted', () => {
    const upsertKeybindingOverride = vi.fn()
    const removeKeybindingOverride = vi.fn()

    hookState.accountResource = {
      value: null,
      loading: false,
      error: null,
    }
    hookState.accountOverrides = {
      value: {
        readOnly: false,
        overrideSet: {
          version: 1,
          overrides: [
            {
              commandId: 'spacewave.palette',
              bindings: [comboBinding('palette-account', 'Ctrl+K')],
            },
          ],
        },
      },
      loading: false,
      error: null,
    }

    const view = render(<AccountOverridesProbe />)

    expect(view.getByTestId('available').textContent).toBe('false')
    expect(view.getByTestId('read-only').textContent).toBe('true')
    expect(view.getByTestId('layer-scope').textContent).toBe('null')
    expect(view.getByTestId('commands').textContent).toBe('spacewave.palette')

    fireEvent.click(view.getByText('replace palette'))
    fireEvent.click(view.getByText('reset layer'))

    expect(upsertKeybindingOverride).not.toHaveBeenCalled()
    expect(removeKeybindingOverride).not.toHaveBeenCalled()
  })

  it('publishes an account layer and writes replace, clear, reset, and layer reset through account settings ops', () => {
    const account = {
      watchKeybindingOverrides: vi.fn(),
      upsertKeybindingOverride: vi.fn(),
      removeKeybindingOverride: vi.fn(),
      setKeybindingSettings: vi.fn(),
    }
    hookState.sessionResource = {
      value: { id: 'session-1' },
      loading: false,
      error: null,
    }
    hookState.sessionInfo = { providerId: 'local', accountId: 'account-1' }
    hookState.accountResource = { value: account, loading: false, error: null }
    hookState.accountOverrides = {
      value: {
        readOnly: false,
        overrideSet: {
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
      loading: false,
      error: null,
    }

    const view = render(<AccountOverridesProbe />)

    expect(hookState.useMountAccount).toHaveBeenCalledWith(
      'local',
      'account-1',
      true,
    )
    expect(view.getByTestId('available').textContent).toBe('true')
    expect(view.getByTestId('read-only').textContent).toBe('false')
    expect(view.getByTestId('layer-scope').textContent).toBe('account')
    expect(view.getByTestId('layer-label').textContent).toBe('Account')
    expect(view.getByTestId('commands').textContent).toBe(
      'spacewave.palette,spacewave.viewer',
    )

    fireEvent.click(view.getByText('replace palette'))
    expect(account.upsertKeybindingOverride).toHaveBeenLastCalledWith({
      override: {
        commandId: 'spacewave.palette',
        replaceBindings: true,
        disabled: undefined,
        clearedBindingIds: [],
        bindings: [comboBinding('palette-account', 'Ctrl+K')],
      },
    })

    fireEvent.click(view.getByText('clear default'))
    expect(account.upsertKeybindingOverride).toHaveBeenLastCalledWith({
      override: expect.objectContaining({
        commandId: 'spacewave.palette',
        clearedBindingIds: ['palette-default'],
        bindings: [comboBinding('palette-existing', 'Ctrl+P')],
      }),
    })

    fireEvent.click(view.getByText('reset palette'))
    expect(account.removeKeybindingOverride).toHaveBeenCalledWith({
      commandId: 'spacewave.palette',
    })

    fireEvent.click(view.getByText('reset layer'))
    expect(account.removeKeybindingOverride).toHaveBeenCalledWith({
      commandId: 'spacewave.viewer',
    })

    fireEvent.click(view.getByText('set discovery settings'))
    expect(account.setKeybindingSettings).toHaveBeenLastCalledWith({
      settings: {
        leaderCombo: 'Alt+Space',
        whichKeyDelayMs: 125,
        display: undefined,
      },
    })
  })

  it('keeps a mounted account read-only when account settings reports read-only state', () => {
    const account = {
      watchKeybindingOverrides: vi.fn(),
      upsertKeybindingOverride: vi.fn(),
      removeKeybindingOverride: vi.fn(),
      setKeybindingSettings: vi.fn(),
    }
    hookState.accountResource = { value: account, loading: false, error: null }
    hookState.accountOverrides = {
      value: {
        readOnly: true,
        overrideSet: {
          version: 1,
          overrides: [
            {
              commandId: 'spacewave.palette',
              bindings: [comboBinding('palette-account', 'Ctrl+K')],
            },
          ],
        },
      },
      loading: false,
      error: null,
    }

    const view = render(<AccountOverridesProbe />)

    expect(view.getByTestId('available').textContent).toBe('true')
    expect(view.getByTestId('read-only').textContent).toBe('true')

    fireEvent.click(view.getByText('replace palette'))
    fireEvent.click(view.getByText('reset layer'))

    expect(account.upsertKeybindingOverride).not.toHaveBeenCalled()
    expect(account.removeKeybindingOverride).not.toHaveBeenCalled()
    expect(account.setKeybindingSettings).not.toHaveBeenCalled()
  })
})
