import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

interface MockRouteParams {
  quickstartId: string
  orgId?: string
}

const mockParams = vi.hoisted(() =>
  vi.fn<() => MockRouteParams>(() => ({
    quickstartId: 'drive',
  })),
)
const mockUseSessionContext = vi.hoisted(() => vi.fn())
const mockUseSessionNavigate = vi.hoisted(() => vi.fn())
const mockUseResourceValue = vi.hoisted(() => vi.fn())
const mockToastError = vi.hoisted(() => vi.fn())
const mockCreateSetup = vi.hoisted(() => vi.fn())
const mockPopulateSpace = vi.hoisted(() => vi.fn())
const mockExecuteDynamicQuickstart = vi.hoisted(() => vi.fn())
const mockSessionCreateSpace = vi.hoisted(() => vi.fn())
const mockSessionDeleteSpace = vi.hoisted(() => vi.fn())
const mockUseRootResource = vi.hoisted(() => vi.fn())
const mockUseVisibleQuickstartOptions = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useParams: mockParams,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: mockToastError },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: mockUseSessionContext },
  useSessionNavigate: () => mockUseSessionNavigate,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('./useQuickstartOptions.js', () => ({
  useVisibleQuickstartOptions: mockUseVisibleQuickstartOptions,
}))

vi.mock('@s4wave/app/session/setup/SetupPageLayout.js', () => ({
  SetupPageLayout: ({
    title,
    children,
  }: {
    title: string
    children: React.ReactNode
  }) => (
    <div data-testid="setup-layout">
      <h1>{title}</h1>
      {children}
    </div>
  ),
}))

vi.mock('./create.js', async () => {
  const actual =
    await vi.importActual<typeof import('./create.js')>('./create.js')
  return {
    ...actual,
    createQuickstartSetupFromSession: mockCreateSetup,
    executeDynamicQuickstart: mockExecuteDynamicQuickstart,
    populateSpace: mockPopulateSpace,
  }
})

import { CreateSpaceRoute } from './CreateSpaceRoute.js'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function makeSession() {
  return {
    createSpace: mockSessionCreateSpace,
    deleteSpace: mockSessionDeleteSpace,
  }
}

function setParams(params: { quickstartId: string; orgId?: string }) {
  mockParams.mockReturnValue(params)
}

