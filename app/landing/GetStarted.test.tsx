import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'

import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import GetStarted from './GetStarted.js'

const mockUseSessionMetadata = vi.hoisted(() => vi.fn())
const mockAddSpaceRootAlias = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseIsStaticMode = vi.hoisted(() => vi.fn(() => false))

declare global {
  var __swStaticHandoffLinks: boolean | undefined
}

vi.mock('@s4wave/app/hooks/useAddSpaceRootAlias.js', () => ({
  useAddSpaceRootAlias: () => ({
    add: mockAddSpaceRootAlias,
    adding: false,
    canAdd: true,
  }),
}))

vi.mock('@s4wave/app/hooks/useSessionMetadata.js', () => ({
  useSessionMetadata: mockUseSessionMetadata,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('../prerender/StaticContext.js', () => ({
  useIsStaticMode: mockUseIsStaticMode,
}))

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...values: Array<string | false | null | undefined>) =>
    values.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/web/ui/command.js', () => ({
  Command: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CommandEmpty: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CommandGroup: ({
    heading,
    children,
  }: {
    heading?: React.ReactNode
    children: React.ReactNode
  }) => (
    <section>
      {heading}
      {children}
    </section>
  ),
  CommandInput: ({
    ref,
    ...props
  }: {
    placeholder?: string
    onKeyDown?: (e: React.KeyboardEvent<HTMLInputElement>) => void
    ref?: React.Ref<HTMLInputElement>
  }) => {
    return (
      <input
        ref={ref}
        placeholder={props.placeholder}
        onKeyDown={props.onKeyDown}
      />
    )
  },
  CommandItem: ({
    children,
    disabled,
    onSelect,
  }: {
    children: React.ReactNode
    disabled?: boolean
    onSelect?: () => void
  }) => (
    <button disabled={disabled} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
  CommandList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
  vi.clearAllMocks()
  mockUseIsStaticMode.mockReturnValue(false)
  globalThis.__swStaticHandoffLinks = undefined
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('GetStarted', () => {
  it('shows account-facing labels for existing sessions', () => {
    mockUseSessionMetadata.mockImplementation((sessionIdx: number | null) => {
      if (sessionIdx === 1) {
        return {
          displayName: 'Casey',
          cloudEntityId: 'casey',
          providerDisplayName: 'Cloud',
          providerId: 'spacewave',
        }
      }
      if (sessionIdx === 2) {
        return {
          cloudEntityId: 'second-user',
          providerDisplayName: 'Cloud',
          providerId: 'spacewave',
        }
      }
      return null
    })

    render(
      <GetStarted
        sessions={[
          {
            sessionIndex: 1,
            sessionRef: {
              providerResourceRef: {
                providerAccountId: 'acct-1',
              },
            },
          },
          {
            sessionIndex: 2,
            sessionRef: {
              providerResourceRef: {
                providerAccountId: 'acct-2',
              },
            },
          },
        ]}
      />,
    )

    expect(screen.getByText('Account: Casey')).toBeTruthy()
    expect(screen.getByText('Cloud · casey')).toBeTruthy()
    expect(screen.getByText('Account: second-user')).toBeTruthy()
    expect(screen.queryByText('Session 1')).toBeNull()
    expect(screen.queryByText('Session 2')).toBeNull()
  })

  it('keeps prerendered quickstart links crawlable before browser handoff', () => {
    mockUseIsStaticMode.mockReturnValue(true)

    const html = renderToString(<GetStarted />)

    expect(html).toContain('href="#/login"')
    expect(html).toContain('href="/quickstart/drive"')
  })

  it('uses hash links for static quickstart routes once boot handoff is active', () => {
    mockUseIsStaticMode.mockReturnValue(true)
    globalThis.__swStaticHandoffLinks = true

    render(<GetStarted />)

    expect(
      screen
        .getByRole('link', { name: /sign in or create account/i })
        .getAttribute('href'),
    ).toBe('#/login')
    expect(
      screen
        .getByRole('link', { name: /create a drive/i })
        .getAttribute('href'),
    ).toBe('#/quickstart/drive')
  })

  it('adds a local state root action to the interactive quickstart list', () => {
    render(<GetStarted />)

    screen.getByRole('button', { name: /open a local state root/i }).click()

    expect(mockAddSpaceRootAlias).toHaveBeenCalledTimes(1)
  })

  it('uses the runtime preference for interactive experimental quickstarts', () => {
    vi.stubEnv('DEV', false)
    render(<GetStarted />)

    expect(screen.queryByRole('button', { name: /add a device/i })).toBeNull()

    cleanup()
    setExperimentalCreatorsEnabled(true)
    render(<GetStarted />)

    expect(screen.getByRole('button', { name: /add a device/i })).toBeTruthy()
  })

  it('keeps static quickstart links on the build-time public inventory', () => {
    vi.stubEnv('DEV', false)
    mockUseIsStaticMode.mockReturnValue(true)
    setExperimentalCreatorsEnabled(true)

    render(<GetStarted />)

    expect(screen.queryByRole('link', { name: /add a device/i })).toBeNull()
    expect(screen.getByRole('link', { name: /create a drive/i })).toBeTruthy()
  })

  it('keeps the public storage quickstart ordering stable', () => {
    vi.stubEnv('DEV', false)
    mockUseIsStaticMode.mockReturnValue(true)

    render(<GetStarted />)

    const storageLinks = screen
      .getAllByRole('link')
      .filter((link) =>
        [
          'Create an Empty Space',
          'Create a Drive',
          'Create/clone a Git Repository',
          'Create a Canvas',
        ].some((label) => link.textContent?.includes(label)),
      )

    expect(storageLinks.map((link) => link.textContent)).toEqual([
      'Create an Empty SpaceStart with a blank space',
      'Create a DrivePrivate browser files with offline work and device sync',
      'Create/clone a Git RepositoryStart fresh or clone an existing Git repository',
      'Create a CanvasVisual workspace with objects on a canvas',
    ])
  })

  it('places the local state root action third in the account group', () => {
    render(<GetStarted />)

    const buttons = screen.getAllByRole('button')
    expect(buttons[0].textContent).toContain('Sign in or create account')
    expect(buttons[1].textContent).toContain('Enter a device pairing code')
    expect(buttons[2].textContent).toContain('Open a local state root')
  })
})
