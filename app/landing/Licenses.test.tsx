import type { ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { licenseEntries } from '@s4wave/app/licenses/data.js'

import { Licenses } from './Licenses.js'

// findMultiVersionPackage returns a package name present at two or more distinct
// versions, derived from the generated license data so the duplicate-key
// invariant test does not pin floating dependency versions.
function findMultiVersionPackage(): { name: string; versions: string[] } {
  const versionsByName = new Map<string, Set<string>>()
  for (const entry of licenseEntries) {
    const versions = versionsByName.get(entry.name) ?? new Set<string>()
    versions.add(entry.version)
    versionsByName.set(entry.name, versions)
  }
  for (const [name, versions] of versionsByName) {
    if (versions.size >= 2) {
      return { name, versions: [...versions] }
    }
  }
  throw new Error('no package renders at multiple versions in license data')
}

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

    const { name, versions } = findMultiVersionPackage()
    const entryCount = licenseEntries.filter((e) => e.name === name).length

    render(<Licenses />)

    expect(screen.getAllByText(name)).toHaveLength(entryCount)
    for (const version of versions) {
      expect(screen.getAllByText(version).length).toBeGreaterThanOrEqual(1)
    }

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
        name: 'Show details for @radix-ui/react-slot 1.2.3',
      }),
    )

    expect(screen.getAllByText('Copyright (c) 2022 WorkOS')).toHaveLength(1)
  })
})
