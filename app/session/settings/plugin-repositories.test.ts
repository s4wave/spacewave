import { describe, expect, it, vi } from 'vitest'

import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// Each fake worktree handle answers for the object key its ref names.
const commits: Record<string, { pinned: string; fetched: string }> = {
  'plugin-repos/s4wave/spreadsheet/worktree': { pinned: 'aaa', fetched: 'bbb' },
}

vi.mock('@s4wave/sdk/git/worktree.js', () => ({
  GitWorktreeTypeID: 'git/worktree',
  GitWorktreeHandle: class {
    constructor(private readonly ref: { key: string }) {}
    getWorktreeInfo() {
      return Promise.resolve({ headCommitHash: commits[this.ref.key].pinned })
    }
    getRepoHandle() {
      const fetched = commits[this.ref.key].fetched
      return Promise.resolve({
        resolveRef: (refName: string) =>
          Promise.resolve({
            commitHash: refName === 'refs/remotes/origin/HEAD' ? fetched : '',
          }),
        [Symbol.dispose]: () => {},
      })
    }
    [Symbol.dispose]() {}
  },
}))

import { readPluginRepositories } from './plugin-repositories.js'

describe('readPluginRepositories', () => {
  it('reads the pinned and fetched commits of plugin repositories only', async () => {
    const world = {
      listObjectsWithType: (typeId: string) =>
        Promise.resolve(
          typeId === 'git/worktree'
            ? ['notes/worktree', 'plugin-repos/s4wave/spreadsheet/worktree']
            : ['devices/laptop'],
        ),
      accessTypedObject: (key: string) => Promise.resolve({ resourceId: key }),
      getResourceRef: () => ({ createRef: (key: string) => ({ key }) }),
    } as unknown as IWorldState

    expect(await readPluginRepositories(world)).toEqual({
      devices: ['devices/laptop'],
      repositories: [
        {
          name: 's4wave/spreadsheet',
          pinnedCommit: 'aaa',
          fetchedCommit: 'bbb',
        },
      ],
    })
  })
})
