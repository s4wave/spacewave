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
})
