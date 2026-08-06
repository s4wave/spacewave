import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import {
  CommandSurface,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'

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

vi.mock('starpc', async (importOriginal) => ({
  ...(await importOriginal<typeof import('starpc')>()),
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
  searchAliases,
}: {
  active?: boolean
  enabled?: boolean
  handler?: (args: Record<string, string>) => void
  keybinding?: string
  defaultBindings?: CommandBinding[]
  searchAliases?: string[]
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
    searchAliases,
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
          searchAliases: undefined,
        },
        handlerResourceId: 11,
        surface: CommandSurface.WEB,
      },
      expect.any(AbortSignal),
    ])

    view.unmount()

    expect(mockCleanup).toHaveBeenCalled()
    expect(mockReleaseResource).toHaveBeenCalledWith(41)
  })

  it('registers search aliases on the web surface', async () => {
    render(<TestCommand searchAliases={['open', 'find']} />)

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalled()
    })

    expect(mockRegisterCommand.mock.lastCall?.[0]).toMatchObject({
      command: {
        searchAliases: ['open', 'find'],
      },
      surface: CommandSurface.WEB,
    })
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
          searchAliases: undefined,
        },
        handlerResourceId: 11,
        surface: CommandSurface.WEB,
      },
      expect.any(AbortSignal),
    ])
  })

  it('registers plural typed default bindings', async () => {
    const defaultBindings: CommandBinding[] = [
      {
        id: 'settings.combo',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+Shift+,' } },
        sourceLabel: 'Spacewave',
      },
      {
        id: 'settings.sequence',
        binding: { case: 'sequence', value: { steps: ['Leader', ','] } },
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
          searchAliases: undefined,
        },
        handlerResourceId: 11,
        surface: CommandSurface.WEB,
      },
      expect.any(AbortSignal),
    ])
  })

  it('preserves legacy keybinding and typed default bindings together', async () => {
    const defaultBindings: CommandBinding[] = [
      {
        id: 'settings.combo',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+Alt+,' } },
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
          searchAliases: undefined,
        },
        handlerResourceId: 11,
        surface: CommandSurface.WEB,
      },
      expect.any(AbortSignal),
    ])
  })

  it('does not churn registration when semantically stable options get new identities during unrelated rerenders', async () => {
    const handled = vi.fn()
    let registerAttempt = 0
    mockRegisterCommand.mockImplementation(() => {
      registerAttempt += 1
      if (registerAttempt > 3) {
        return Promise.reject(new Error('registration churn guard tripped'))
      }
      return Promise.resolve({ resourceId: 100 + registerAttempt })
    })

    const stableDefaultBindings: CommandBinding[] = [
      {
        id: 'settings.combo',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+,' } },
        sourceLabel: 'Spacewave',
      },
    ]
    const makeDefaultBindings = vi.fn(() =>
      stableDefaultBindings.map((binding) => ({ ...binding })),
    )
    function CommandOwner() {
      const [version, setVersion] = React.useState(0)
      const defaultBindings = makeDefaultBindings()

      return (
        <>
          <TestCommand
            defaultBindings={defaultBindings}
            handler={(args) => handled({ args, version })}
          />
          <button
            type="button"
            onClick={() => setVersion((value) => value + 1)}
          >
            unrelated rerender {version}
          </button>
        </>
      )
    }

    const view = render(<CommandOwner />)

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalledTimes(1)
      expect(attachedHandlerService.current).not.toBeNull()
    })
    await waitFor(() => {
      expect(mockSetActive).toHaveBeenCalledWith(
        { resourceId: 101, active: true },
        expect.any(AbortSignal),
      )
    })

    fireEvent.click(view.getByRole('button', { name: /unrelated rerender 0/ }))
    expect(view.getByRole('button').textContent).toBe('unrelated rerender 1')
    view.rerender(<CommandOwner />)
    await waitFor(
      () => {
        expect(mockRegisterCommand).toHaveBeenCalledTimes(2)
      },
      { timeout: 500 },
    ).catch(() => undefined)

    await attachedHandlerService.current?.HandleCommand?.({
      args: { source: 'rerender' },
    })

    expect(handled).toHaveBeenCalledWith({
      args: { source: 'rerender' },
      version: 1,
    })
    expect(mockRegisterCommand).toHaveBeenCalledTimes(1)
    expect(mockReleaseResource).not.toHaveBeenCalled()
    expect(mockAttachResource).toHaveBeenCalledTimes(1)
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
  it('does not re-register when inline default bindings keep the same command shape', async () => {
    function InlineDefaultBindingsCommand() {
      useCommand({
        commandId: 'spacewave.view.palette',
        label: 'Command Palette',
        defaultBindings: [
          {
            id: 'global-palette',
            binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
          },
        ],
        handler: vi.fn(),
      })
      return null
    }

    const view = render(<InlineDefaultBindingsCommand />)

    await waitFor(() => {
      expect(mockRegisterCommand).toHaveBeenCalled()
    })

    const registerCount = mockRegisterCommand.mock.calls.length
    view.rerender(<InlineDefaultBindingsCommand />)

    await Promise.resolve()

    expect(mockRegisterCommand.mock.calls.length).toBe(registerCount)
    expect(mockCleanup).not.toHaveBeenCalled()
    expect(mockReleaseResource).not.toHaveBeenCalled()
  })
})
