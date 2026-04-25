import { describe, expect, it } from 'vitest'

import {
  collectUnixFSGalleryCandidates,
  streamUnixFSGalleryCandidates,
} from './gallery.js'

type FakeNode =
  | {
      kind: 'dir'
      children: Record<string, FakeNode>
    }
  | {
      kind: 'file'
    }

class FakeHandle {
  constructor(
    private readonly node: FakeNode,
    private readonly path = '',
  ) {}

  [Symbol.dispose](): void {}

  clone(): Promise<FakeHandle> {
    return Promise.resolve(new FakeHandle(this.node, this.path))
  }

  getFileInfo(): Promise<{ isDir: boolean }> {
    return Promise.resolve({ isDir: this.node.kind === 'dir' })
  }

  lookup(name: string): Promise<FakeHandle> {
    if (this.node.kind !== 'dir') {
      return Promise.reject(new Error('not a directory'))
    }
    const child = this.node.children[name]
    if (!child) {
      return Promise.reject(new Error(`missing child: ${name}`))
    }
    return Promise.resolve(
      new FakeHandle(child, this.path ? `${this.path}/${name}` : name),
    )
  }

  async lookupPath(path: string): Promise<{
    handle: FakeHandle
    traversedPath: string[]
  }> {
    const parts = path.split('/').filter(Boolean)
    let handle: FakeHandle = new FakeHandle(this.node, this.path)
    for (const part of parts) {
      handle = await handle.lookup(part)
    }
    return {
      handle,
      traversedPath: parts,
    }
  }

  readdirAll(): Promise<Array<{ isDir: boolean; name: string }>> {
    if (this.node.kind !== 'dir') {
      return Promise.resolve([])
    }
    return Promise.resolve(
      Object.entries(this.node.children).map(([name, child]) => ({
        name,
        isDir: child.kind === 'dir',
      })),
    )
  }
}

function buildRootHandle(node: FakeNode) {
  return new FakeHandle(node) as unknown as Parameters<
    typeof collectUnixFSGalleryCandidates
  >[0]
}

describe('collectUnixFSGalleryCandidates', () => {
  it('recurses under the scoped path and returns nested image candidates', async () => {
    const items = await collectUnixFSGalleryCandidates(
      buildRootHandle({
        kind: 'dir',
        children: {
          docs: {
            kind: 'dir',
            children: {
              'cover.png': { kind: 'file' },
              nested: {
                kind: 'dir',
                children: {
                  'poster.jpg': { kind: 'file' },
                },
              },
            },
          },
          other: {
            kind: 'dir',
            children: {
              'outside.webp': { kind: 'file' },
            },
          },
        },
      }),
      '/docs',
      new AbortController().signal,
    )

    expect(items.map((item) => item.path)).toEqual([
      '/docs/cover.png',
      '/docs/nested/poster.jpg',
    ])
  })

  it('filters the candidate set to the supported gallery mime types', async () => {
    const items = await collectUnixFSGalleryCandidates(
      buildRootHandle({
        kind: 'dir',
        children: {
          media: {
            kind: 'dir',
            children: {
              'logo.svg': { kind: 'file' },
              'photo.avif': { kind: 'file' },
              'notes.txt': { kind: 'file' },
              'icon.ico': { kind: 'file' },
            },
          },
        },
      }),
      '/media',
      new AbortController().signal,
    )

    expect(items.map((item) => [item.name, item.mimeType])).toEqual([
      ['logo.svg', 'image/svg+xml'],
      ['photo.avif', 'image/avif'],
    ])
  })

  it('falls back to the parent directory when the scoped path points at a file', async () => {
    const items = await collectUnixFSGalleryCandidates(
      buildRootHandle({
        kind: 'dir',
        children: {
          docs: {
            kind: 'dir',
            children: {
              'cover.png': { kind: 'file' },
              'poster.jpg': { kind: 'file' },
              notes: {
                kind: 'dir',
                children: {
                  'ignore.txt': { kind: 'file' },
                },
              },
            },
          },
        },
      }),
      '/docs/cover.png',
      new AbortController().signal,
    )

    expect(items.map((item) => item.path)).toEqual([
      '/docs/cover.png',
      '/docs/poster.jpg',
    ])
  })
})

describe('streamUnixFSGalleryCandidates', () => {
  it('emits progressive updates before the recursive crawl completes', async () => {
    const states: Array<{
      complete: boolean
      count: number
      scopePath: string
    }> = []
    for await (const state of streamUnixFSGalleryCandidates(
      buildRootHandle({
        kind: 'dir',
        children: {
          docs: {
            kind: 'dir',
            children: {
              'cover.png': { kind: 'file' },
              nested: {
                kind: 'dir',
                children: {
                  'poster.jpg': { kind: 'file' },
                },
              },
            },
          },
        },
      }),
      '/docs',
      new AbortController().signal,
    )) {
      states.push({
        complete: state.complete,
        count: state.items.length,
        scopePath: state.scopePath,
      })
    }

    expect(states).toEqual([
      { count: 0, complete: false, scopePath: '/docs' },
      { count: 1, complete: false, scopePath: '/docs' },
      { count: 2, complete: false, scopePath: '/docs' },
      { count: 2, complete: true, scopePath: '/docs' },
    ])
  })
})
