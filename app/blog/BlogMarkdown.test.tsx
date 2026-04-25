import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import Markdown from 'markdown-to-jsx'

import { blogMarkdownOptions } from './BlogMarkdown.js'

describe('blogMarkdownOptions', () => {
  it('renders yt-embed tags as youtube iframes', () => {
    const html = renderToStaticMarkup(
      <Markdown options={blogMarkdownOptions}>
        {'<yt-embed videoid="bof8TkZkr1I"></yt-embed>'}
      </Markdown>,
    )

    expect(html).toContain('youtube-nocookie.com/embed/bof8TkZkr1I?rel=0')
    expect(html).toContain('<iframe')
    expect(html).not.toContain('<yt-embed')
  })
})
