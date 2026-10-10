import { describe, expect, test } from 'vitest'

import { bindWebPkgImports } from './web-pkg-imports.js'

describe('shared package imports', () => {
  test('emits pinned URL prefixes before importing a renderer', () => {
    // Keep the existing runtime import map separate from plugin bindings.
    const target = document.implementation.createHTMLDocument()
    target.head.innerHTML =
      '<script type="importmap">{"imports":{"react":"/entrypoint/pkgs/react/index.mjs"}}</script>'

    // Bind every subpath of a shared package to the selected provider manifest.
    bindWebPkgImports(
      {
        '@s4wave/web': '/b/pa/spacewave-web/manifest/root/pkgs/@s4wave/web/',
        react: '',
      },
      target,
    )

    // Verify the release binding without replacing the runtime's existing map.
    const scripts = target.querySelectorAll('script[type="importmap"]')
    expect(scripts).toHaveLength(2)
    expect(JSON.parse(scripts[1]!.textContent!)).toEqual({
      imports: {
        'bldr-web-pkg/@s4wave/web/':
          '/b/pa/spacewave-web/manifest/root/pkgs/@s4wave/web/',
        'bldr-web-pkg/react/': '/b/pkg/react/',
      },
    })
    expect(JSON.parse(scripts[0]!.textContent!).imports.react).toBe(
      '/entrypoint/pkgs/react/index.mjs',
    )
  })
})
