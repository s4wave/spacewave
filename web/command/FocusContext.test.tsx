import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render } from '@testing-library/react'
import { CommandFocusContext } from '@s4wave/sdk/command/command.pb.js'

import { TabActiveProvider } from '@s4wave/web/contexts/TabActiveContext.js'

import {
  FocusContextProvider,
  ShellTabFocusContextProvider,
  focusContextDomProps,
  resolveFocusContextsForTarget,
  useFocusContextStack,
} from './FocusContext.js'

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

function StackObserver() {
  const stack = useFocusContextStack()
  return <output data-testid="stack">{stack.join(',')}</output>
}

describe('FocusContext', () => {
  afterEach(() => {
    cleanup()
  })

  it('adds the active shell tab context from the tab-active owner', () => {
    const activeView = render(
      <TabActiveProvider value={true}>
        <ShellTabFocusContextProvider>
          <StackObserver />
        </ShellTabFocusContextProvider>
      </TabActiveProvider>,
    )

    expect(activeView.getByTestId('stack').textContent).toBe(
      [CommandFocusContext.GLOBAL, CommandFocusContext.SHELL_TAB].join(','),
    )
    activeView.unmount()

    const inactiveView = render(
      <TabActiveProvider value={false}>
        <ShellTabFocusContextProvider>
          <StackObserver />
        </ShellTabFocusContextProvider>
      </TabActiveProvider>,
    )

    expect(inactiveView.getByTestId('stack').textContent).toBe(
      String(CommandFocusContext.GLOBAL),
    )
  })

  it('resolves typed DOM focus contexts from the focused target ancestry', () => {
    const view = render(
      <FocusContextProvider focusContext={CommandFocusContext.MODAL}>
        <FocusContextProvider focusContext={CommandFocusContext.LIST}>
          <button type="button">Row action</button>
        </FocusContextProvider>
      </FocusContextProvider>,
    )

    expect(resolveFocusContextsForTarget(view.getByRole('button'))).toEqual([
      CommandFocusContext.GLOBAL,
      CommandFocusContext.MODAL,
      CommandFocusContext.LIST,
    ])
  })

  it('uses text-input context for editable controls outside an editor', () => {
    const input = document.createElement('input')

    expect(resolveFocusContextsForTarget(input)).toEqual([
      CommandFocusContext.GLOBAL,
      CommandFocusContext.TEXT_INPUT,
    ])
  })

  it('keeps Lexical contenteditable targets in the editor context', () => {
    const editor = document.createElement('div')
    setFocusContextAttribute(editor, CommandFocusContext.EDITOR)
    const editable = document.createElement('div')
    editable.contentEditable = 'true'
    editor.append(editable)

    expect(resolveFocusContextsForTarget(editable)).toEqual([
      CommandFocusContext.GLOBAL,
      CommandFocusContext.EDITOR,
    ])
  })

  it('resolves every typed command focus context from DOM providers', () => {
    const contexts = [
      CommandFocusContext.GLOBAL,
      CommandFocusContext.SHELL_TAB,
      CommandFocusContext.EDITOR,
      CommandFocusContext.LIST,
      CommandFocusContext.CANVAS,
      CommandFocusContext.MODAL,
      CommandFocusContext.TEXT_INPUT,
    ]
    const target = document.createElement('button')
    let parent: HTMLElement = target
    for (const context of [...contexts].reverse()) {
      if (context === CommandFocusContext.GLOBAL) continue
      const element = document.createElement('div')
      setFocusContextAttribute(element, context)
      element.append(parent)
      parent = element
    }

    expect(resolveFocusContextsForTarget(target)).toEqual(contexts)
  })
})

function setFocusContextAttribute(
  element: HTMLElement,
  focusContext: CommandFocusContext,
): void {
  for (const [name, value] of Object.entries(
    focusContextDomProps(focusContext),
  )) {
    element.setAttribute(name, value)
  }
}
