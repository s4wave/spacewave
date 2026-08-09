import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { HeroCard } from './HeroCard.js'
import { PostList } from './PostList.js'
import type { BlogPost } from './types.js'

const navigate = vi.fn()

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => navigate,
}))

const post: BlogPost = {
  slug: 'keyboard-boundaries',
  url: '/blog/2026/05/keyboard-boundaries',
  title: 'Keyboard boundaries',
  date: '2026-05-18',
  author: {
    name: 'Christian Stewart',
    avatar: '/christian-stewart.png',
    url: 'https://github.com/paralin',
    bio: '',
  },
  authorSlug: 'paralin',
  summary: 'Interactive controls have separate destinations.',
  tags: ['accessibility', 'keyboard'],
  draft: false,
  body: '',
}

function dispatchClick(
  link: HTMLElement,
  init: MouseEventInit = {},
): MouseEvent {
  const event = new MouseEvent('click', {
    bubbles: true,
    cancelable: true,
    button: 0,
    ...init,
  })
  link.dispatchEvent(event)
  return event
}

describe.each([
  ['HeroCard', () => render(<HeroCard post={post} />)],
  ['PostList', () => render(<PostList posts={[post]} />)],
])('%s post link', (_name, renderCard) => {
  afterEach(() => {
    cleanup()
    navigate.mockReset()
  })

  it('prevents the native default only for unmodified primary activation', () => {
    renderCard()
    const link = screen.getByRole('link', { name: post.title })

    const primary = dispatchClick(link)
    expect(primary.defaultPrevented).toBe(true)
    expect(navigate).toHaveBeenCalledWith({ path: post.url })

    for (const init of [
      { metaKey: true },
      { ctrlKey: true },
      { shiftKey: true },
      { altKey: true },
      { button: 1 },
      { button: 2 },
    ]) {
      navigate.mockReset()
      const nativeClick = dispatchClick(link, init)
      expect(nativeClick.defaultPrevented).toBe(false)
      expect(navigate).not.toHaveBeenCalled()
    }
  })
})

describe('blog card control boundaries', () => {
  afterEach(() => {
    cleanup()
    navigate.mockReset()
  })

  it('keeps the author keyboard action separate from the featured post link', async () => {
    const user = userEvent.setup()
    const { container } = render(<HeroCard post={post} />)

    const authorLink = screen.getByRole('link', { name: post.author.name })
    expect(authorLink.classList).toContain('pointer-events-auto')
    expect(authorLink.closest('.pointer-events-none')).not.toBeNull()
    authorLink.focus()
    await user.keyboard('{Enter}')
    expect(navigate).not.toHaveBeenCalled()
    expect(container.querySelector('article[role="link"]')).toBeNull()

    const postLink = screen.getByRole('link', { name: post.title })
    postLink.focus()
    await user.keyboard('{Enter}')
    expect(navigate).toHaveBeenCalledWith({ path: post.url })
  })

  it('leaves tag-wrapper gaps transparent while tag controls stay independent', async () => {
    const user = userEvent.setup()
    const { container } = render(<PostList posts={[post]} />)
    const article = container.querySelector('article')
    expect(article).not.toBeNull()

    for (const tag of post.tags) {
      navigate.mockReset()
      const tagButton = screen.getByRole('button', { name: tag })
      expect(tagButton.classList).toContain('pointer-events-auto')
      expect(tagButton.parentElement?.classList).toContain(
        'pointer-events-none',
      )
      tagButton.focus()
      await user.keyboard('{Enter}')
      expect(navigate).toHaveBeenCalledTimes(1)
      expect(navigate).toHaveBeenCalledWith({ path: `/blog/tag/${tag}` })
    }

    expect(container.querySelector('article[role="link"]')).toBeNull()
    expect(
      within(article as HTMLElement).getByRole('link', { name: post.title }),
    ).toBeDefined()
  })
})
