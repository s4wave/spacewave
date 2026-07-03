import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { CommandBindingKind } from '@s4wave/sdk/command/command.pb.js'
import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'

import { useCommand } from './useCommand.js'

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

const mockRegisterCommand = vi.fn()
const mockSetActive = vi.fn()
const mockSetEnabled = vi.fn()
const mockReleaseResource = vi.fn()
const mockAttachResource = vi.fn()
const mockCleanup = vi.fn()
const mockContextValue = {
  service: {
    RegisterCommand: mockRegisterCommand,
    SetActive: mockSetActive,
    SetEnabled: mockSetEnabled,
  },
  releaseResource: mockReleaseResource,
  attachResource: mockAttachResource,
}
type AttachedHandlerService = {
  GetSubItems?: (
    req: { query?: string },
    signal?: AbortSignal,
  ) => Promise<{
    items?: Array<{ id?: string; label?: string; description?: string }>
  }>
  HandleCommand?: (req: {
    args?: Record<string, string>
  }) => Promise<Record<string, never>>
}
const attachedHandlerService: { current: AttachedHandlerService | null } = {
  current: null,
}

vi.mock('starpc', () => ({
  createHandler: (_definition: unknown, handler: unknown) => handler,
}))

vi.mock('@aptre/bldr-sdk/resource/server/index.js', () => ({
  newResourceMux: (handler: unknown) => ({ lookupMethod: handler }),
}))

vi.mock('./CommandContext.js', () => ({
  useCommandContext: () => mockContextValue,
}))

vi.mock('@s4wave/sdk/command/registry/registry_srpc.pb.js', () => ({
  CommandHandlerServiceDefinition: {},
}))

function TestCommand({
  active = true,
  enabled = true,
  handler = vi.fn(),
  keybinding,
  defaultBindings,
  subItems,
}: {
  active?: boolean
  enabled?: boolean
  handler?: (args: Record<string, string>) => void
  keybinding?: string
  defaultBindings?: CommandBinding[]
  subItems?: (
    query: string,
    signal: AbortSignal,
  ) => Promise<Array<{ id: string; label: string; description?: string }>>
}) {
  useCommand({
    commandId: 'spacewave.session.settings',
    label: 'Session Settings',
    active,
    enabled,
    handler,
    keybinding,
    defaultBindings,
    subItems,
    hasSubItems: !!subItems,
  })
  return null
}

