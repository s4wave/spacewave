import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RouterProvider } from '@s4wave/web/router/router.js'

import { authors } from './authors.js'
import { BlogPostPage } from './BlogPost.js'
import type { BlogPost } from './types.js'

vi.mock('./BlogCta.js', () => ({
  BlogCta: function BlogCta() {
    return null
  },
}))

const post: BlogPost = {
  slug: 'codeql-test',
  url: '/blog/2026/05/codeql-test',
  title: 'CodeQL Test',
  date: '2026-05-18',
  author: {
    name: 'Spacewave',
    avatar: '',
    url: 'https://spacewave.app',
    bio: '',
  },
  authorSlug: 'spacewave',
  summary: 'summary',
  tags: [],
  draft: false,
  body: 'Rendered markdown body',
}

function renderPost(p: BlogPost) {
  return render(
    <RouterProvider path={p.url} onNavigate={() => {}}>
      <BlogPostPage post={p} />
    </RouterProvider>,
  )
}

describe('BlogPostPage', () => {
  afterEach(() => cleanup())

  it('renders post markdown without accepting pre-rendered HTML input', () => {
    const { container } = renderPost({
      ...post,
      body: '**trusted markdown**',
    })

    expect(screen.getByText('trusted markdown').tagName).toBe('STRONG')
    expect(container.innerHTML).not.toContain('dangerouslySetInnerHTML')
  })

  it('uses the bundled same-origin author avatar', () => {
    renderPost({
      ...post,
      author: authors.paralin,
    })

    const src = screen
      .getByRole('img', { name: 'Christian Stewart' })
      .getAttribute('src')
    expect(src).toBe(authors.paralin.avatar)
    expect(src).toMatch(/christian-stewart\.png$/)
    expect(src).not.toMatch(/^https?:/)
  })
})
