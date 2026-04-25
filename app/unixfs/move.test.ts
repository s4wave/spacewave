import { describe, expect, it, vi } from 'vitest'
import {
  buildUnixFSMoveItems,
  moveUnixFSItems,
  validateUnixFSMove,
} from './move.js'

function buildDisposableHandle<T extends object>(value: T) {
  return {
    [Symbol.dispose]: () => undefined,
    ...value,
  }
}

describe('buildUnixFSMoveItems', () => {
  it('builds root-relative move items for the current directory selection', () => {
    expect(
      buildUnixFSMoveItems('/docs', [
        { id: 'a', name: 'notes.txt', isDir: false },
        { id: 'b', name: 'nested', isDir: true },
      ]),
    ).toEqual([
      { id: 'a', name: 'notes.txt', isDir: false, path: '/docs/notes.txt' },
      { id: 'b', name: 'nested', isDir: true, path: '/docs/nested' },
    ])
  })
})

describe('validateUnixFSMove', () => {
  it('accepts a move into a different directory', () => {
    expect(
      validateUnixFSMove(
        [{ id: 'a', name: 'notes.txt', isDir: false, path: '/notes.txt' }],
        '/archive',
      ),
    ).toEqual({ accepted: true, reason: null })
  })

  it('rejects moves into the same parent directory', () => {
    expect(
      validateUnixFSMove(
        [{ id: 'a', name: 'notes.txt', isDir: false, path: '/notes.txt' }],
        '/',
      ),
    ).toEqual({ accepted: false, reason: 'same-parent' })
  })

  it('rejects moving a directory into its own descendant', () => {
    expect(
      validateUnixFSMove(
        [{ id: 'a', name: 'docs', isDir: true, path: '/docs' }],
        '/docs/archive',
      ),
    ).toEqual({ accepted: false, reason: 'descendant' })
  })
})

describe('moveUnixFSItems', () => {
  it('groups moves by parent path and reuses one destination handle', async () => {
    const renameRoot = vi.fn()
    const renameDocs = vi.fn()
    const clone = vi.fn()
    const lookupPath = vi.fn()

    clone.mockResolvedValue(
      buildDisposableHandle({
        id: 10,
        rename: renameRoot,
      }),
    )
    lookupPath.mockImplementation((path: string) => {
      if (path === 'archive') {
        return { handle: buildDisposableHandle({ id: 77 }) }
      }
      if (path === 'docs') {
        return {
          handle: buildDisposableHandle({
            id: 21,
            rename: renameDocs,
          }),
        }
      }
      throw new Error(`unexpected path ${path}`)
    })

    await moveUnixFSItems(
      {
        clone,
        lookupPath,
      } as never,
      [
        { id: 'a', name: 'hello.txt', isDir: false, path: '/hello.txt' },
        { id: 'b', name: 'todo.txt', isDir: false, path: '/docs/todo.txt' },
      ],
      '/archive',
    )

    expect(clone).toHaveBeenCalledOnce()
    expect(lookupPath).toHaveBeenCalledWith('archive', undefined)
    expect(lookupPath).toHaveBeenCalledWith('docs', undefined)
    expect(renameRoot).toHaveBeenCalledWith(
      'hello.txt',
      'hello.txt',
      77,
      undefined,
    )
    expect(renameDocs).toHaveBeenCalledWith(
      'todo.txt',
      'todo.txt',
      77,
      undefined,
    )
  })
})
