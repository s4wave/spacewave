import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  render,
  screen,
  cleanup,
  fireEvent,
  act,
  waitFor,
} from '@testing-library/react'
import { PinUnlockOverlay } from './PinUnlockOverlay.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => null,
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 1,
}))

describe('PinUnlockOverlay', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  const baseMetadata: SessionMetadata = {
    displayName: 'Test Session',
    providerDisplayName: 'Test Provider',
  }

  it('renders the session display name', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Test Session')).toBeDefined()
  })

  it('renders "Locked Session" when displayName is empty', () => {
    render(
      <PinUnlockOverlay
        metadata={{ providerDisplayName: 'Provider' }}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Locked Session')).toBeDefined()
  })

  it('renders provider display name', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Test Provider')).toBeDefined()
  })

  it('does not render provider name when not provided', () => {
    render(
      <PinUnlockOverlay
        metadata={{ displayName: 'Session' }}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.queryByText('Test Provider')).toBeNull()
  })

  it('renders PIN input field', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByPlaceholderText('PIN')).toBeDefined()
  })

  it('renders "Enter PIN to unlock" label', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Enter PIN to unlock')).toBeDefined()
  })

  it('renders unlock button', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Unlock')).toBeDefined()
  })

  it('disables unlock button when PIN is empty', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    const unlockButton = screen.getByText('Unlock').closest('button')
    expect(unlockButton?.hasAttribute('disabled')).toBe(true)
  })

  it('enables unlock button when PIN has content', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    const input = screen.getByPlaceholderText('PIN')
    fireEvent.change(input, { target: { value: '1234' } })
    const unlockButton = screen.getByText('Unlock').closest('button')
    expect(unlockButton?.hasAttribute('disabled')).toBe(false)
  })

  it('shows error when unlock rejects', async () => {
    const onUnlock = vi.fn()
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={onUnlock}
        onReset={vi.fn()}
      />,
    )
    // The unlock button is disabled when PIN is empty, but the handleUnlock
    // function also guards against empty PIN. We can test the error display
    // by entering a PIN, triggering unlock with a rejection, then checking error.
    const input = screen.getByPlaceholderText('PIN')
    fireEvent.change(input, { target: { value: '9999' } })

    onUnlock.mockRejectedValueOnce(new Error('Wrong PIN. Please try again.'))

    const unlockButton = screen.getByText('Unlock').closest('button')
    act(() => {
      fireEvent.click(unlockButton!)
    })
    await waitFor(() => {
      expect(screen.getByText('Wrong PIN. Please try again.')).toBeDefined()
    })
  })

  it('shows "Unlocking..." text during unlock', () => {
    // Use a promise that never resolves to keep the unlocking state active.
    const onUnlock = vi.fn(() => new Promise<void>(() => {}))
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={onUnlock}
        onReset={vi.fn()}
      />,
    )
    const input = screen.getByPlaceholderText('PIN')
    fireEvent.change(input, { target: { value: '1234' } })

    const unlockButton = screen.getByText('Unlock').closest('button')
    act(() => {
      fireEvent.click(unlockButton!)
    })
    expect(screen.getByText('Unlocking...')).toBeDefined()
  })

  it('renders "Forgot PIN?" button', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Forgot PIN?')).toBeDefined()
  })

  it('shows forgot PIN help text when "Forgot PIN?" is clicked', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByText('Forgot PIN?'))
    expect(
      screen.getByText(
        /Re-authenticate with your account password or backup key/,
      ),
    ).toBeDefined()
  })

  it('hides forgot PIN help text when "Back to PIN entry" is clicked', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByText('Forgot PIN?'))
    expect(
      screen.getByText(
        /Re-authenticate with your account password or backup key/,
      ),
    ).toBeDefined()
    fireEvent.click(screen.getByText('Back to PIN entry'))
    expect(
      screen.queryByText(
        /Re-authenticate with your account password or backup key/,
      ),
    ).toBeNull()
  })

  it('renders "Sessions" back button', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    expect(screen.getByText('Sessions')).toBeDefined()
  })

  it('renders password type input for PIN', () => {
    render(
      <PinUnlockOverlay
        metadata={baseMetadata}
        onUnlock={vi.fn()}
        onReset={vi.fn()}
      />,
    )
    const input = screen.getByPlaceholderText('PIN')
    expect(input.getAttribute('type')).toBe('password')
  })
})
