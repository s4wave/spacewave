import type { ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { Licenses } from './Licenses.js'

vi.mock('./LegalPageLayout.js', () => ({
  LegalPageLayout: ({
    title,
    subtitle,
    children,
  }: {
    title: string
    subtitle?: string
    children: ReactNode
  }) => (
    <div>
      <h1>{title}</h1>
      {subtitle && <p>{subtitle}</p>}
      {children}
    </div>
  ),
}))

describe('Licenses', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders duplicate package names without duplicate React keys', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    render(<Licenses />)

    expect(screen.getAllByText('@radix-ui/react-primitive')).toHaveLength(2)
    expect(screen.getByText('2.1.3')).toBeTruthy()
    expect(screen.getByText('2.1.4')).toBeTruthy()

    const duplicateKeyWarnings = errorSpy.mock.calls.filter(
      ([message]) =>
        typeof message === 'string' &&
        message.includes('Encountered two children with the same key'),
    )
    expect(duplicateKeyWarnings).toHaveLength(0)
  })

  it('tracks disclosure state per package version', () => {
    render(<Licenses />)

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Show details for @radix-ui/react-primitive 2.1.3',
      }),
    )

    expect(screen.getAllByText('Copyright (c) 2022 WorkOS')).toHaveLength(1)
  })
})
