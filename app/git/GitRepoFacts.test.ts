import { describe, expect, it } from 'vitest'

import { gitCloneAddress } from './GitRepoFacts.js'

describe('gitCloneAddress', () => {
  it('writes the key raw, as the Git remote parser reads it', () => {
    // The parser never percent-decodes, so spaces, percent signs and
    // non-ASCII letters go in as they are.
    expect(gitCloneAddress(1, 'sp', 'My Repo')).toBe(
      'spacewave:///u/1/so/sp/-/My Repo',
    )
    expect(gitCloneAddress(1, 'sp', '100%')).toBe(
      'spacewave:///u/1/so/sp/-/100%',
    )
    expect(gitCloneAddress(2, 'sp', 'repos/café')).toBe(
      'spacewave:///u/2/so/sp/-/repos/café',
    )
  })

  it('gives no address for a key the parser would read back as another', () => {
    // A leading, trailing or doubled slash is cleaned away, and a "-"
    // segment splits the path.
    for (const key of [
      '/repos/x',
      'repos/x/',
      'a//b',
      'a/./b',
      'a/../b',
      'a/-/b',
    ]) {
      expect(gitCloneAddress(1, 'sp', key)).toBeNull()
    }
  })

  it('gives no address without a session, Space or key', () => {
    // Each part is needed to name the object.
    expect(gitCloneAddress(0, 'sp', 'k')).toBeNull()
    expect(gitCloneAddress(1, undefined, 'k')).toBeNull()
    expect(gitCloneAddress(1, 'sp', '')).toBeNull()
  })
})
