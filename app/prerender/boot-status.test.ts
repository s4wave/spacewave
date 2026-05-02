import { describe, expect, it } from 'vitest'

import { canMutateBrowserBootStatusTarget } from './boot-status.js'

describe('canMutateBrowserBootStatusTarget', () => {
  it('allows boot status updates outside prerendered React roots', () => {
    document.body.innerHTML = '<p data-sw-boot-status></p>'

    expect(
      canMutateBrowserBootStatusTarget(
        document.querySelector('[data-sw-boot-status]'),
      ),
    ).toBe(true)
  })

  it('blocks boot status updates inside prerendered React-owned pages', () => {
    document.body.innerHTML = `
      <div id="bldr-root" data-prerendered="true">
        <p data-sw-boot-status></p>
      </div>
    `

    expect(
      canMutateBrowserBootStatusTarget(
        document.querySelector('[data-sw-boot-status]'),
      ),
    ).toBe(false)
  })

  it('allows boot status updates for the non-hydrated root loading screen', () => {
    document.body.innerHTML = `
      <div id="bldr-root" data-prerendered="true">
        <div id="sw-loading">
          <p data-sw-boot-status></p>
        </div>
      </div>
    `

    expect(
      canMutateBrowserBootStatusTarget(
        document.querySelector('[data-sw-boot-status]'),
      ),
    ).toBe(true)
  })
})
