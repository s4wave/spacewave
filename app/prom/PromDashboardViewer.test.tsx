import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { PromDashboardViewer } from './PromDashboardViewer.js'
import { formatClock } from './prom-view.js'

// reads holds the Dashboard and targets streams the viewer follows, in the
// order it opens them.
const { reads } = vi.hoisted(() => ({
  reads: {
    calls: 0,
    dash: null as unknown as Resource<unknown>,
    targets: null as unknown as Resource<unknown>,
    handle: null as unknown as Resource<unknown>,
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () =>
    reads.calls++ % 2 === 0 ? reads.dash : reads.targets,
}))

vi.mock('@s4wave/web/hooks/useWatchTypedHandle.js', async (importActual) => ({
  ...(await importActual<object>()),
  useWatchTypedHandle: () => reads.handle,
}))

// container is the Space context the viewer reads its write access from.
const { container } = vi.hoisted(() => ({
  container: { canDeleteObjects: false } as Record<string, unknown>,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: { useContextSafe: () => container },
}))

vi.mock('@s4wave/app/alarm/AlarmStrip.js', () => ({
  AlarmStrip: () => null,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  usePath: () => '/',
  useNavigate: () => vi.fn(),
}))

// resource builds one stream's state.
function resource(
  value: unknown,
  error: Error | null = null,
  loading = false,
  last: unknown = null,
): Resource<unknown> {
  return resourceOf({ value, last, loading, error, retry: vi.fn() })
}

const worldState = resource({} as IWorldState) as Resource<IWorldState>

// promHandle builds a Dashboard handle whose queries never answer.
function promHandle() {
  const pending = () => new Promise<never>(() => {})
  return {
    labelValues: vi.fn(pending),
    labelNames: vi.fn(pending),
    query: vi.fn(pending),
  }
}

function viewerElement(objectKey = 'dash/forge') {
  return (
    <PromDashboardViewer
      objectInfo={{
        info: {
          case: 'worldObjectInfo',
          value: { objectKey, objectType: 'prom/dashboard' },
        },
      }}
      worldState={worldState}
    />
  )
}

function renderViewer() {
  return render(viewerElement())
}

describe('PromDashboardViewer', () => {
  beforeEach(() => {
    reads.calls = 0
    reads.handle = resource({ state: 'ready', handle: {} })
  })
  afterEach(cleanup)

  it('shows a failed targets read with its retry instead of waiting', () => {
    // The Dashboard arrived; the targets stream failed.
    reads.dash = resource({ panels: [] })
    reads.targets = resource(null, new Error('targets stream closed'))
    renderViewer()

    // The failure shows with the targets' retry, not a skeleton.
    expect(
      screen.getByText('Could not read the Dashboard: targets stream closed'),
    ).toBeDefined()
    expect(screen.queryByText('Loading the Dashboard')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(reads.targets.retry).toHaveBeenCalledTimes(1)
  })

  it('keeps the read records under a later targets failure, claiming nothing empty', () => {
    // Both records arrived, then the targets stream failed past its backoff.
    // The kept target last scraped at scrapedMs.
    const scrapedMs = Date.UTC(2026, 10, 14, 11, 30)
    reads.dash = resource({ panels: [] })
    reads.targets = resource(
      null,
      new Error('targets stream closed'),
      false,
      new Map([
        ['target/forge', { status: { lastScrapeMs: BigInt(scrapedMs) } }],
      ]),
    )
    renderViewer()

    // The kept target is drawn: its last scrape dates the Dashboard.
    expect(screen.getByText(`Updated ${formatClock(scrapedMs)}`)).toBeDefined()

    // The failure is a notice with Retry; the empty states stay unclaimed.
    expect(
      screen.getByText('Could not read the Dashboard: targets stream closed'),
    ).toBeDefined()
    expect(
      screen.queryByText('No Scrape targets in this Space yet.'),
    ).toBeNull()
    expect(screen.queryByText('This Dashboard has no panels.')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(reads.targets.retry).toHaveBeenCalledTimes(1)
  })

  it('keeps an open draft under a lost handle, offering no Save', () => {
    // A writer opens the new panel editor and types a title.
    container.canDeleteObjects = true
    reads.dash = resource({ panels: [] })
    reads.targets = resource(new Map([['target/forge', {}]]))
    const view = renderViewer()
    fireEvent.click(screen.getByRole('button', { name: 'Add panel' }))
    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'Load' },
    })
    expect(screen.getByRole('button', { name: 'Save' })).toBeDefined()

    // The handle watch fails past its backoff, failing both record watches
    // over their last records.
    const lost = new Error('plugin stream closed')
    reads.handle = resource(null, lost)
    reads.dash = resource(null, lost, false, { panels: [] })
    reads.targets = resource(null, lost, false, new Map([['target/forge', {}]]))
    view.rerender(viewerElement())

    // The draft stays; Save is withheld and the notice says why.
    expect((screen.getByLabelText('Title') as HTMLInputElement).value).toBe(
      'Load',
    )
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect(
      screen.getByText('Could not read the Dashboard: plugin stream closed'),
    ).toBeDefined()

    // The held handle never crosses to another Dashboard.
    view.rerender(viewerElement('dash/other'))
    expect(screen.queryByLabelText('Title')).toBeNull()
    container.canDeleteObjects = false
  })

  it('keeps an open draft through a handle Retry and the handle returning', async () => {
    // A writer opens the new panel editor on the first handle and types.
    container.canDeleteObjects = true
    const first = promHandle()
    reads.handle = resource({ state: 'ready', handle: first })
    reads.dash = resource({ panels: [] })
    reads.targets = resource(new Map([['target/forge', {}]]))
    const view = renderViewer()
    fireEvent.click(screen.getByRole('button', { name: 'Add panel' }))
    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'Load' },
    })
    const firstQueries = first.labelValues.mock.calls.length

    // The handle fails, then its Retry reloads it. The record watches stay
    // failed over their last records until the next handle's watches yield.
    const lost = new Error('plugin stream closed')
    const kept = (last: unknown) => resource(null, lost, false, last)
    reads.handle = resource(null, lost)
    reads.dash = kept({ panels: [] })
    reads.targets = kept(new Map([['target/forge', {}]]))
    view.rerender(viewerElement())
    reads.handle = resource(null, null, true)
    view.rerender(viewerElement())

    // The draft stays under the reload, Save withheld, and typing a
    // selector runs no suggestion query on the released handle.
    expect((screen.getByLabelText('Title') as HTMLInputElement).value).toBe(
      'Load',
    )
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    const expr = screen.getByLabelText('Expression') as HTMLTextAreaElement
    fireEvent.change(expr, { target: { value: 'up{j', selectionStart: 4 } })
    await act(async () => {})
    expect(first.labelValues.mock.calls.length).toBe(firstQueries)
    expect(first.labelNames).not.toHaveBeenCalled()

    // The next handle arrives, then its record watches yield.
    const next = promHandle()
    reads.handle = resource({ state: 'ready', handle: next })
    view.rerender(viewerElement())
    reads.dash = resource({ panels: [] })
    reads.targets = resource(new Map([['target/forge', {}]]))
    view.rerender(viewerElement())

    // The draft is still open, Save returns, and suggestions query the
    // next handle.
    expect((screen.getByLabelText('Title') as HTMLInputElement).value).toBe(
      'Load',
    )
    await act(async () => {})
    expect(screen.getByRole('button', { name: 'Save' })).toBeDefined()
    expect(next.labelValues).toHaveBeenCalled()
    container.canDeleteObjects = false
  })

  it('waits while either record has not arrived', () => {
    // The targets are still on their first read.
    reads.dash = resource({ panels: [] })
    reads.targets = resource(null, null, true)
    renderViewer()

    expect(screen.getByText('Loading the Dashboard')).toBeDefined()
  })
})
