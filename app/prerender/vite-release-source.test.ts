import { describe, expect, it } from 'vitest'
import type { Alias, UserConfig } from 'vite'

import hydrateConfig from './vite.hydrate.config.js'
import ssrConfig from './vite.ssr.config.js'

function aliasReplacement(
  config: UserConfig,
  find: string,
): string | undefined {
  const aliases = config.resolve?.alias as Alias[]
  return aliases.find((alias) => String(alias.find) === find)?.replacement
}

describe('release prerender Bldr sources', () => {
  it.each([
    ['hydrate', hydrateConfig],
    ['ssr', ssrConfig],
  ])('%s resolves Bldr SDK imports from release build output', (_, config) => {
    expect(
      aliasReplacement(config, String(/^@aptre\/bldr-sdk\/(.*)$/)),
    ).toContain('/.bldr-dist/src/sdk/$1')
  })
})
