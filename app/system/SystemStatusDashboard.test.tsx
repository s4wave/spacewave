import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'

import { SystemStatusDashboard } from './SystemStatusDashboard.js'

const mockNavigate = vi.fn()
const mockSetOpenMenu = vi.hoisted(() => vi.fn())
const mockMobileState = vi.hoisted(() => ({ value: false }))
const mockStatus = vi.hoisted(() => ({
  controllers: [
    { id: 'controller/a', version: '1', description: 'Alpha' },
    { id: 'controller/b', version: '1', description: 'Beta' },
  ],
  directives: [{ name: 'directive/a', ident: 'ident-a' }],
  spaces: [
    {
      entry: {
        ref: {
          providerResourceRef: {
            id: 'space-1',
          },
        },
        source: 'created',
      },
      spaceMeta: {
        name: 'Primary Space',
      },
    },
    {
      entry: {
        ref: {
          providerResourceRef: {
            id: 'space-2',
          },
        },
        source: 'shared',
      },
      spaceMeta: {
        name: 'Shared Space',
      },
    },
  ],
}))

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: () => ({
    value: {
      sessions: [{ sessionIndex: 1 }],
    },
  }),
}))

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: (sessionIndex: number) => ({
    displayName:
      sessionIndex === 1 ? 'Primary Session' : `Session ${sessionIndex}`,
    providerDisplayName: 'Local',
    providerId: 'local',
    providerAccountId: `acct-${sessionIndex}`,
    createdAt: 1735776000000n,
    lockMode: 0,
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 1,
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mockSetOpenMenu,
}))

vi.mock('@s4wave/web/hooks/useMobile.js', () => ({
  useIsMobile: () => mockMobileState.value,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/devtools/ResourceTreeTab.js', () => ({
  ResourceTreeTab: () => <div>Resource Tree</div>,
}))

vi.mock('@s4wave/web/devtools/ResourceDetailsPanel.js', () => ({
  ResourceDetailsPanel: () => <div>Resource Details</div>,
}))

vi.mock('@s4wave/web/devtools/StateTreeTab.js', () => ({
  StateTreeTab: () => <div>State Tree</div>,
}))

vi.mock('@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js', () => ({
  useResourceDevToolsContext: () => null,
  useSelectedResourceId: () => null,
  useTrackedResources: () => new Map(),
}))

vi.mock('./useSystemStatus.js', () => ({
  useWatchControllers: () => ({
    controllerCount: mockStatus.controllers.length,
    controllers: mockStatus.controllers,
  }),
  useWatchDirectives: () => ({
    directiveCount: mockStatus.directives.length,
    directives: mockStatus.directives,
  }),
  useWatchSpacesList: () => mockStatus.spaces,
}))

function renderDashboard(
  props: React.ComponentProps<typeof SystemStatusDashboard> = {},
  rootAtom = atom({}),
) {
  return render(
    <StateNamespaceProvider rootAtom={rootAtom}>
      <SystemStatusDashboard {...props} />
    </StateNamespaceProvider>,
  )
}

