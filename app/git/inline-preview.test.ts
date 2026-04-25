import { describe, expect, it } from 'vitest'

import { getGitWorktreeInlinePreviewObjectKey } from './inline-preview.js'

describe('getGitWorktreeInlinePreviewObjectKey', () => {
  it('uses the repo object key in files mode', () => {
    expect(
      getGitWorktreeInlinePreviewObjectKey({
        mode: 'files',
        repoObjectKey: 'repo/demo',
        workdirObjectKey: 'repo/demo/workdir',
      }),
    ).toBe('repo/demo')
  })

  it('uses the workdir object key in workdir mode', () => {
    expect(
      getGitWorktreeInlinePreviewObjectKey({
        mode: 'workdir',
        repoObjectKey: 'repo/demo',
        workdirObjectKey: 'repo/demo/workdir',
      }),
    ).toBe('repo/demo/workdir')
  })

  it('returns undefined when the selected mode has no projected object key', () => {
    expect(
      getGitWorktreeInlinePreviewObjectKey({
        mode: 'workdir',
        repoObjectKey: 'repo/demo',
        workdirObjectKey: null,
      }),
    ).toBeUndefined()
  })
})