describe('useCommand', () => {
  beforeEach(() => {
    attachedHandlerService.current = null
    mockAttachResource.mockImplementation(
      (_label: string, lookupMethod: AttachedHandlerService) => {
        attachedHandlerService.current = lookupMethod
        return Promise.resolve({
          resourceId: 11,
          cleanup: mockCleanup,
        })
      },
    )
    mockRegisterCommand.mockResolvedValue({ resourceId: 41 })
    mockSetActive.mockResolvedValue({})
    mockSetEnabled.mockResolvedValue({})
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('registers the command and updates active and enabled by registration id', async () => {
    const view = render(<TestCommand active={false} enabled={false} />)

    await waitFor(() => {
      expect(mockAttachResource).toHaveBeenCalled()
      expect(mockRegisterCommand).toHaveBeenCalled()
      expect(mockSetActive).toHaveBeenCalledWith(
        { resourceId: 41, active: false },
        expect.any(AbortSignal),
      )
      expect(mockSetEnabled).toHaveBeenCalledWith(
        { resourceId: 41, enabled: false },
        expect.any(AbortSignal),
      )
    })

    expect(mockRegisterCommand.mock.lastCall).toEqual([
      {
        command: {
          commandId: 'spacewave.session.settings',
          label: 'Session Settings',
          keybinding: undefined,
          defaultBindings: undefined,
          menuPath: undefined,
          menuGroup: undefined,
          menuOrder: undefined,
          icon: undefined,
          description: undefined,
          hasSubItems: false,
        },
        handlerResourceId: 11,
      },
      expect.any(AbortSignal),
    ])

    view.unmount()

    expect(mockCleanup).toHaveBeenCalled()
    expect(mockReleaseResource).toHaveBeenCalledWith(41)
  })

  it('registers legacy keybinding without typed default bindings', async () => {
    render(<TestCommand keybinding="CmdOrCtrl+," />)

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalled()
    })

    expect(mockRegisterCommand.mock.lastCall).toEqual([
      {
        command: {
          commandId: 'spacewave.session.settings',
          label: 'Session Settings',
          keybinding: 'CmdOrCtrl+,',
          defaultBindings: undefined,
          menuPath: undefined,
          menuGroup: undefined,
          menuOrder: undefined,
          icon: undefined,
          description: undefined,
          hasSubItems: false,
        },
        handlerResourceId: 11,
      },
      expect.any(AbortSignal),
    ])
  })

  it('registers plural typed default bindings', async () => {
    const defaultBindings: CommandBinding[] = [
      {
        id: 'settings.combo',
        kind: CommandBindingKind.COMBO,
        combo: { combo: 'CmdOrCtrl+Shift+,' },
        sourceLabel: 'Spacewave',
      },
      {
        id: 'settings.sequence',
        kind: CommandBindingKind.SEQUENCE,
        sequence: { steps: ['Leader', ','] },
        sourceLabel: 'Spacewave',
      },
    ]

    render(<TestCommand defaultBindings={defaultBindings} />)

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalled()
    })

    expect(mockRegisterCommand.mock.lastCall).toEqual([
      {
        command: {
          commandId: 'spacewave.session.settings',
          label: 'Session Settings',
          keybinding: undefined,
          defaultBindings,
          menuPath: undefined,
          menuGroup: undefined,
          menuOrder: undefined,
          icon: undefined,
          description: undefined,
          hasSubItems: false,
        },
        handlerResourceId: 11,
      },
      expect.any(AbortSignal),
    ])
  })

  it('preserves legacy keybinding and typed default bindings together', async () => {
    const defaultBindings: CommandBinding[] = [
      {
        id: 'settings.combo',
        kind: CommandBindingKind.COMBO,
        combo: { combo: 'CmdOrCtrl+Alt+,' },
      },
    ]

    render(
      <TestCommand
        keybinding="CmdOrCtrl+,"
        defaultBindings={defaultBindings}
      />,
    )

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalled()
    })

    expect(mockRegisterCommand.mock.lastCall).toEqual([
      {
        command: {
          commandId: 'spacewave.session.settings',
          label: 'Session Settings',
          keybinding: 'CmdOrCtrl+,',
          defaultBindings,
          menuPath: undefined,
          menuGroup: undefined,
          menuOrder: undefined,
          icon: undefined,
          description: undefined,
          hasSubItems: false,
        },
        handlerResourceId: 11,
      },
      expect.any(AbortSignal),
    ])
  })

  it('uses the latest handler and sub-items callbacks without re-registering', async () => {
    const firstHandler = vi.fn()
    const secondHandler = vi.fn()
    const firstSubItems = vi
      .fn()
      .mockResolvedValue([{ id: 'first', label: 'First' }])
    const secondSubItems = vi
      .fn()
      .mockResolvedValue([{ id: 'second', label: 'Second' }])

    const view = render(
      <TestCommand handler={firstHandler} subItems={firstSubItems} />,
    )

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalled()
      expect(attachedHandlerService.current).not.toBeNull()
    })

    const registerCount = mockRegisterCommand.mock.calls.length
    view.rerender(
      <TestCommand handler={secondHandler} subItems={secondSubItems} />,
    )

    await attachedHandlerService.current?.HandleCommand?.({
      args: { target: 'updated' },
    })
    expect(secondHandler).toHaveBeenCalledWith({ target: 'updated' })

    const subItemsResp = await attachedHandlerService.current?.GetSubItems?.({
      query: 'next',
    })
    expect(subItemsResp?.items).toEqual([{ id: 'second', label: 'Second' }])
    expect(firstHandler).not.toHaveBeenCalled()
    expect(firstSubItems).not.toHaveBeenCalled()
    expect(secondSubItems).toHaveBeenCalledWith('next', expect.any(AbortSignal))
    expect(mockRegisterCommand.mock.calls.length).toBe(registerCount)
  })
})
