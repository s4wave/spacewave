import { describe, expect, it } from 'vitest'

import { buildPageHtml } from './html-template.js'
import {
  ROOT_BOOT_VISIBILITY_CSS,
  ROOT_BOOT_VISIBILITY_SCRIPT,
} from './root-loading-shell.js'

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
      canonicalUrl: 'https://spacewave.app/',
      ogImage: 'https://spacewave.app/static/assets/og.png',
    })

    expect(html).toContain('<meta name="darkreader-lock"/>')
    expect(html).toContain('<link rel="icon" href="/static/assets/icon.png"/>')
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
      canonicalUrl: 'https://spacewave.app/',
      ogImage: 'https://spacewave.app/static/assets/og.png',
    })

    expect(html).toContain(
      '<link rel="stylesheet" href="/static/BlogRoutes-def.css"/>',
    )
    expect(html.indexOf('/static/App-abc.css')).toBeLessThan(
      html.indexOf('/static/BlogRoutes-def.css'),
    )
  })

  it('selects the root loading shell in the head before stylesheets load', () => {
    const html = buildPageHtml({
      body: `<div id="sw-landing">Landing</div>
        <div id="sw-loading" style="display:none">Loading</div>`,
      title: 'Spacewave',
      description: 'Spacewave direct route',
      headScript: ROOT_BOOT_VISIBILITY_SCRIPT,
      bootstrapScript: '<script type="module" src="/boot.mjs"></script>',
      criticalCss: ROOT_BOOT_VISIBILITY_CSS,
      mainCssUrl: '/static/app.css',
      iconUrl: '/static/assets/icon.png',
      canonicalUrl: 'https://spacewave.app/',
      ogImage: 'https://spacewave.app/static/assets/og.png',
    })

    const headEnd = html.indexOf('</head>')
    const decision = html.indexOf(ROOT_BOOT_VISIBILITY_SCRIPT)
    const visibilityCss = html.indexOf(ROOT_BOOT_VISIBILITY_CSS)
    const stylesheet = html.indexOf('<link rel="stylesheet"')

    expect(decision).toBeGreaterThan(-1)
    expect(decision).toBeLessThan(headEnd)
    expect(visibilityCss).toBeGreaterThan(decision)
    expect(visibilityCss).toBeLessThan(headEnd)
    expect(stylesheet).toBeGreaterThan(visibilityCss)
  })

  it('escapes page text in head attributes', () => {
    const html = buildPageHtml({
      body: '<main>Tag</main>',
      title: '"a&b" posts',
      description: 'Blog posts tagged "<a&b>".',
      bootstrapScript: '<script type="module" src="/boot.mjs"></script>',
      criticalCss: '',
      mainCssUrl: '/static/app.css',
      iconUrl: '/static/assets/icon.png',
      canonicalUrl: 'https://spacewave.app/blog/tag/a&b',
      ogImage: 'https://spacewave.app/static/assets/og.png',
      robots: 'noindex, follow',
    })

    expect(html).toContain('<title>&quot;a&amp;b&quot; posts</title>')
    expect(html).toContain(
      '<meta name="description" content="Blog posts tagged &quot;&lt;a&amp;b&gt;&quot;."/>',
    )
    expect(html).toContain(
      '<link rel="canonical" href="https://spacewave.app/blog/tag/a&amp;b"/>',
    )
    expect(html).toContain('<meta name="robots" content="noindex, follow"/>')
  })
})
