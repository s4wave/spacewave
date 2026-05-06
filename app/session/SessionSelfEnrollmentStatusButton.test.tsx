import type { ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SessionSelfEnrollmentStatusButton } from './SessionSelfEnrollmentStatusButton.js'
import {
  buildSessionSelfEnrollmentStatusView,
  type SessionSelfEnrollmentStatusView,
} from './SessionSelfEnrollmentStatusContext.js'

const mockUseSessionSelfEnrollmentStatus = vi.hoisted(() => vi.fn())
const mockToggle = vi.hoisted(() => vi.fn())
const mockSelected = vi.hoisted(() => ({ value: true }))

vi.mock('./SessionSelfEnrollmentStatusContext.js', async () => {
  const actual = await vi.importActual<
    typeof import('./SessionSelfEnrollmentStatusContext.js')
  >('./SessionSelfEnrollmentStatusContext.js')
  return {
    ...actual,
    useSessionSelfEnrollmentStatus: mockUseSessionSelfEnrollmentStatus,
  }
})

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: (props: {
    id: string
    position?: 'left' | 'right'
    button: (
      selected: boolean,
      onClick: () => void,
      className?: string,
    ) => ReactNode
    children?: ReactNode
  }) => (
    <div
      data-testid={`bottom-bar-level-${props.id}`}
      data-position={props.position ?? 'left'}
    >
      {props.button(mockSelected.value, mockToggle, '')}
      {props.children}
    </div>
  ),
}))

describe('SessionSelfEnrollmentStatusButton', () => {
  beforeEach(() => {
    mockToggle.mockClear()
    mockSelected.value = true
  })

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
      detail: '1 space need an account unlock before this session can connect.',
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

    render(<SessionSelfEnrollmentStatusButton />)

    expect(
      screen.getByTestId('bottom-bar-level-session-self-enrollment-status'),
    ).toBeTruthy()
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
        resource: { start, skip } as unknown as SessionSelfEnrollmentStatusView['resource'],
        count: 1,
        sharedObjectIds: ['a'],
        generationKey: 'gen-1',
      }),
    )

    render(<SessionSelfEnrollmentStatusButton />)

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

    render(<SessionSelfEnrollmentStatusButton />)

    expect(
      screen.queryByTestId('bottom-bar-level-session-self-enrollment-status'),
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
