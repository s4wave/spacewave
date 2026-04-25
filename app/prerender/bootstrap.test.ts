import { describe, expect, it } from 'vitest'

import { buildBootstrapScript } from './bootstrap.js'

describe('buildBootstrapScript', () => {
  it('references the stable boot asset instead of a hashed entrypoint', () => {
    const script = buildBootstrapScript()

    expect(script).toBe('<script type="module" src="/boot.mjs"></script>')
  })
})
