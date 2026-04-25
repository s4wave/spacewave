import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, waitFor } from '@testing-library/react'

import { useCommand } from './useCommand.js'

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
  subItems,
}: {
  active?: boolean
  enabled?: boolean
  handler?: (args: Record<string, string>) => void
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
