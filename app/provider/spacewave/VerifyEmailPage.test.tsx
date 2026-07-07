import type { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import type { EmailManagement } from '@s4wave/web/hooks/useEmailManagement.js'
import type { EmailInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { VerifyEmailPage } from './VerifyEmailPage.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseEmailManagement = vi.hoisted(() => vi.fn<() => EmailManagement>())
const mockUseOnboardingContext = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/app/session/SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

vi.mock('@s4wave/web/hooks/useEmailManagement.js', () => ({
  useEmailManagement: mockUseEmailManagement,
}))

vi.mock('@s4wave/web/contexts/SpacewaveOnboardingContext.js', () => ({
  SpacewaveOnboardingContext: {
    useContextSafe: mockUseOnboardingContext,
  },
}))

describe('VerifyEmailPage', () => {
  let emailManagement: EmailManagement

  beforeEach(() => {
    cleanup()
    mockNavigate.mockReset()
    mockUseEmailManagement.mockReset()
    mockUseOnboardingContext.mockReset()
    mockUseOnboardingContext.mockReturnValue({ emailVerified: false })
    emailManagement = makeEmailManagement({
      emails: [makeEmail({ email: 'casey@example.com', verified: false })],
      verifyingEmail: 'casey@example.com',
      code: '123456',
    })
    mockUseEmailManagement.mockImplementation(() => emailManagement)
  })

  afterEach(() => {
    cleanup()
  })

  it('replaces a stale code flow when Onboarding Status observes out-of-band verification', () => {
    const { rerender } = render(<VerifyEmailPage />)

    expect(screen.getAllByText('casey@example.com')).toHaveLength(2)
    expect(screen.getByText('Not yet verified')).toBeTruthy()
    expect(screen.getByDisplayValue('123456')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Verify email' })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Send again/i })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Continue' })).toBeNull()

    mockUseOnboardingContext.mockReturnValue({ emailVerified: true })
    rerender(<VerifyEmailPage />)

    expect(screen.getByText('Email verified')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Continue' })).toBeTruthy()
    expect(screen.queryByText('Not yet verified')).toBeNull()
    expect(screen.queryByDisplayValue('123456')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Verify email' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Send again/i })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Send code' })).toBeNull()
    expect(screen.queryByText('Use a different email')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    expect(mockNavigate).toHaveBeenCalledWith({ path: '../' })
  })
})

function makeEmailManagement(
  overrides: Partial<EmailManagement> = {},
): EmailManagement {
  return {
    emails: [],
    loading: false,
    verifyingEmail: null,
    setVerifyingEmail: vi.fn(),
    code: '',
    setCode: vi.fn(),
    retryAfter: 0,
    sendingCode: null,
    verifyingCode: false,
    addingEmail: false,
    removingEmail: null,
    settingPrimary: null,
    sendCode: vi.fn(),
    verifyCode: vi.fn(),
    addEmail: vi.fn(),
    removeEmail: vi.fn(),
    setPrimaryEmail: vi.fn(),
    ...overrides,
  }
}

function makeEmail({
  email,
  verified,
}: {
  email: string
  verified: boolean
}): EmailInfo {
  return {
    email,
    verified,
    primary: true,
  }
}
