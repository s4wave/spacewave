import { describe, expect, it } from 'vitest'

import { getPublicQuickstartOptions } from '@s4wave/app/quickstart/options.js'
import { getMetadata } from './metadata.js'
import { STATIC_PAGES } from './static-pages.js'
import { buildQuickstartStaticPages } from './static-pages.js'

describe('buildQuickstartStaticPages', () => {
  it('omits experimental quickstart pages from the release inventory', () => {
    const pages = buildQuickstartStaticPages(getPublicQuickstartOptions(false))

    expect(pages.map((page) => page.path)).toEqual([
      '/quickstart/space',
      '/quickstart/drive',
      '/quickstart/git',
      '/quickstart/canvas',
    ])
  })

  it('keeps experimental quickstart pages in the dev inventory', () => {
    const pages = buildQuickstartStaticPages(getPublicQuickstartOptions(true))

    expect(pages.some((page) => page.path === '/quickstart/notebook')).toBe(
      true,
    )
    expect(pages.some((page) => page.path === '/quickstart/forge')).toBe(true)
  })

  it('keeps metadata in sync with the static page inventory', () => {
    for (const page of STATIC_PAGES) {
      const meta = getMetadata(page.path)

      expect(meta.title).not.toBe('')
      expect(meta.description).not.toBe('')
      expect(meta.description.length).toBeGreaterThanOrEqual(120)
      expect(meta.description.length).toBeLessThanOrEqual(160)
      expect(meta.canonicalPath).toBeTruthy()
      expect(meta.ogImage).toBeTruthy()
    }
  })
})
