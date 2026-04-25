import { describe, it, expect, vi, beforeEach } from 'vitest'

import { Blog } from './proto/blog.pb.js'

function createMockCursor(blockData?: Uint8Array) {
  return {
    getBlock: vi.fn(() => Promise.resolve({
      found: !!blockData,
      data: blockData,
    })),
    release: vi.fn(),
  }
}

function createMockObjectState(cursor?: ReturnType<typeof createMockCursor>) {
  return {
    accessWorldState: vi.fn(() => Promise.resolve(cursor ?? createMockCursor())),
    release: vi.fn(),
  }
}

function createMockTx(
  objectState?: ReturnType<typeof createMockObjectState> | null,
) {
  return {
    getObject: vi.fn(() => Promise.resolve(objectState ?? null)),
    release: vi.fn(),
  }
}

function createMockEngine(tx?: ReturnType<typeof createMockTx>) {
  return {
    newTransaction: vi.fn(() => Promise.resolve(tx ?? createMockTx())),
    getSeqno: vi.fn(() => Promise.resolve({ seqno: 1n })),
    waitSeqno: vi.fn(
      (_seqno: bigint, signal?: AbortSignal) =>
        new Promise<{ seqno: bigint }>((_resolve, reject) => {
          if (signal?.aborted) {
            reject(new DOMException('Aborted', 'AbortError'))
            return
          }
          const onAbort = () =>
            reject(new DOMException('Aborted', 'AbortError'))
          signal?.addEventListener('abort', onAbort, { once: true })
        }),
    ),
    release: vi.fn(),
  }
}

let mockEngineInstance: ReturnType<typeof createMockEngine>

vi.mock('@s4wave/sdk/world/engine.js', () => ({
  Engine: vi.fn(function () {
    return mockEngineInstance
  }),
}))

const { BlogResource } = await import('./blog-resource.js')

function createMockEngineRef() {
  return {
    resourceId: 1,
    released: false,
    client: {},
    createRef: vi.fn(),
    createResource: vi.fn(),
    release: vi.fn(),
    [Symbol.dispose]: vi.fn(),
  }
}

function makeBlogBytes(name: string, authorRegistryPath: string) {
  return Blog.toBinary(
    Blog.create({
      name,
      sources: [{ name: 'Posts', ref: 'blog-fs/-/posts' }],
      authorRegistryPath,
    }),
  )
}

describe('BlogResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('WatchBlog', () => {
    it('yields nothing when engineRef is undefined', async () => {
      const resource = new BlogResource('obj-key', undefined)
      const results: unknown[] = []
      for await (const msg of resource.WatchBlog({})) {
        results.push(msg)
      }
      expect(results).toHaveLength(0)
    })

    it('yields the Blog block from the world', async () => {
      const blogData = makeBlogBytes('Test Blog', 'authors.yaml')
      const cursor = createMockCursor(blogData)
      const objectState = createMockObjectState(cursor)
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new BlogResource('my-blog', engineRef as never)

      const ac = new AbortController()
      const results: unknown[] = []
      for await (const msg of resource.WatchBlog({}, ac.signal)) {
        results.push(msg)
        ac.abort()
      }

      expect(results).toHaveLength(1)
      const blog = (results[0] as { blog: Blog }).blog
      expect(blog.name).toBe('Test Blog')
      expect(blog.authorRegistryPath).toBe('authors.yaml')
      expect(blog.sources).toHaveLength(1)
      expect(blog.sources![0].ref).toBe('blog-fs/-/posts')

      expect(mockEngineInstance.newTransaction).toHaveBeenCalledWith(
        false,
        ac.signal,
      )
      expect(tx.getObject).toHaveBeenCalledWith('my-blog', ac.signal)
      expect(objectState.accessWorldState).toHaveBeenCalledWith(
        undefined,
        ac.signal,
      )
      expect(cursor.getBlock).toHaveBeenCalledWith({}, ac.signal)
    })

    it('releases all resources in the read chain', async () => {
      const blogData = makeBlogBytes('Cleanup Blog', '')
      const cursor = createMockCursor(blogData)
      const objectState = createMockObjectState(cursor)
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new BlogResource('key', engineRef as never)

      const ac = new AbortController()
      for await (const _ of resource.WatchBlog({}, ac.signal)) {
        ac.abort()
      }

      expect(cursor.release).toHaveBeenCalled()
      expect(objectState.release).toHaveBeenCalled()
      expect(tx.release).toHaveBeenCalled()
      expect(mockEngineInstance.release).toHaveBeenCalled()
    })
  })
})
