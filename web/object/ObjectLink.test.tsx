import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AddTabRequest } from '@s4wave/sdk/layout/layout.pb.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import {
  createObjectLinkAddTabRequest,
  ObjectLink,
  objectLinkTabId,
} from './ObjectLink.js'
import { TabContextProvider, type TabContextValue } from './TabContext.js'

const writeText = vi.fn()

function renderWithSpace(
  children: ReactNode,
  overrides: {
    navigateToObjects?: (keys: string[]) => void
    buildObjectUrls?: (keys: string[]) => string[]
  } = {},
) {
  const navigateToObjects = overrides.navigateToObjects ?? vi.fn()
  const buildObjectUrls =
    overrides.buildObjectUrls ??
    ((keys: string[]) =>
      keys.map((key) => '/space/-/' + encodeURIComponent(key)))

  const result = render(
    <SpaceContainerContext.Provider
      spaceId="space-1"
      spaceState={{}}
      spaceWorldResource={{ value: {} } as never}
      spaceWorld={{} as never}
      navigateToRoot={vi.fn()}
      navigateToObjects={navigateToObjects}
      buildObjectUrls={buildObjectUrls}
      navigateToSubPath={vi.fn()}
    >
      {children}
    </SpaceContainerContext.Provider>,
  )

  return { ...result, navigateToObjects, buildObjectUrls }
}

describe('ObjectLink', () => {
  beforeEach(() => {
    writeText.mockReset()
    writeText.mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    vi.spyOn(window, 'open').mockImplementation(() => null)
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('navigates the current space route outside ObjectLayout', () => {
    const navigateToObjects = vi.fn()

    renderWithSpace(
      <ObjectLink
        objectKey="glados/workfront/alpha"
        objectType="glados/workfront"
        label="Alpha workfront"
        kind="Workfront"
      />,
      { navigateToObjects },
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'Open Alpha workfront' }),
    )

    expect(navigateToObjects).toHaveBeenCalledWith(['glados/workfront/alpha'])
  })

  it('adds an ObjectLayout sibling tab inside layout context', () => {
    const addTab = vi
      .fn<TabContextValue['addTab']>()
      .mockResolvedValue({ tabId: 'object-link-tab' })
    const navigateToObjects = vi.fn()
    const tabContext: TabContextValue = {
      tabId: 'current-tab',
      addTab,
      navigateTab: vi.fn(),
      isObjectLayout: true,
    }

    renderWithSpace(
      <TabContextProvider value={tabContext}>
        <ObjectLink
          objectKey="glados/question/needs-choice"
          objectType="glados/question"
          componentID="glados.question"
          label="Needs choice"
          kind="Question"
          status="pending"
        />
      </TabContextProvider>,
      { navigateToObjects },
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Needs choice' }))

    expect(navigateToObjects).not.toHaveBeenCalled()
    expect(addTab).toHaveBeenCalledTimes(1)
    const request: AddTabRequest | undefined = addTab.mock.calls[0]?.[0]
    if (!request?.tab) {
      throw new Error('expected ObjectLink to add an ObjectLayout tab')
    }
    expect(request).toMatchObject({
      afterTabId: 'current-tab',
      select: true,
      tab: {
        name: 'Needs choice',
        helpText: 'glados/question/needs-choice',
        enableClose: true,
      },
    })
    expect(request.tab.id).toBe(
      objectLinkTabId({
        objectKey: 'glados/question/needs-choice',
        objectType: 'glados/question',
        componentID: 'glados.question',
      }),
    )
    const layoutTab = ObjectLayoutTab.fromBinary(request.tab.data)
    expect(layoutTab.componentId).toBe('glados.question')
    expect(layoutTab.objectInfo?.info).toMatchObject({
      case: 'worldObjectInfo',
      value: {
        objectKey: 'glados/question/needs-choice',
        objectType: 'glados/question',
      },
    })
  })

  it('copies and opens the referenced object through secondary actions', () => {
    renderWithSpace(
      <ObjectLink
        objectKey="glados/evidence/result"
        objectType="glados/evidence"
        label="Evidence result"
      />,
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'Copy Evidence result' }),
    )
    expect(writeText).toHaveBeenCalledWith('glados/evidence/result')

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Open Evidence result in new tab',
      }),
    )
    expect(window.open).toHaveBeenCalledWith(
      '/space/-/glados%2Fevidence%2Fresult',
      '_blank',
      'noopener,noreferrer',
    )
  })

  it('does not report copy success when clipboard write fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    writeText.mockRejectedValueOnce(new Error('denied'))

    renderWithSpace(
      <ObjectLink
        objectKey="glados/evidence/result"
        objectType="glados/evidence"
        label="Evidence result"
      />,
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'Copy Evidence result' }),
    )

    await waitFor(() => {
      expect(warn).toHaveBeenCalledWith(
        'ObjectLink: failed to copy object key:',
        expect.any(Error),
      )
    })
  })

  it('renders missing refs without navigation actions', () => {
    renderWithSpace(<ObjectLink label="Decision proof" kind="Evidence" />)

    expect(screen.getByText('Decision proof')).toBeDefined()
    expect(
      screen.getByRole('button', { name: 'Open Decision proof' }),
    ).toHaveProperty('disabled', true)
    expect(
      screen.queryByRole('button', { name: 'Copy Decision proof' }),
    ).toBeNull()
  })

  it('builds stable ObjectLayout add requests for duplicate focus', () => {
    const a = createObjectLinkAddTabRequest({
      objectKey: 'glados/decision/approved',
      objectType: 'glados/decision',
      componentID: 'glados.decision',
      label: 'Approved',
      currentTabId: 'source',
    })
    const b = createObjectLinkAddTabRequest({
      objectKey: 'glados/decision/approved',
      objectType: 'glados/decision',
      componentID: 'glados.decision',
      label: 'Approved',
      currentTabId: 'source',
    })

    expect(a.tab?.id).toBe(b.tab?.id)
    expect(a.afterTabId).toBe('source')
    expect(a.select).toBe(true)
  })

  it('keeps component and route path in stable ObjectLayout tab requests', () => {
    const proof = createObjectLinkAddTabRequest({
      objectKey: 'glados/decision/approved',
      objectType: 'glados/decision',
      componentID: 'glados.decision',
      label: 'Proof',
      path: '/proof',
      currentTabId: 'source',
    })
    const internals = createObjectLinkAddTabRequest({
      objectKey: 'glados/decision/approved',
      objectType: 'glados/decision',
      componentID: 'spacewave.debug.viewer',
      label: 'Internals',
      path: '/internals',
      currentTabId: 'source',
    })

    expect(proof.tab?.id).not.toBe(internals.tab?.id)
    const proofTab = ObjectLayoutTab.fromBinary(
      proof.tab?.data ?? new Uint8Array(),
    )
    const internalsTab = ObjectLayoutTab.fromBinary(
      internals.tab?.data ?? new Uint8Array(),
    )
    expect(proofTab.componentId).toBe('glados.decision')
    expect(proofTab.path).toBe('/proof')
    expect(internalsTab.componentId).toBe('spacewave.debug.viewer')
    expect(internalsTab.path).toBe('/internals')
  })
})
