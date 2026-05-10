import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import Markdown from 'markdown-to-jsx'

import { blogMarkdownOptions } from './BlogMarkdown.js'

declare global {
  interface Window {
    happyDOM: {
      settings: {
        navigation: {
          disableChildFrameNavigation: boolean
          disableFallbackToSetURL: boolean
        }
      }
    }
  }
}

const crossOriginIsolatedDescriptor = Object.getOwnPropertyDescriptor(
  window,
  'crossOriginIsolated',
)
const iframeCredentiallessDescriptor = Object.getOwnPropertyDescriptor(
  window.HTMLIFrameElement.prototype,
  'credentialless',
)
const happyDOMNavigation = window.happyDOM.settings.navigation
const disableChildFrameNavigation =
  happyDOMNavigation.disableChildFrameNavigation
const disableFallbackToSetURL = happyDOMNavigation.disableFallbackToSetURL

function restoreDescriptor(
  object: object,
  key: string,
  descriptor: PropertyDescriptor | undefined,
) {
  if (descriptor) {
    Object.defineProperty(object, key, descriptor)
    return
  }

  Reflect.deleteProperty(object, key)
}

function setCrossOriginIsolated(value: boolean) {
  Object.defineProperty(window, 'crossOriginIsolated', {
    configurable: true,
    value,
  })
}

function setIframeCredentiallessSupported(value: boolean) {
  if (!value) {
    Reflect.deleteProperty(window.HTMLIFrameElement.prototype, 'credentialless')
    return
  }

  Object.defineProperty(window.HTMLIFrameElement.prototype, 'credentialless', {
    configurable: true,
    value: false,
  })
}

describe('blogMarkdownOptions', () => {
  beforeEach(() => {
    happyDOMNavigation.disableChildFrameNavigation = true
    happyDOMNavigation.disableFallbackToSetURL = true
  })

  afterEach(() => {
    cleanup()
    happyDOMNavigation.disableChildFrameNavigation = disableChildFrameNavigation
    happyDOMNavigation.disableFallbackToSetURL = disableFallbackToSetURL
    restoreDescriptor(
      window,
      'crossOriginIsolated',
      crossOriginIsolatedDescriptor,
    )
    restoreDescriptor(
      window.HTMLIFrameElement.prototype,
      'credentialless',
      iframeCredentiallessDescriptor,
    )
  })

  it('renders yt-embed tags as a static fallback before browser detection', () => {
    const html = renderToStaticMarkup(
      <Markdown options={blogMarkdownOptions}>
        {'<yt-embed videoid="bof8TkZkr1I"></yt-embed>'}
      </Markdown>,
    )

    expect(html).toContain('https://www.youtube.com/watch?v=bof8TkZkr1I')
    expect(html).toContain('Watch on YouTube')
    expect(html).not.toContain('<iframe')
    expect(html).not.toContain('<yt-embed')
  })

  it('upgrades to a youtube iframe outside cross-origin isolation', async () => {
    setCrossOriginIsolated(false)
    setIframeCredentiallessSupported(false)

    render(
      <Markdown options={blogMarkdownOptions}>
        {'<yt-embed videoid="bof8TkZkr1I"></yt-embed>'}
      </Markdown>,
    )

    const iframe = await screen.findByTitle('YouTube video bof8TkZkr1I')

    expect(iframe.getAttribute('src')).toBe(
      'https://www.youtube-nocookie.com/embed/bof8TkZkr1I?rel=0',
    )
    expect(iframe.hasAttribute('credentialless')).toBe(false)
  })

  it('keeps the fallback when an isolated browser lacks iframe credentialless', async () => {
    setCrossOriginIsolated(true)
    setIframeCredentiallessSupported(false)

    render(
      <Markdown options={blogMarkdownOptions}>
        {'<yt-embed videoid="bof8TkZkr1I"></yt-embed>'}
      </Markdown>,
    )

    await waitFor(() => {
      expect(screen.queryByTitle('YouTube video bof8TkZkr1I')).toBeNull()
    })
    expect(
      screen
        .getByRole('link', { name: /watch on youtube/i })
        .getAttribute('href'),
    ).toBe('https://www.youtube.com/watch?v=bof8TkZkr1I')
  })

  it('adds iframe credentialless for isolated browsers that support it', async () => {
    setCrossOriginIsolated(true)
    setIframeCredentiallessSupported(true)

    render(
      <Markdown options={blogMarkdownOptions}>
        {'<yt-embed videoid="bof8TkZkr1I"></yt-embed>'}
      </Markdown>,
    )

    const iframe = await screen.findByTitle('YouTube video bof8TkZkr1I')

    expect(iframe.getAttribute('src')).toBe(
      'https://www.youtube-nocookie.com/embed/bof8TkZkr1I?rel=0',
    )
    expect(iframe.hasAttribute('credentialless')).toBe(true)
  })
})