describe('SystemStatusDashboard', () => {
  beforeEach(() => {
    cleanup()
    vi.useRealTimers()
    mockMobileState.value = false
    mockNavigate.mockReset()
    mockSetOpenMenu.mockReset()
    mockStatus.controllers = [
      { id: 'controller/a', version: '1', description: 'Alpha' },
      { id: 'controller/b', version: '1', description: 'Beta' },
    ]
    mockStatus.directives = [{ name: 'directive/a', ident: 'ident-a' }]
    mockStatus.spaces = [
      {
        entry: {
          ref: {
            providerResourceRef: {
              id: 'space-1',
            },
          },
          source: 'created',
        },
        spaceMeta: {
          name: 'Primary Space',
        },
      },
      {
        entry: {
          ref: {
            providerResourceRef: {
              id: 'space-2',
            },
          },
          source: 'shared',
        },
        spaceMeta: {
          name: 'Shared Space',
        },
      },
    ]
  })

  it('shows the spaces count in the stats ribbon', () => {
    renderDashboard()
    expect(screen.getByText('2 spc')).toBeDefined()
  })

  it('keeps the placeholder logs drawer hidden until log streaming lands', () => {
    renderDashboard()
    expect(screen.queryByRole('button', { name: 'Expand logs' })).toBeNull()
  })

  it('renders the spaces detail list when selected', () => {
    renderDashboard()
    fireEvent.click(screen.getByText('All spaces'))
    expect(screen.getAllByText('Spaces').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Primary Space').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Shared Space').length).toBeGreaterThan(0)
    expect(screen.getByText('space-1')).toBeDefined()
  })

  it('selects a space detail from the all spaces list', () => {
    renderDashboard()
    fireEvent.click(screen.getByText('All spaces'))
    fireEvent.click(screen.getByText('space-1').closest('button')!)

    expect(screen.getByText('Open Space')).toBeDefined()
    expect(screen.getByText('Source')).toBeDefined()
  })

  it('opens a selected space from the detail view and closes the overlay', () => {
    const onClose = vi.fn()
    renderDashboard({ onClose })
    fireEvent.click(screen.getByText('Primary Space'))
    expect(screen.getByText('Open Space')).toBeDefined()
    fireEvent.click(screen.getByText('Open Space'))
    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/1/so/space-1' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('selects a controller detail from the controllers list', () => {
    renderDashboard()

    fireEvent.click(screen.getByText('Beta').closest('button')!)

    expect(screen.getByText('Controller')).toBeDefined()
    expect(screen.getByText('List Index')).toBeDefined()
  })

  it('lists directive groups in the sidebar and opens the selected group detail', () => {
    renderDashboard()

    expect(screen.getByText('directive/a')).toBeDefined()

    fireEvent.click(screen.getByText('directive/a'))

    expect(screen.getByText('Directive Type')).toBeDefined()
    expect(screen.getByText('ident-a')).toBeDefined()
  })

  it('keeps expanded directive rows above the resources section header', () => {
    renderDashboard()

    const directiveRow = screen.getByText('directive/a').closest('button')
    const resourcesSection = screen.getByRole('treeitem', { name: /Resources/ })

    expect(directiveRow).toBeDefined()
    expect(resourcesSection).toBeDefined()
    expect(
      directiveRow!.compareDocumentPosition(resourcesSection) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0)
  })

  it('expands the truncated sidebar lists', () => {
    mockStatus.spaces = Array.from({ length: 7 }, (_, i) => ({
      entry: {
        ref: {
          providerResourceRef: {
            id: `space-${i + 1}`,
          },
        },
        source: 'created',
      },
      spaceMeta: {
        name: `Space ${i + 1}`,
      },
    }))
    mockStatus.controllers = Array.from({ length: 7 }, (_, i) => ({
      id: `controller/${i + 1}`,
      version: '1',
      description: `Controller ${i + 1}`,
    }))

    renderDashboard()

    const moreButtons = screen.getAllByText('+2 more')
    fireEvent.click(moreButtons[0])
    fireEvent.click(moreButtons[1])

    expect(screen.getByText('Space 7')).toBeDefined()
    expect(screen.getAllByText('controller/7').length).toBeGreaterThan(0)
  })

  it('opens session details for the selected account', async () => {
    renderDashboard()

    fireEvent.click(screen.getByText('Primary Session'))
    fireEvent.click(screen.getByText('Open Session Details'))
    await Promise.resolve()

    expect(mockSetOpenMenu).toHaveBeenCalledWith('account')
  })

  it('shows the mobile sections picker and preserves the current selection', () => {
    const rootAtom = atom({})
    const { rerender } = renderDashboard({}, rootAtom)

    fireEvent.click(screen.getByText('Primary Space'))
    expect(screen.getByText('Open Space')).toBeDefined()

    mockMobileState.value = true
    rerender(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <SystemStatusDashboard />
      </StateNamespaceProvider>,
    )

    expect(screen.getByText('Sections')).toBeDefined()
    expect(screen.getByText('Open Space')).toBeDefined()

    fireEvent.click(screen.getByText('Sections'))
    expect(screen.getByRole('tree')).toBeDefined()
  })

  it('persists durable dashboard state across remounts', () => {
    const rootAtom = atom({})
    const { unmount } = renderDashboard({}, rootAtom)

    fireEvent.click(screen.getByText('All spaces'))
    fireEvent.click(screen.getByText('space-1').closest('button')!)

    expect(screen.getByText('Open Space')).toBeDefined()

    unmount()

    renderDashboard({}, rootAtom)

    expect(screen.getByText('Open Space')).toBeDefined()
  })

  it('supports keyboard tree navigation and section expansion', () => {
    renderDashboard()

    const accountsSection = screen.getByRole('treeitem', { name: /Accounts/ })
    accountsSection.focus()
    fireEvent.keyDown(accountsSection, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(
      screen.getByRole('treeitem', { name: /Primary Session/ }),
    )

    const resourcesSection = screen.getByRole('treeitem', { name: /Resources/ })
    resourcesSection.focus()
    fireEvent.keyDown(resourcesSection, { key: 'ArrowRight' })
    expect(
      screen.getByRole('treeitem', { name: /Resource tree/ }),
    ).toBeDefined()

    fireEvent.keyDown(resourcesSection, { key: 'ArrowLeft' })
    expect(screen.queryByRole('treeitem', { name: /Resource tree/ })).toBeNull()
  })

  it('does not warn when multiple controllers share the same id', () => {
    mockStatus.controllers = [
      { id: 'dup/controller', version: '1', description: 'Alpha' },
      { id: 'dup/controller', version: '2', description: 'Beta' },
      { id: 'dup/controller', version: '3', description: 'Gamma' },
    ]
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    renderDashboard()

    expect(
      errorSpy.mock.calls.some((call) =>
        call.some((arg) => typeof arg === 'string' && arg.includes('same key')),
      ),
    ).toBe(false)

    errorSpy.mockRestore()
  })

  it('shows transient stat deltas and only advances live stamps on snapshot changes', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-13T13:05:00'))
    const rootAtom = atom({})
    const { rerender } = renderDashboard({}, rootAtom)
    const ribbonLive = screen.getByLabelText('Ribbon live')
    const initialStamp = ribbonLive.getAttribute('data-updated-at')

    expect(initialStamp).toBeTruthy()

    vi.setSystemTime(new Date('2026-04-13T13:05:10'))
    rerender(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <SystemStatusDashboard />
      </StateNamespaceProvider>,
    )

    expect(screen.queryByText('+1')).toBeNull()
    expect(
      screen.getByLabelText('Ribbon live').getAttribute('data-updated-at'),
    ).toBe(initialStamp)

    mockStatus.controllers = [
      ...mockStatus.controllers,
      { id: 'controller/c', version: '1', description: 'Gamma' },
    ]
    rerender(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <SystemStatusDashboard />
      </StateNamespaceProvider>,
    )

    expect(screen.getByText('+1')).toBeDefined()
    expect(
      screen.getByLabelText('Ribbon live').getAttribute('data-updated-at'),
    ).not.toBe(initialStamp)

    act(() => {
      vi.advanceTimersByTime(1600)
    })

    expect(screen.queryByText('+1')).toBeNull()
  })

  it('fresh-highlights newly appeared controllers and directive groups', () => {
    const rootAtom = atom({})
    const { rerender } = renderDashboard({}, rootAtom)

    mockStatus.controllers = [
      ...mockStatus.controllers,
      { id: 'controller/c', version: '1', description: 'Gamma' },
    ]
    rerender(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <SystemStatusDashboard />
      </StateNamespaceProvider>,
    )

    const controllerRows = screen
      .getAllByText('controller/c')
      .map((node) => node.closest('button'))
      .filter(
        (node): node is HTMLButtonElement => node instanceof HTMLButtonElement,
      )
    expect(
      controllerRows.some((row) => row.className.includes('bg-success/5')),
    ).toBe(true)

    fireEvent.click(screen.getByText('All directives'))
    mockStatus.directives = [
      ...mockStatus.directives,
      { name: 'directive/b', ident: 'ident-b' },
    ]
    rerender(
      <StateNamespaceProvider rootAtom={rootAtom}>
        <SystemStatusDashboard />
      </StateNamespaceProvider>,
    )

    const directiveRows = screen
      .getAllByText('directive/b')
      .map((node) => node.parentElement)
      .filter((node): node is HTMLElement => node instanceof HTMLElement)
    expect(
      directiveRows.some((row) => row.className.includes('bg-warning/6')),
    ).toBe(true)
  })
})
