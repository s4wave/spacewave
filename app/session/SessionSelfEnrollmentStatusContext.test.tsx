import { describe, expect, it } from 'vitest'

import { buildSessionSelfEnrollmentStatusView } from './SessionSelfEnrollmentStatusContext.js'

describe('buildSessionSelfEnrollmentStatusView', () => {
  it('maps pending projection state to local labels and affordances', () => {
    const view = buildSessionSelfEnrollmentStatusView(
      null,
      {
        count: 2,
        sharedObjectIds: ['so-1', 'so-2'],
        completedSharedObjectIds: ['so-1'],
        generationKey: 'gen-1',
      },
      false,
      null,
    )

    expect(view.visible).toBe(true)
    expect(view.summaryLabel).toBe('Spaces need this session')
    expect(view.detailLabel).toBe('2 spaces can connect in the background.')
    expect(view.ariaLabel).toBe(
      'Session self-enrollment status: Spaces need this session',
    )
    expect(view.progressVisible).toBe(true)
    expect(view.progressIndeterminate).toBe(false)
    expect(view.progress).toBe(50)
    expect(view.startVisible).toBe(true)
    expect(view.startLabel).toBe('Connect')
    expect(view.skipVisible).toBe(true)
  })

  it('keeps an idle projection out of the bottom bar', () => {
    const view = buildSessionSelfEnrollmentStatusView(
      null,
      { count: 0 },
      false,
      null,
    )

    expect(view.visualState).toBe('ready')
    expect(view.visible).toBe(false)
    expect(view.progressVisible).toBe(false)
    expect(view.startVisible).toBe(false)
    expect(view.skipVisible).toBe(false)
  })

  it('keeps watcher errors hidden unless the projection contains failed work', () => {
    const view = buildSessionSelfEnrollmentStatusView(
      null,
      null,
      false,
      new Error('unimplemented'),
    )

    expect(view.visualState).toBe('failed')
    expect(view.failed).toBe(true)
    expect(view.visible).toBe(false)
    expect(view.summaryLabel).toBe('Connection status unavailable')
    expect(view.detailLabel).toBe('unimplemented')
  })
})
