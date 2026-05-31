import { describe, expect, it } from 'vitest'

import {
  collectUnixFSGalleryCandidates,
  type UnixFSGalleryDiscoveryState,
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

const fakeNodeWatchers = new WeakMap<FakeNode, Set<() => void>>()

function watchFakeNode(node: FakeNode, cb: () => void): () => void {
  let watchers = fakeNodeWatchers.get(node)
  if (!watchers) {
    watchers = new Set()
    fakeNodeWatchers.set(node, watchers)
  }
  watchers.add(cb)
  return () => watchers?.delete(cb)
}

function notifyFakeNode(node: FakeNode): void {
  for (const cb of fakeNodeWatchers.get(node) ?? []) {
    cb()
  }
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

  async *watchReaddir(
    signal?: AbortSignal,
  ): AsyncIterable<Array<{ isDir: boolean; name: string }>> {
    if (this.node.kind !== 'dir') {
      return
    }
    yield await this.readdirAll()

    while (!signal?.aborted) {
      await new Promise<void>((resolve) => {
        const release = watchFakeNode(this.node, () => {
          release()
          resolve()
        })
        signal?.addEventListener(
          'abort',
          () => {
            release()
            resolve()
          },
          { once: true },
        )
      })
      if (!signal?.aborted) {
        yield await this.readdirAll()
      }
    }
  }

  putFile(path: string): void {
    this.putNode(path, { kind: 'file' })
  }

  remove(path: string): void {
    const [parent, name] = this.lookupParent(path)
    delete parent.children[name]
    notifyFakeNode(parent)
  }

  private putNode(path: string, node: FakeNode): void {
    const [parent, name] = this.lookupParent(path)
    parent.children[name] = node
    notifyFakeNode(parent)
  }

  private lookupParent(
    path: string,
  ): [{ kind: 'dir'; children: Record<string, FakeNode> }, string] {
    const parts = path.split('/').filter(Boolean)
    const name = parts.pop()
    if (!name) {
      throw new Error('path must include a file name')
    }
    let node = this.node
    for (const part of parts) {
      if (node.kind !== 'dir') {
        throw new Error(`not a directory: ${part}`)
      }
      const child = node.children[part]
      if (!child) {
        throw new Error(`missing child: ${part}`)
      }
      node = child
    }
    if (node.kind !== 'dir') {
      throw new Error('parent is not a directory')
    }
    return [node, name]
  }
}

function buildRootHandle(node: FakeNode) {
  return new FakeHandle(node) as unknown as Parameters<
    typeof collectUnixFSGalleryCandidates
  >[0]
}

function buildFakeRootHandle(node: FakeNode): FakeHandle {
  return new FakeHandle(node)
}

async function nextGalleryPaths(
  iter: AsyncIterator<UnixFSGalleryDiscoveryState>,
): Promise<string[] | undefined> {
  const next = await Promise.race([
    iter.next(),
    new Promise<IteratorResult<UnixFSGalleryDiscoveryState>>((_, reject) => {
      setTimeout(
        () => reject(new Error('timed out waiting for gallery state')),
        1000,
      )
    }),
  ])
  const state: UnixFSGalleryDiscoveryState | undefined = next.done
    ? undefined
    : next.value
  return state?.items.map((item) => item.path)
}

async function waitForGalleryPaths(
  iter: AsyncIterator<UnixFSGalleryDiscoveryState>,
  expected: string[],
): Promise<void> {
  for (let i = 0; i < 10; i++) {
    const paths = await nextGalleryPaths(iter)
    expect(paths).toBeDefined()
    if (JSON.stringify(paths) === JSON.stringify(expected)) {
      return
    }
  }
  throw new Error(`gallery stream did not reach ${expected.join(', ')}`)
}

function waitForWatchTurn(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0))
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
  it('emits the watched recursive crawl state', async () => {
    const states: Array<{
      complete: boolean
      count: number
      scopePath: string
    }> = []
    const ctrl = new AbortController()
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
      ctrl.signal,
    )) {
      states.push({
        complete: state.complete,
        count: state.items.length,
        scopePath: state.scopePath,
      })
      if (state.complete) {
        ctrl.abort()
      }
    }

    expect(states).toEqual([
      { count: 0, complete: false, scopePath: '/docs' },
      { count: 2, complete: true, scopePath: '/docs' },
    ])
  })

  it('updates when watched directory contents change', async () => {
    const root = buildFakeRootHandle({
      kind: 'dir',
      children: {
        docs: {
          kind: 'dir',
          children: {
            'cover.png': { kind: 'file' },
          },
        },
      },
    })
    const ctrl = new AbortController()
    const iter = streamUnixFSGalleryCandidates(
      root as unknown as Parameters<typeof streamUnixFSGalleryCandidates>[0],
      '/docs',
      ctrl.signal,
    )[Symbol.asyncIterator]()

    await waitForGalleryPaths(iter, ['/docs/cover.png'])

    await waitForWatchTurn()
    root.putFile('/docs/new.webp')

    await waitForGalleryPaths(iter, ['/docs/cover.png', '/docs/new.webp'])

    await waitForWatchTurn()
    root.remove('/docs/cover.png')

    await waitForGalleryPaths(iter, ['/docs/new.webp'])

    ctrl.abort()
    await iter.return?.()
  })
})
