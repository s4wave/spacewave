import { beforeEach, describe, expect, it, vi } from 'vitest'

import { Blog } from './proto/blog.pb.js'

// createMockCursor returns a cursor over blockData, or over a missing block.
function createMockCursor(blockData?: Uint8Array) {
  const release = vi.fn()
  return {
    getBlock: vi.fn(() =>
      Promise.resolve({ found: !!blockData, data: blockData }),
    ),
    release,
    [Symbol.dispose]: release,
  }
}

// createMockObjectState returns an object whose root reads through cursor.
function createMockObjectState(
  cursor: ReturnType<typeof createMockCursor>,
  hash = 1,
) {
  const release = vi.fn()
  return {
    getRootRef: vi.fn(() =>
      Promise.resolve({
        rootRef: {
          bucketId: 'test',
          rootRef: { hash: { hash: new Uint8Array([hash]) } },
        },
      }),
    ),
    accessWorldState: vi.fn(() => Promise.resolve(cursor)),
    release,
    [Symbol.dispose]: release,
  }
}

// createMockSnapshot returns a read snapshot holding objectState.
function createMockSnapshot(
  objectState: ReturnType<typeof createMockObjectState> | null,
  seqno = 1n,
) {
  const release = vi.fn()
  return {
    getSeqno: vi.fn(() => Promise.resolve({ seqno })),
    getObject: vi.fn(() => Promise.resolve(objectState)),
    release,
    [Symbol.dispose]: release,
  }
}

type MockSnapshot = ReturnType<typeof createMockSnapshot>

// abortError is the rejection of an aborted wait.
function abortError() {
  return new DOMException('Aborted', 'AbortError')
}

// createMockEngineRef returns an Engine reference that clones itself.
function createMockEngineRef(): Record<string, unknown> {
  return {
    resourceId: 1,
    released: false,
    client: {},
    createRef: vi.fn(() => createMockEngineRef()),
    createResource: vi.fn(),
    release: vi.fn(),
    [Symbol.dispose]: vi.fn(),
  }
}

// createMockEngine returns an Engine whose watch yields each queued snapshot,
// then waits for the next push until the signal aborts.
function createMockEngine(snapshots: MockSnapshot[]) {
  const queue = [...snapshots]
  let wake: (() => void) | undefined
  return {
    watchWorldState: vi.fn(async function* (signal?: AbortSignal) {
      for (;;) {
        const next = queue.shift()
        if (next) {
          yield next
          continue
        }
        await new Promise<void>((resolve, reject) => {
          if (signal?.aborted) {
            reject(abortError())
            return
          }
          wake = resolve
          signal?.addEventListener('abort', () => reject(abortError()), {
            once: true,
          })
        })
      }
    }),
    push(snapshot: MockSnapshot) {
      queue.push(snapshot)
      wake?.()
    },
    resourceRef: createMockEngineRef(),
    id: 1,
    release: vi.fn(),
    [Symbol.dispose]() {
      this.release()
    },
  }
}

let mockEngineInstance: ReturnType<typeof createMockEngine>

vi.mock('@s4wave/sdk/world/engine.js', () => ({
  Engine: vi.fn(function (ref) {
    mockEngineInstance.resourceRef = ref
    return mockEngineInstance
  }),
}))

const { BlogResource } = await import('./blog-resource.js')

// makeBlogBytes encodes a Blog with one posts source.
function makeBlogBytes(name: string, authorRegistryPath: string) {
  return Blog.toBinary(
    Blog.create({
      name,
      sources: [{ name: 'Posts', ref: 'blog-fs/-/posts' }],
      authorRegistryPath,
    }),
  )
}

// snapshotOf returns a snapshot of a blog object with the given root hash.
function snapshotOf(name: string, hash = 1, seqno = 1n) {
  const cursor = createMockCursor(makeBlogBytes(name, 'authors.yaml'))
  const objectState = createMockObjectState(cursor, hash)
  return {
    cursor,
    objectState,
    snapshot: createMockSnapshot(objectState, seqno),
  }
}

describe('BlogResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('yields nothing when engineRef is undefined', async () => {
    const resource = new BlogResource('obj-key', undefined)
    const results: unknown[] = []
    for await (const msg of resource.WatchBlog({})) {
      results.push(msg)
    }
    expect(results).toHaveLength(0)
  })

  it('yields the Blog block from the world', async () => {
    // Watch a blog object.
    const { cursor, objectState, snapshot } = snapshotOf('Test Blog')
    mockEngineInstance = createMockEngine([snapshot])
    const resource = new BlogResource('my-blog', createMockEngineRef() as never)
    const ac = new AbortController()
    const results: { blog: Blog }[] = []
    for await (const msg of resource.WatchBlog({}, ac.signal)) {
      results.push(msg as { blog: Blog })
      ac.abort()
    }

    // The first message carries the decoded block.
    expect(results).toHaveLength(1)
    const blog = results[0].blog
    expect(blog.name).toBe('Test Blog')
    expect(blog.authorRegistryPath).toBe('authors.yaml')
    expect(blog.sources?.[0].ref).toBe('blog-fs/-/posts')

    // It was read from the snapshot's object root, and the chain was released.
    expect(snapshot.getObject).toHaveBeenCalledWith('my-blog', ac.signal)
    expect(objectState.accessWorldState).toHaveBeenCalled()
    expect(cursor.release).toHaveBeenCalled()
    expect(objectState.release).toHaveBeenCalled()
    expect(snapshot.release).toHaveBeenCalled()
  })

  it('yields again when the object root changes', async () => {
    // Watch a blog object, then publish a changed root.
    const first = snapshotOf('First')
    const second = snapshotOf('Second', 2, 2n)
    mockEngineInstance = createMockEngine([first.snapshot])
    const resource = new BlogResource('my-blog', createMockEngineRef() as never)
    const ac = new AbortController()
    const names: string[] = []
    for await (const msg of resource.WatchBlog({}, ac.signal)) {
      names.push(msg.blog?.name ?? '')
      if (names.length === 1) {
        mockEngineInstance.push(second.snapshot)
      } else {
        ac.abort()
      }
    }

    // Each root yields its own block.
    expect(names).toEqual(['First', 'Second'])
  })
})
