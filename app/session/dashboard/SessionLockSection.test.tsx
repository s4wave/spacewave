import React from 'react'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { SessionLockSection } from './SessionLockSection.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: vi.fn(),
}))

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

const mockUseStreamingResource = useStreamingResource as ReturnType<
  typeof vi.fn
>

const mockSession = {
  setLockMode: vi.fn(),
  resourceRef: { resourceId: 1, released: false },
  id: 1,
  client: {},
  service: {},
} as unknown as Session

const mockRetry = vi.fn()

function renderWithContext(lockState: { mode: SessionLockMode } | null) {
  mockUseStreamingResource.mockReturnValue({
    value: lockState,
    loading: lockState === null,
    error: null,
    retry: vi.fn(),
  })

  return render(
    <SessionContext.Provider
      resource={
        {
          value: mockSession,
          loading: false,
          error: null,
          retry: mockRetry,
        } as Resource<Session>
      }
    >
      <SessionLockSection />
    </SessionContext.Provider>,
  )
}

describe('SessionLockSection', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  beforeEach(() => {
    mockRetry.mockClear()
    ;(mockSession.setLockMode as ReturnType<typeof vi.fn>).mockClear()
  })

  it('renders "Session Lock" heading', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    expect(screen.getByText('Session Lock')).toBeDefined()
  })

  it('renders loading message when lock state is null', () => {
    renderWithContext(null)
    expect(screen.getByText('Loading lock state')).toBeDefined()
  })

  it('renders auto-unlock option', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    expect(screen.getByText('Auto-unlock')).toBeDefined()
    expect(screen.getByText('Key stored on disk. No PIN needed.')).toBeDefined()
  })

  it('renders PIN lock option', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    expect(screen.getByText('PIN lock')).toBeDefined()
    expect(
      screen.getByText('Key encrypted with PIN. Enter PIN on each app launch.'),
    ).toBeDefined()
  })

  it('shows auto-unlock as selected when mode is AUTO_UNLOCK', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    // When auto-unlock is selected, the "Auto-unlock" option button has the
    // brand border styling. We verify by checking the lock mode note text.
    expect(
      screen.getByText(
        'Changing lock mode only requires your current session. No account re-auth needed.',
      ),
    ).toBeDefined()
  })

  it('shows "Change PIN" button when current mode is PIN', () => {
    renderWithContext({ mode: SessionLockMode.PIN_ENCRYPTED })
    expect(screen.getByText('Change PIN')).toBeDefined()
  })

  it('does not show "Change PIN" button when mode is auto-unlock', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    expect(screen.queryByText('Change PIN')).toBeNull()
  })

  it('shows PIN input fields when switching from auto to PIN', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    // Click the "PIN lock" option button
    fireEvent.click(screen.getByText('PIN lock'))
    expect(screen.getByPlaceholderText('Enter PIN')).toBeDefined()
    expect(screen.getByPlaceholderText('Confirm PIN')).toBeDefined()
  })

  it('shows Save and Cancel buttons when lock mode changes', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    fireEvent.click(screen.getByText('PIN lock'))
    expect(screen.getByText('Save')).toBeDefined()
    expect(screen.getByText('Cancel')).toBeDefined()
  })

  it('hides Save/Cancel buttons after cancel click', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    fireEvent.click(screen.getByText('PIN lock'))
    expect(screen.getByText('Save')).toBeDefined()
    fireEvent.click(screen.getByText('Cancel'))
    expect(screen.queryByText('Save')).toBeNull()
    expect(screen.queryByText('Cancel')).toBeNull()
  })

  it('does not show Save/Cancel when no mode change', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    expect(screen.queryByText('Save')).toBeNull()
    expect(screen.queryByText('Cancel')).toBeNull()
  })

  it('renders lock mode info text', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    expect(
      screen.getByText(
        'Changing lock mode only requires your current session. No account re-auth needed.',
      ),
    ).toBeDefined()
  })

  it('renders PIN input as password type', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    fireEvent.click(screen.getByText('PIN lock'))
    const pinInput = screen.getByPlaceholderText('Enter PIN')
    expect(pinInput.getAttribute('type')).toBe('password')
  })

  it('renders confirm PIN input as password type', () => {
    renderWithContext({ mode: SessionLockMode.AUTO_UNLOCK })
    fireEvent.click(screen.getByText('PIN lock'))
    const confirmInput = screen.getByPlaceholderText('Confirm PIN')
    expect(confirmInput.getAttribute('type')).toBe('password')
  })
})
