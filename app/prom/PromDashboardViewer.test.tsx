import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/resourceTransitionState.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { PromDashboardViewer } from './PromDashboardViewer.js'

// reads holds the Dashboard and targets streams the viewer follows, in the
// order it opens them.
const { reads } = vi.hoisted(() => ({
  reads: {
    calls: 0,
    dash: null as unknown as Resource<unknown>,
    targets: null as unknown as Resource<unknown>,
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () =>
    reads.calls++ % 2 === 0 ? reads.dash : reads.targets,
}))

vi.mock('@s4wave/web/hooks/useWatchTypedHandle.js', () => ({
  useWatchTypedHandle: () => ({
    value: { state: 'ready', handle: {} },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
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

function renderViewer() {
  return render(
    <PromDashboardViewer
      objectInfo={{
        info: {
          case: 'worldObjectInfo',
          value: { objectKey: 'dash/forge', objectType: 'prom/dashboard' },
        },
      }}
      worldState={resource({} as IWorldState) as Resource<IWorldState>}
    />,
  )
}

describe('PromDashboardViewer', () => {
  beforeEach(() => {
    reads.calls = 0
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
    reads.dash = resource({ panels: [] })
    reads.targets = resource(
      null,
      new Error('targets stream closed'),
      false,
      new Map(),
    )
    renderViewer()

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

  it('waits while either record has not arrived', () => {
    // The targets are still on their first read.
    reads.dash = resource({ panels: [] })
    reads.targets = resource(null, null, true)
    renderViewer()

    expect(screen.getByText('Loading the Dashboard')).toBeDefined()
  })
})
