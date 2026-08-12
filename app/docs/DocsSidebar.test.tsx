import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const navigate = vi.fn()

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => navigate,
}))

import { DocsSidebar } from './DocsSidebar.js'
import type { DocPage, DocSection } from './types.js'

const currentDoc: DocPage = {
  slug: 'create-your-first-space',
  url: '/docs/users/start/create-your-first-space',
  title: 'Create Your First Space',
  site: 'users',
  section: 'start',
  order: 1,
  summary: 'Create a Space.',
  body: 'Body',
  filename: '01-create-your-first-space.md',
}

const nextDoc: DocPage = {
  ...currentDoc,
  slug: 'invite-collaborators',
  url: '/docs/users/start/invite-collaborators',
  title: 'Invite Collaborators',
  order: 2,
  filename: '02-invite-collaborators.md',
}

const sections: DocSection[] = [
  {
    id: 'start',
    label: 'Start',
    site: 'users',
    order: 1,
    pages: [currentDoc, nextDoc],
  },
]

describe('DocsSidebar', () => {
  afterEach(cleanup)

  it('marks the current page and navigates from its button', async () => {
    const user = userEvent.setup()
    navigate.mockReset()
    render(<DocsSidebar sections={sections} currentDoc={currentDoc} />)

    expect(
      screen
        .getByRole('button', { name: currentDoc.title })
        .getAttribute('aria-current'),
    ).toBe('page')
    expect(
      screen
        .getByRole('button', { name: nextDoc.title })
        .getAttribute('aria-current'),
    ).toBeNull()

    await user.click(screen.getByRole('button', { name: nextDoc.title }))
    expect(navigate).toHaveBeenCalledWith({ path: nextDoc.url })
  })
})
