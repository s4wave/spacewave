import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import GetStarted from './GetStarted.js'

const mockUseSessionMetadata = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseIsStaticMode = vi.hoisted(() => vi.fn(() => false))

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
  CommandInput: React.forwardRef<
    HTMLInputElement,
    {
      placeholder?: string
      onKeyDown?: (e: React.KeyboardEvent<HTMLInputElement>) => void
    }
  >(function CommandInputMock(props, ref) {
    return (
      <input
        ref={ref}
        placeholder={props.placeholder}
        onKeyDown={props.onKeyDown}
      />
    )
  }),
  CommandItem: ({
    children,
    onSelect,
  }: {
    children: React.ReactNode
    onSelect?: () => void
  }) => <button onClick={() => onSelect?.()}>{children}</button>,
  CommandList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  mockUseIsStaticMode.mockReturnValue(false)
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

  it('uses hash links for non-prerendered static quickstart routes', () => {
    mockUseIsStaticMode.mockReturnValue(true)

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
    ).toBe('/quickstart/drive')
  })
})
