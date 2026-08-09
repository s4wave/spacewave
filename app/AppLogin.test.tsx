import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AppLogin } from './AppLogin.js'

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  listSessions: vi.fn(),
}))

let navigateToSession:
  | ((sessionIndex: number, isNew: boolean) => void | Promise<void>)
  | undefined

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mocks.navigate,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => ({ listSessions: mocks.listSessions }),
}))

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: () => ({ data: undefined, loading: false }),
}))

vi.mock('@s4wave/app/hooks/useAddSpaceRootAlias.js', () => ({
  useAddSpaceRootAlias: () => ({ add: vi.fn(), canAdd: false }),
}))

vi.mock('@s4wave/app/provider/spacewave/useSpacewaveAuth.js', () => ({
  useSpacewaveAuth: () => ({
    cloudProviderConfig: null,
  }),
}))

vi.mock('@s4wave/web/ui/login-form.js', () => ({
  LoginForm: (props: {
    onNavigateToSession?: (
      sessionIndex: number,
      isNew: boolean,
    ) => void | Promise<void>
  }) => {
    navigateToSession = props.onNavigateToSession
    return null
  },
}))

vi.mock('@s4wave/app/auth/AuthScreenLayout.js', () => ({
  AuthScreenLayout: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => null,
}))

vi.mock('@s4wave/web/ui/BackButton.js', () => ({
  BackButton: () => null,
}))

describe('AppLogin', () => {
  beforeEach(() => {
    navigateToSession = undefined
    mocks.navigate.mockReset()
    mocks.listSessions.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('reports session lookup failure to the authentication caller', async () => {
    const lookupError = new Error('session lookup failed')
    mocks.listSessions.mockRejectedValue(lookupError)
    render(<AppLogin />)

    const completion = navigateToSession?.(2, false)

    expect(completion).toBeInstanceOf(Promise)
    await expect(completion).rejects.toBe(lookupError)
    expect(mocks.navigate).not.toHaveBeenCalled()
  })
})
