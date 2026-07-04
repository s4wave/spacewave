import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import {
  CommandFocusContext,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'

import {
  StateNamespaceProvider,
  atom,
  type Atom,
} from '@s4wave/web/state/index.js'
import { useLocalKeybindingOverrides } from './useLocalKeybindingOverrides.js'
import { localKeybindingStoreId } from './keybinding-overrides.js'

if (typeof document === 'undefined') {
  const happyDomWindow = new Window({ url: 'http://localhost/' })

  Object.defineProperties(globalThis, {
    window: { value: happyDomWindow, configurable: true },
    document: { value: happyDomWindow.document, configurable: true },
    HTMLElement: { value: happyDomWindow.HTMLElement, configurable: true },
    Element: { value: happyDomWindow.Element, configurable: true },
    Node: { value: happyDomWindow.Node, configurable: true },
    Text: { value: happyDomWindow.Text, configurable: true },
    DocumentFragment: {
      value: happyDomWindow.DocumentFragment,
      configurable: true,
    },
    SVGElement: { value: happyDomWindow.SVGElement, configurable: true },
    Event: { value: happyDomWindow.Event, configurable: true },
    CustomEvent: { value: happyDomWindow.CustomEvent, configurable: true },
    KeyboardEvent: { value: happyDomWindow.KeyboardEvent, configurable: true },
    MouseEvent: { value: happyDomWindow.MouseEvent, configurable: true },
    FocusEvent: { value: happyDomWindow.FocusEvent, configurable: true },
    InputEvent: { value: happyDomWindow.InputEvent, configurable: true },
    MutationObserver: {
      value: happyDomWindow.MutationObserver,
      configurable: true,
    },
  })
}

function comboBinding(id: string, combo: string): CommandBinding {
  return {
    id,
    binding: { case: 'combo', value: { combo } },
    when: CommandFocusContext.GLOBAL,
  }
}

function OverridesProbe({ name }: { name: string }) {
  const overrides = useLocalKeybindingOverrides()
  const commandIds = Object.keys(overrides.overrideSet.overrides).sort()
  const commandOverride = overrides.overrideSet.overrides['spacewave.palette']
  return (
    <section aria-label={name}>
      <div data-testid={`${name}-store-id`}>{localKeybindingStoreId}</div>
      <div data-testid={`${name}-layer`}>{overrides.layer.label}</div>
      <div data-testid={`${name}-commands`}>{commandIds.join(',')}</div>
      <div data-testid={`${name}-palette`}>
        {JSON.stringify(commandOverride ?? null)}
      </div>
      <button
        type="button"
        onClick={() =>
          overrides.addCommandBinding(
            'spacewave.palette',
            comboBinding('local-palette', 'Ctrl+K'),
          )
        }
      >
        {name} add
      </button>
      <button
        type="button"
        onClick={() =>
          overrides.setCommandBindings('spacewave.palette', [
            comboBinding('local-replace', 'Ctrl+P'),
          ])
        }
      >
        {name} replace
      </button>
      <button
        type="button"
        onClick={() =>
          overrides.clearCommandBindingId(
            'spacewave.palette',
            'default-palette',
          )
        }
      >
        {name} clear default
      </button>
      <button
        type="button"
        onClick={() => overrides.clearCommandBindings('spacewave.palette')}
      >
        {name} disable
      </button>
      <button
        type="button"
        onClick={() =>
          overrides.removeLocalCommandBinding(
            'spacewave.palette',
            'local-palette',
          )
        }
      >
        {name} remove local
      </button>
      <button
        type="button"
        onClick={() => overrides.resetCommand('spacewave.palette')}
      >
        {name} reset command
      </button>
      <button type="button" onClick={overrides.resetLayer}>
        {name} reset layer
      </button>
    </section>
  )
}

function renderWithStore(rootAtom: Atom<Record<string, unknown>>) {
  return render(
    <StateNamespaceProvider rootAtom={rootAtom}>
      <OverridesProbe name="writer" />
      <OverridesProbe name="reader" />
    </StateNamespaceProvider>,
  )
}

function click(element: Element): void {
  act(() => {
    fireEvent.click(element)
  })
}

describe('useLocalKeybindingOverrides', () => {
  afterEach(() => {
    cleanup()
  })

  it('persists under keybindings/local and publishes live override updates to subscribers', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    const view = renderWithStore(rootAtom)

    expect(view.getByTestId('writer-store-id').textContent).toBe(
      'keybindings/local',
    )
    expect(view.getByTestId('writer-layer').textContent).toBe('Local')
    expect(view.getByTestId('reader-palette').textContent).toBe('null')

    click(view.getByText('writer add'))

    expect(view.getByTestId('reader-commands').textContent).toBe(
      'spacewave.palette',
    )
    expect(view.getByTestId('reader-palette').textContent).toBe(
      JSON.stringify({
        bindings: [comboBinding('local-palette', 'Ctrl+K')],
      }),
    )
    expect(rootAtom.get()).toEqual({
      keybindings: {
        local: {
          version: 1,
          overrides: {
            'spacewave.palette': {
              bindings: [comboBinding('local-palette', 'Ctrl+K')],
            },
          },
          settings: {},
        },
      },
    })

    click(view.getByText('writer clear default'))

    expect(view.getByTestId('reader-palette').textContent).toBe(
      JSON.stringify({
        clearedBindingIds: ['default-palette'],
        bindings: [comboBinding('local-palette', 'Ctrl+K')],
      }),
    )

    click(view.getByText('writer remove local'))

    expect(view.getByTestId('reader-palette').textContent).toBe(
      JSON.stringify({ clearedBindingIds: ['default-palette'], bindings: [] }),
    )
  })

  it('exposes replacement, disabling, command reset, and local layer reset operations without reload', () => {
    const rootAtom = atom<Record<string, unknown>>({})

    const view = renderWithStore(rootAtom)

    click(view.getByText('writer replace'))
    expect(view.getByTestId('reader-palette').textContent).toBe(
      JSON.stringify({
        replaceBindings: true,
        bindings: [comboBinding('local-replace', 'Ctrl+P')],
      }),
    )

    click(view.getByText('writer disable'))
    expect(view.getByTestId('reader-palette').textContent).toBe(
      JSON.stringify({ replaceBindings: true, bindings: [] }),
    )

    click(view.getByText('writer reset command'))
    expect(view.getByTestId('reader-palette').textContent).toBe('null')

    click(view.getByText('writer add'))
    click(view.getByText('writer reset layer'))

    expect(view.getByTestId('reader-commands').textContent).toBe('')
    expect(rootAtom.get()).toEqual({})
  })
})
