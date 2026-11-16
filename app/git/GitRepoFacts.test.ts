import { describe, expect, it } from 'vitest'

import { gitCloneAddress } from './GitRepoFacts.js'

describe('gitCloneAddress', () => {
  it('escapes the key for the Git remote parser', () => {
    // Spaces, percent signs and non-ASCII letters are percent-encoded, and
    // the slashes between ordinary segments stay raw.
    expect(gitCloneAddress(1, 'sp', 'My Repo')).toBe(
      'spacewave:///u/1/so/sp/-/My%20Repo',
    )
    expect(gitCloneAddress(1, 'sp', '100%')).toBe(
      'spacewave:///u/1/so/sp/-/100%25',
    )
    expect(gitCloneAddress(2, 'sp', 'repos/café')).toBe(
      'spacewave:///u/2/so/sp/-/repos/caf%C3%A9',
    )
  })

  it('escapes what the parser would clean or split', () => {
    // These addresses are the ones TestParseSpacewaveURIEscapes in
    // sdk/space/object-uri_test.go reads back as the key.
    const cases: [string, string][] = [
      ['/repos/x', '%2Frepos/x'],
      ['repos/x/', 'repos/x%2F'],
      ['a//b', 'a%2F%2Fb'],
      ['a/./b', 'a/%2E/b'],
      ['a/../b', 'a/%2E%2E/b'],
      ['a/-/b', 'a/%2D/b'],
      ['-', '%2D'],
    ]
    for (const [key, escaped] of cases) {
      expect(gitCloneAddress(1, 'sp', key)).toBe(
        `spacewave:///u/1/so/sp/-/${escaped}`,
      )
    }
  })

  it('gives no address without a session, Space or key', () => {
    // Each part is needed to name the object.
    expect(gitCloneAddress(0, 'sp', 'k')).toBeNull()
    expect(gitCloneAddress(1, undefined, 'k')).toBeNull()
    expect(gitCloneAddress(1, 'sp', '')).toBeNull()
  })
})
