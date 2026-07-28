import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { DocPage } from './types.js'
import { DocsSearch } from './DocsSearch.js'

const docs: DocPage[] = [
  {
    slug: 'cli-reference',
    url: '/docs/developers/cli/cli-reference',
    title: 'CLI Reference',
    site: 'developers',
    section: 'cli',
    order: 1,
    summary: 'Reference the command tree.',
    body: 'The auth backup command writes a recovery key.',
    filename: '01-cli-reference.md',
  },
  {
    slug: 'backup-and-recovery',
    url: '/docs/self-hosters/storage/backup-and-recovery',
    title: 'Backup and Recovery',
    site: 'self-hosters',
    section: 'storage',
    order: 2,
    summary: 'Recovery paths for stored data.',
    body: 'Keep the recovery material somewhere safe.',
    filename: '02-backup-and-recovery.md',
  },
]

describe('DocsSearch', () => {
  afterEach(() => {
    cleanup()
  })

  it('ranks a title match above a body-only match through the search input', async () => {
    const user = userEvent.setup()
    render(<DocsSearch docs={docs} onSelect={() => {}} />)

    await user.type(screen.getByPlaceholderText('Search docs…'), 'backup')

    const results = screen.getAllByRole('button')
    expect(results).toHaveLength(2)
    expect(results[0].textContent).toContain('Backup and Recovery')
    expect(results[1].textContent).toContain('CLI Reference')
  })

  it('selects a result from keyboard focus', async () => {
    const user = userEvent.setup()
    let selected: DocPage | undefined
    render(<DocsSearch docs={docs} onSelect={(doc) => (selected = doc)} />)

    await user.type(screen.getByPlaceholderText('Search docs…'), 'backup')
    await user.tab()
    await user.keyboard('{Enter}')

    expect(selected?.title).toBe('Backup and Recovery')
  })
})
