import { useState } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'

import { SessionSelfEnrollmentStatusButton } from './SessionSelfEnrollmentStatusButton.js'
import {
  buildSessionSelfEnrollmentStatusView,
  type SessionSelfEnrollmentStatusView,
} from './SessionSelfEnrollmentStatusContext.js'

const mockUseSessionSelfEnrollmentStatus = vi.hoisted(() => vi.fn())

vi.mock('./SessionSelfEnrollmentStatusContext.js', async () => {
  const actual = await vi.importActual<
    typeof import('./SessionSelfEnrollmentStatusContext.js')
  >('./SessionSelfEnrollmentStatusContext.js')
  return {
    ...actual,
    useSessionSelfEnrollmentStatus: mockUseSessionSelfEnrollmentStatus,
  }
})

function Harness() {
  const [openMenu, setOpenMenu] = useState('session-self-enrollment-status')
  return (
    <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
      <SessionSelfEnrollmentStatusButton />
      <ViewerFrame />
    </BottomBarRoot>
  )
}

describe('SessionSelfEnrollmentStatusButton', () => {
  afterEach(() => {
    cleanup()
  })

  it.each([
    {
      name: 'running',
      view: view({
        running: true,
        count: 2,
        sharedObjectIds: ['a', 'b'],
        completedSharedObjectIds: ['a'],
      }),
      label: 'Connecting spaces',
      detail: 'Connecting 2 spaces.',
    },
    {
      name: 'waiting for step-up',
      view: view({
        credentialRequired: true,
        count: 1,
        sharedObjectIds: ['a'],
      }),
      label: 'Spaces need this session key',
      detail:
        '1 space needs an account unlock before this session can connect.',
    },
    {
      name: 'failed',
      view: view({
        count: 1,
        sharedObjectIds: ['a'],
        failures: [{ sharedObjectId: 'a', message: 'not a participant' }],
      }),
      label: 'Some spaces need attention',
      detail: '1 space failed to connect.',
      failure: 'not a participant',
    },
    {
      name: 'skipped',
      view: view({
        count: 1,
        sharedObjectIds: ['a'],
        generationKey: 'gen-1',
        skipped: true,
      }),
      label: 'Space connection skipped',
      detail: 'This generation will stay skipped for now.',
    },
  ])('renders $name status', (test) => {
    mockUseSessionSelfEnrollmentStatus.mockReturnValue(test.view)

    render(<Harness />)

    expect(
      screen.getByTestId('session-self-enrollment-status-button'),
    ).toBeTruthy()
    expect(
      screen
        .getByTestId('session-self-enrollment-status-button')
        .getAttribute('aria-label'),
    ).toBe(`Session self-enrollment status: ${test.label}`)
    expect(screen.getByText(test.label)).toBeTruthy()
    expect(screen.getByText(test.detail)).toBeTruthy()
    if (test.failure) {
      expect(screen.getByText(test.failure)).toBeTruthy()
    }
  })

  it('starts and skips through the mounted self-enrollment resource', async () => {
    const start = vi.fn().mockResolvedValue(undefined)
    const skip = vi.fn().mockResolvedValue(undefined)
    mockUseSessionSelfEnrollmentStatus.mockReturnValue(
      view({
        resource: {
          start,
          skip,
        } as unknown as SessionSelfEnrollmentStatusView['resource'],
        count: 1,
        sharedObjectIds: ['a'],
        generationKey: 'gen-1',
      }),
    )

    render(<Harness />)

    fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(start).toHaveBeenCalledTimes(1)
    })
    fireEvent.click(screen.getByRole('button', { name: 'Skip' }))

    await waitFor(() => {
      expect(skip).toHaveBeenCalledWith('gen-1')
    })
  })

  it('stays hidden when no self-enrollment work is visible', () => {
    mockUseSessionSelfEnrollmentStatus.mockReturnValue(view({ count: 0 }))

    render(<Harness />)

    expect(
      screen.queryByTestId('session-self-enrollment-status-button'),
    ).toBeNull()
  })

  it('stays hidden while idle status is still loading', () => {
    mockUseSessionSelfEnrollmentStatus.mockReturnValue({
      ...view({ count: 0 }),
      visualState: 'loading',
      loading: true,
      summaryLabel: 'Checking connected spaces',
      detailLabel: 'Waiting for connected-space status.',
    })

    render(<Harness />)

    expect(
      screen.queryByTestId('session-self-enrollment-status-button'),
    ).toBeNull()
  })

  it('stays hidden when the idle status watch is unavailable', () => {
    mockUseSessionSelfEnrollmentStatus.mockReturnValue(
      buildSessionSelfEnrollmentStatusView(
        null,
        null,
        false,
        new Error('unimplemented'),
      ),
    )

    render(<Harness />)

    expect(
      screen.queryByTestId('session-self-enrollment-status-button'),
    ).toBeNull()
  })
})

function view(
  snapshot: Parameters<typeof buildSessionSelfEnrollmentStatusView>[1] & {
    resource?: SessionSelfEnrollmentStatusView['resource']
  },
): SessionSelfEnrollmentStatusView {
  const { resource = null, ...state } = snapshot
  return buildSessionSelfEnrollmentStatusView(resource, state, false, null)
}
