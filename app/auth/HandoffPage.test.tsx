import type { ReactNode } from 'react'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { LoginResult } from '@s4wave/web/ui/login-form.js'

import { HandoffPage } from './HandoffPage.js'

interface LoginFormProps {
  initialUsername?: string
  onLoginWithPassword: (
    username: string,
    password: string,
    turnstileToken: string,
  ) => Promise<LoginResult>
  onCreateAccountWithPassword: (
    username: string,
    password: string,
    turnstileToken: string,
  ) => Promise<{ sessionIndex: number }>
}

const mocks = vi.hoisted(() => ({
  createAccount: vi.fn(),
  encryptForHandoffViaSession: vi.fn(),
  loginAccount: vi.fn(),
  loginFormProps: undefined as LoginFormProps | undefined,
  navigate: vi.fn(),
  root: undefined as
    | {
        lookupProvider: ReturnType<typeof vi.fn>
      }
    | undefined,
  setStoredHandoffPayload: vi.fn(),
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mocks.navigate,
  useParams: () => ({ payload: 'payload-123' }),
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mocks.root,
}))

vi.mock('@s4wave/app/provider/spacewave/useSpacewaveAuth.js', () => ({
  useCloudProviderConfig: () => null,
}))

vi.mock('@s4wave/sdk/provider/spacewave/spacewave.js', () => ({
  SpacewaveProvider: class MockSpacewaveProvider {
    createAccount = mocks.createAccount
    loginAccount = mocks.loginAccount
  },
}))

vi.mock('./handoff-state.js', () => ({
  decodeHandoffRequest: () => ({
    clientType: 'cli',
    deviceName: 'Terminal',
    devicePublicKey: new Uint8Array([1, 2, 3]),
    sessionNonce: 'nonce-1',
  }),
  encryptForHandoffViaSession: mocks.encryptForHandoffViaSession,
  setStoredHandoffPayload: mocks.setStoredHandoffPayload,
}))

vi.mock('@s4wave/web/ui/login-form.js', () => ({
  LoginForm: (props: LoginFormProps) => {
    mocks.loginFormProps = props
    return (
      <div
        data-testid="login-form"
        data-initial-username={props.initialUsername ?? ''}
      >
        login-form
      </div>
    )
  },
}))

vi.mock('@s4wave/app/auth/AuthScreenLayout.js', () => ({
  AuthScreenLayout: ({
    intro,
    children,
  }: {
    intro: ReactNode
    children: ReactNode
  }) => (
    <div>
      <div>{intro}</div>
      <div>{children}</div>
    </div>
  ),
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div>logo</div>,
}))

describe('HandoffPage', () => {
  let releaseProvider: ReturnType<typeof vi.fn>

  beforeEach(() => {
    cleanup()
    window.location.hash =
      '#/auth/link/payload-123?intent=signup&username=Spacewave'
    releaseProvider = vi.fn()
    mocks.root = {
      lookupProvider: vi.fn().mockResolvedValue({
        resourceRef: {},
        [Symbol.dispose]: releaseProvider,
      }),
    }
    mocks.loginFormProps = undefined
    mocks.navigate.mockReset()
    mocks.createAccount.mockReset()
    mocks.loginAccount.mockReset()
    mocks.encryptForHandoffViaSession.mockReset()
    mocks.setStoredHandoffPayload.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('prefills signup username from the handoff hash query', () => {
    render(<HandoffPage />)

    expect(screen.getByText('Creating a Spacewave CLI account')).toBeDefined()
    expect(screen.getByText('spacewave')).toBeDefined()
    expect(
      screen.getByTestId('login-form').getAttribute('data-initial-username'),
    ).toBe('spacewave')
  })

  it('renders completing and complete states while handing off a session', async () => {
    let finishEncryption: (() => void) | undefined
    mocks.loginAccount.mockResolvedValue({
      result: { case: 'session', value: { sessionIndex: 4 } },
    })
    mocks.encryptForHandoffViaSession.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishEncryption = resolve
        }),
    )
    render(<HandoffPage />)

    const loginPromise = mocks.loginFormProps?.onLoginWithPassword(
      'casey',
      'password',
      'turnstile-token',
    )
    expect(loginPromise).toBeDefined()
    await waitFor(() => {
      expect(screen.getByText('Completing sign-in…')).toBeDefined()
    })
    expect(
      screen.getByText('Sending credentials to Spacewave CLI.'),
    ).toBeDefined()
    expect(releaseProvider).not.toHaveBeenCalled()

    finishEncryption?.()
    await act(async () => await loginPromise)

    expect(screen.getByText('Sign-in complete')).toBeDefined()
    expect(
      screen.getByText('You can close this tab and return to Spacewave CLI.'),
    ).toBeDefined()
    expect(screen.getByText('Device: Terminal')).toBeDefined()
    expect(mocks.encryptForHandoffViaSession).toHaveBeenCalledWith(
      mocks.root,
      4,
      new Uint8Array([1, 2, 3]),
      'nonce-1',
    )
    await expect(loginPromise).resolves.toEqual({
      type: 'session',
      sessionIndex: 4,
    })
    expect(releaseProvider).toHaveBeenCalledOnce()
  })

  it('returns to retryable auth and retains a login encryption error', async () => {
    const error = new Error('encryption unavailable')
    mocks.loginAccount.mockResolvedValue({
      result: { case: 'session', value: { sessionIndex: 4 } },
    })
    mocks.encryptForHandoffViaSession.mockRejectedValue(error)
    render(<HandoffPage />)

    let rejection: unknown
    await act(async () => {
      try {
        await mocks.loginFormProps?.onLoginWithPassword(
          'casey',
          'password',
          'turnstile-token',
        )
      } catch (caught) {
        rejection = caught
      }
    })

    expect(rejection).toBe(error)
    expect(screen.getByTestId('login-form')).toBeDefined()
    expect(screen.getByRole('alert').textContent).toBe('encryption unavailable')
    expect(screen.queryByText('Completing sign-in…')).toBeNull()
    expect(releaseProvider).toHaveBeenCalledOnce()
  })

  it('returns to retryable auth and retains an account-creation encryption error', async () => {
    const error = new Error('encryption unavailable')
    mocks.createAccount.mockResolvedValue({
      sessionListEntry: { sessionIndex: 6 },
    })
    mocks.encryptForHandoffViaSession.mockRejectedValue(error)
    render(<HandoffPage />)

    let rejection: unknown
    await act(async () => {
      try {
        await mocks.loginFormProps?.onCreateAccountWithPassword(
          'casey',
          'password',
          'turnstile-token',
        )
      } catch (caught) {
        rejection = caught
      }
    })

    expect(rejection).toBe(error)
    expect(screen.getByTestId('login-form')).toBeDefined()
    expect(screen.getByRole('alert').textContent).toBe('encryption unavailable')
    expect(screen.queryByText('Completing sign-in…')).toBeNull()
    expect(releaseProvider).toHaveBeenCalledOnce()
  })

  it('keeps the auth state and releases the provider when login fails', async () => {
    const error = new Error('login unavailable')
    mocks.loginAccount.mockRejectedValue(error)
    render(<HandoffPage />)

    await expect(
      mocks.loginFormProps?.onLoginWithPassword(
        'casey',
        'password',
        'turnstile-token',
      ),
    ).rejects.toBe(error)

    expect(screen.getByTestId('login-form')).toBeDefined()
    expect(screen.queryByText('Completing sign-in…')).toBeNull()
    expect(releaseProvider).toHaveBeenCalledOnce()
  })
})
