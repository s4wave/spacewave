import { describe, expect, it } from 'vitest'

import { buildPageHtml } from './html-template.js'

describe('buildPageHtml', () => {
  it('adds the Dark Reader lock to prerendered documents', () => {
    const html = buildPageHtml({
      body: '<main>Spacewave</main>',
      title: 'Spacewave',
      description: 'Spacewave public page',
      bootstrapScript: '<script type="module" src="/boot.mjs"></script>',
      criticalCss: '',
      mainCssUrl: '/static/app.css',
      iconUrl: '/static/assets/icon.png',
    })

    expect(html).toContain('<meta name="darkreader-lock"/>')
    expect(html.indexOf('<meta name="darkreader-lock"/>')).toBeLessThan(
      html.indexOf('<title>Spacewave</title>'),
    )
  })

  it('links route stylesheets after the app stylesheet', () => {
    const html = buildPageHtml({
      body: '<article>Post</article>',
      title: 'Post',
      description: 'Blog post',
      bootstrapScript: '<script type="module" src="/boot.mjs"></script>',
      criticalCss: '',
      mainCssUrl: '/static/App-abc.css',
      additionalCssUrls: ['/static/BlogRoutes-def.css'],
      iconUrl: '/static/assets/icon.png',
    })

    expect(html).toContain(
      '<link rel="stylesheet" href="/static/BlogRoutes-def.css"/>',
    )
    expect(html.indexOf('/static/App-abc.css')).toBeLessThan(
      html.indexOf('/static/BlogRoutes-def.css'),
    )
  })
})