describe('CreateSpaceRoute', () => {
  beforeEach(() => {
    mockParams.mockReturnValue({ quickstartId: 'drive', orgId: undefined })
    mockToastError.mockReset()
    mockUseSessionContext.mockReset()
    mockUseSessionNavigate.mockReset()
    mockUseResourceValue.mockReset()
    mockCreateSetup.mockReset()
    mockPopulateSpace.mockReset()
    mockExecuteDynamicQuickstart.mockReset()
    mockSessionCreateSpace.mockReset()
    mockSessionDeleteSpace.mockReset()
    mockUseRootResource.mockReset()
    mockUseVisibleQuickstartOptions.mockReset()

    const session = makeSession()
    mockUseSessionContext.mockReturnValue({ value: session })
    mockUseResourceValue.mockImplementation(
      (resource: { value?: unknown } | null | undefined) => resource?.value,
    )
    mockUseRootResource.mockReturnValue({ value: { client: {} } })
    mockUseVisibleQuickstartOptions.mockReturnValue([
      {
        id: 'drive',
        name: 'Create a Drive',
        description: 'Drive workspace',
        category: 'storage',
        icon: () => null,
      },
    ])
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('runs Create -> Mount -> Populate -> Done and navigates to the new space', async () => {
    const spaceResp = {
      sharedObjectRef: { providerResourceRef: { id: '01HXYZ' } },
    }
    mockSessionCreateSpace.mockResolvedValue(spaceResp)
    mockCreateSetup.mockResolvedValue({
      space: {},
      spaceContents: {},
      spaceWorld: {},
      spaceWorldState: {},
    })
    mockPopulateSpace.mockResolvedValue(undefined)

    act(() => {
      render(<CreateSpaceRoute />)
    })

    await waitFor(() => {
      expect(mockUseSessionNavigate).toHaveBeenCalledWith({
        path: 'so/01HXYZ',
        replace: true,
      })
    })

    expect(mockSessionCreateSpace).toHaveBeenCalledTimes(1)
    expect(mockSessionCreateSpace.mock.calls[0]?.[0]).toEqual({
      spaceName: 'My Drive',
    })
    expect(mockCreateSetup).toHaveBeenCalledTimes(1)
    expect(mockPopulateSpace).toHaveBeenCalledTimes(1)
    expect(mockPopulateSpace.mock.calls[0]?.[0]).toBe('drive')
    expect(mockExecuteDynamicQuickstart).not.toHaveBeenCalled()
  })

  it('executes dynamic quickstart registrations after mounting the new space', async () => {
    setParams({ quickstartId: 'glados-workspace' })
    mockUseVisibleQuickstartOptions.mockReturnValue([
      {
        id: 'glados-workspace',
        name: 'Glados Workspace',
        description: 'Operator workspace',
        category: 'tools',
        icon: () => null,
        dynamic: true,
        pluginId: 'glados-web',
        spaceName: 'Glados Workspace',
      },
    ])
    const root = { client: {} }
    const setup = {
      space: { id: 42 },
      spaceContents: {},
      spaceWorld: {},
      spaceWorldState: {},
    }
    const spaceResp = {
      sharedObjectRef: { providerResourceRef: { id: '01HXYZ' } },
    }
    const rootResource = { value: root }
    const sessionResource = { value: makeSession() }
    mockUseSessionContext.mockReturnValue(sessionResource)
    mockUseRootResource.mockReturnValue(rootResource)
    mockUseResourceValue.mockImplementation((resource) => {
      if (resource === rootResource) return root
      return sessionResource.value
    })
    mockSessionCreateSpace.mockResolvedValue(spaceResp)
    mockCreateSetup.mockResolvedValue(setup)
    mockExecuteDynamicQuickstart.mockResolvedValue(undefined)

    act(() => {
      render(<CreateSpaceRoute />)
    })

    await waitFor(() => {
      expect(mockUseSessionNavigate).toHaveBeenCalledWith({
        path: 'so/01HXYZ',
        replace: true,
      })
    })

    expect(mockSessionCreateSpace.mock.calls[0]?.[0]).toEqual({
      spaceName: 'Glados Workspace',
    })
    expect(mockExecuteDynamicQuickstart).toHaveBeenCalledWith(
      root,
      'glados-workspace',
      expect.objectContaining({ space: setup.space }),
      expect.any(AbortSignal),
    )
    expect(mockPopulateSpace).not.toHaveBeenCalled()
  })

  it('passes organization ownership when launched from an organization route', async () => {
    mockParams.mockReturnValue({ quickstartId: 'drive', orgId: 'org-1' })
    const spaceResp = {
      sharedObjectRef: { providerResourceRef: { id: '01HXYZ' } },
    }
    mockSessionCreateSpace.mockResolvedValue(spaceResp)
    mockCreateSetup.mockResolvedValue({
      space: {},
      spaceContents: {},
      spaceWorld: {},
      spaceWorldState: {},
    })
    mockPopulateSpace.mockResolvedValue(undefined)

    act(() => {
      render(<CreateSpaceRoute />)
    })

    await waitFor(() => {
      expect(mockSessionCreateSpace).toHaveBeenCalledWith(
        {
          spaceName: 'My Drive',
          ownerType: 'organization',
          ownerId: 'org-1',
        },
        expect.any(AbortSignal),
      )
    })
  })

  it('surfaces populateSpace errors and allows retry', async () => {
    const spaceResp = {
      sharedObjectRef: { providerResourceRef: { id: '01HXYZ' } },
    }
    mockSessionCreateSpace.mockResolvedValue(spaceResp)
    mockCreateSetup.mockResolvedValue({
      space: {},
      spaceContents: {},
      spaceWorld: {},
      spaceWorldState: {},
    })
    mockPopulateSpace.mockRejectedValueOnce(new Error('populate failed'))

    act(() => {
      render(<CreateSpaceRoute />)
    })

    const retryButton = await screen.findByRole('button', { name: 'Retry' })
    expect(screen.getByText('populate failed')).toBeDefined()
    expect(mockUseSessionNavigate).not.toHaveBeenCalled()

    mockPopulateSpace.mockResolvedValueOnce(undefined)
    act(() => {
      fireEvent.click(retryButton)
    })

    await waitFor(() => {
      expect(mockUseSessionNavigate).toHaveBeenCalledWith({
        path: 'so/01HXYZ',
        replace: true,
      })
    })
    expect(mockSessionCreateSpace).toHaveBeenCalledTimes(2)
    expect(mockPopulateSpace).toHaveBeenCalledTimes(2)
  })

  it('Cancel returns to the session dashboard without calling deleteSpace and aborts the pipeline', async () => {
    const createDeferred = deferred<unknown>()
    mockSessionCreateSpace.mockImplementation(
      (_req: unknown, signal: AbortSignal) => {
        signal.addEventListener('abort', () => {
          createDeferred.reject(signal.reason ?? new Error('aborted'))
        })
        return createDeferred.promise
      },
    )

    act(() => {
      render(<CreateSpaceRoute />)
    })

    const cancelButton = await screen.findByRole('button', { name: 'Cancel' })
    act(() => {
      fireEvent.click(cancelButton)
    })

    expect(mockUseSessionNavigate).toHaveBeenCalledWith({ path: '' })
    expect(mockSessionDeleteSpace).not.toHaveBeenCalled()

    cleanup()
    await waitFor(() => {
      const abortSignal = mockSessionCreateSpace.mock.calls[0]?.[1] as
        | AbortSignal
        | undefined
      expect(abortSignal?.aborted).toBe(true)
    })
  })

  it('redirects invalid quickstart ids to the dashboard without creating', () => {
    setParams({ quickstartId: 'local' })
    act(() => {
      render(<CreateSpaceRoute />)
    })
    expect(mockUseSessionNavigate).toHaveBeenCalledWith({
      path: '',
      replace: true,
    })
    expect(mockToastError).toHaveBeenCalled()
    expect(mockSessionCreateSpace).not.toHaveBeenCalled()
  })

  it('returns to the organization dashboard for invalid org quickstarts', () => {
    setParams({ quickstartId: 'local', orgId: 'org-1' })

    act(() => {
      render(<CreateSpaceRoute />)
    })

    expect(mockUseSessionNavigate).toHaveBeenCalledWith({
      path: 'org/org-1/',
      replace: true,
    })
    expect(mockSessionCreateSpace).not.toHaveBeenCalled()
  })
})
