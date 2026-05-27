import { describe, expect, it } from 'vitest'

import { getLegacyDocRedirect } from './legacy-doc-redirects.js'
import { loadDocs } from './load-docs.js'
import { getSectionLabel, getSections, siteDefs } from './sections.js'
import { getRawMarkdownUrl } from './source-url.js'

describe('docs data', () => {
  it('keeps duplicated section ids scoped to their owning site', () => {
    const sections = getSections()
    const usersCli = sections.find(
      (section) => section.site === 'users' && section.id === 'cli',
    )
    const developersCli = sections.find(
      (section) => section.site === 'developers' && section.id === 'cli',
    )

    expect(usersCli?.label).toBe('Command Line')
    expect(developersCli?.label).toBe('CLI Reference')
    expect(usersCli?.pages.every((page) => page.site === 'users')).toBe(true)
    expect(
      developersCli?.pages.every((page) => page.site === 'developers'),
    ).toBe(true)
  })

  it('loads current anchor pages for every docs site', () => {
    const docs = loadDocs()
    const sites = new Set(docs.map((doc) => doc.site))
    const urls = new Set(docs.map((doc) => doc.url))

    for (const site of siteDefs) {
      expect(sites.has(site.id)).toBe(true)
    }

    expect(Array.from(urls)).toEqual(
      expect.arrayContaining([
        '/docs/users/start/start-here',
        '/docs/users/start/create-your-first-space',
        '/docs/self-hosters/start/choose-how-to-run-spacewave',
        '/docs/developers/start/developer-start-here',
        '/docs/developers/cli/cli-reference',
        '/docs/developers/platform/space-native-docs-boundaries',
      ]),
    )

    expect(
      docs.some(
        (doc) =>
          doc.site === 'developers' &&
          doc.section === 'platform' &&
          doc.slug === 'space-native-docs-boundaries' &&
          doc.body.includes('notes/docs'),
      ),
    ).toBe(true)
  })

  it('uses site-owned labels and current raw source URLs', () => {
    const doc = loadDocs().find(
      (page) => page.url === '/docs/developers/cli/cli-reference',
    )

    expect(doc).toBeDefined()
    expect(getSectionLabel('developers', 'cli')).toBe('CLI Reference')
    expect(getRawMarkdownUrl(doc!)).toBe(
      'https://raw.githubusercontent.com/s4wave/spacewave/master/app/docs/content/developers/cli/01-cli-reference.md',
    )
  })

  it('keeps app-local docs links resolvable inside the current corpus', () => {
    const docs = loadDocs()
    const urls = new Set(docs.map((doc) => doc.url))
    const missingLinks: string[] = []

    for (const doc of docs) {
      for (const match of doc.body.matchAll(
        /\]\((\/docs\/[^)#]+)(?:#[^)]+)?\)/g,
      )) {
        const url = match[1]
        if (
          !urls.has(url) &&
          !siteDefs.some((site) => url === `/docs/${site.id}`)
        ) {
          missingLinks.push(`${doc.url} -> ${url}`)
        }
      }
    }

    expect(missingLinks).toEqual([])
  })

  it('redirects app-owned links into deleted docs pages', () => {
    expect(getLegacyDocRedirect('/docs/users/cli/install')).toBe(
      '/docs/users/cli/command-line-basics',
    )
    expect(
      getLegacyDocRedirect('/docs/developers/cli/installation-and-commands'),
    ).toBe('/docs/developers/cli/cli-reference')
  })
})
