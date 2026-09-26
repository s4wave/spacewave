import { describe, expect, it, vi } from 'vitest'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Client as SRPCClient } from 'starpc'
import { FSHandle } from './handle.js'
import type {
  HandleLookupPathRequest,
  HandleLookupRequest,
  HandleReadStreamRequest,
  HandleUploadFileRequest,
  HandleUploadTreeRequest,
} from './handle.pb.js'

const lookupMock = vi.hoisted(() => vi.fn())
const lookupPathMock = vi.hoisted(() => vi.fn())
const readStreamMock = vi.hoisted(() => vi.fn())
const uploadFileMock = vi.hoisted(() => vi.fn())
const uploadTreeMock = vi.hoisted(() => vi.fn())
const readChunkMaxBytes = 64 * 1024
const uploadDataFrameMaxBytes = 64 * 1024

vi.mock('./handle_srpc.pb.js', () => ({
  FSHandleResourceServiceClient: class {
    Lookup = lookupMock
    LookupPath = lookupPathMock
    ReadStream = readStreamMock
    UploadFile = uploadFileMock
    UploadTree = uploadTreeMock
  },
}))

function buildResourceRef(resourceId = 1): ClientResourceRef {
  const ref: ClientResourceRef = {
    resourceId,
    released: false,
    client: new SRPCClient(),
    createRef: (id) => buildResourceRef(id),
    createResource(id, ResourceClass, ...args) {
      return new ResourceClass(buildResourceRef(id), ...args)
    },
    release: vi.fn(),
    [Symbol.dispose]: vi.fn(),
  }
  return ref
}

function resetUploadFileMock(sent: HandleUploadFileRequest[]): void {
  uploadFileMock.mockReset()
  uploadFileMock.mockImplementation(
    async (requests: AsyncIterable<HandleUploadFileRequest>) => {
      for await (const request of requests) {
        sent.push(request)
      }
      return {
        bytesWritten: sent.reduce(
          (total, request) => total + BigInt(request.data?.byteLength ?? 0),
          0n,
        ),
      }
    },
  )
}

function resetUploadTreeMock(sent: HandleUploadTreeRequest[]): void {
  uploadTreeMock.mockReset()
  uploadTreeMock.mockImplementation(
    async (requests: AsyncIterable<HandleUploadTreeRequest>) => {
      for await (const request of requests) {
        sent.push(request)
      }
      return {
        bytesWritten: 10n,
        filesWritten: 1n,
        directoriesWritten: 1n,
      }
    },
  )
}

function resetReadStreamMock(
  chunks: Uint8Array[],
  sent: HandleReadStreamRequest[],
): void {
  readStreamMock.mockReset()
  readStreamMock.mockImplementation(async function* (
    request: HandleReadStreamRequest,
  ) {
    sent.push(request)
    for (const data of chunks) {
      yield { data }
    }
  })
}

function chunkStream(chunks: Uint8Array[]): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(chunk)
      }
      controller.close()
    },
  })
}

function dataMessageFrames(sent: HandleUploadTreeRequest[]): Uint8Array[] {
  return sent.flatMap((request) =>
    request.body?.case === 'data' ? [request.body.value] : [],
  )
}

describe('FSHandle path identity', () => {
  it('keeps root and dot lookup paths at root identity', async () => {
    lookupPathMock.mockReset()
    lookupPathMock.mockImplementation((request: HandleLookupPathRequest) =>
      Promise.resolve({
        resourceId: request.path === '.' ? 2 : 3,
        traversedPath: [],
        info: { isDir: true },
      }),
    )

    const handle = new FSHandle(buildResourceRef())
    const dot = await handle.lookupPath('.')
    const root = await handle.lookupPath('/')

    expect(dot.handle.getPath()).toBe('')
    expect(dot.traversedPath).toEqual([])
    expect(root.handle.getPath()).toBe('')
    expect(root.traversedPath).toEqual([])
  })

  it('records backend-cleaned traversed paths after path lookup', async () => {
    lookupPathMock.mockReset()
    lookupPathMock.mockResolvedValue({
      resourceId: 2,
      traversedPath: ['docs', 'logo.png'],
      info: { name: 'logo.png', isDir: false },
    })

    const handle = new FSHandle(buildResourceRef())
    const result = await handle.lookupPath('/docs//logo.png')

    expect(lookupPathMock).toHaveBeenCalledWith(
      { path: '/docs//logo.png' },
      undefined,
    )
    expect(result.traversedPath).toEqual(['docs', 'logo.png'])
    expect(result.handle.getPath()).toBe('docs/logo.png')
  })

  it('joins relative backend-cleaned lookup paths onto the current handle path', async () => {
    lookupPathMock.mockReset()
    lookupPathMock.mockResolvedValue({
      resourceId: 3,
      traversedPath: ['nested', 'report.md'],
      info: { name: 'report.md', isDir: false },
    })

    const handle = new FSHandle(buildResourceRef(), { path: 'docs' })
    const result = await handle.lookupPath('./nested//report.md')

    expect(result.traversedPath).toEqual(['nested', 'report.md'])
    expect(result.handle.getPath()).toBe('docs/nested/report.md')
  })

  it('normalizes nested child lookup identity from the current handle path', async () => {
    lookupMock.mockReset()
    lookupMock.mockImplementation((request: HandleLookupRequest) =>
      Promise.resolve({
        resourceId: 4,
        info: { name: request.name, isDir: false },
      }),
    )

    const handle = new FSHandle(buildResourceRef(), { path: 'docs' })
    const child = await handle.lookup('report.md')

    expect(lookupMock).toHaveBeenCalledWith({ name: 'report.md' }, undefined)
    expect(child.getPath()).toBe('docs/report.md')
  })
})

describe('FSHandle readAt', () => {
  it('joins the streamed frames of an explicit length', async () => {
    const first = new Uint8Array(readChunkMaxBytes)
    first.fill(1)
    const second = new Uint8Array([2, 3, 4])
    const requests: HandleReadStreamRequest[] = []
    resetReadStreamMock([first, second], requests)

    const handle = new FSHandle(buildResourceRef())
    const length = BigInt(first.byteLength + second.byteLength)
    const result = await handle.readAt(5n, length)

    expect(result.bytesRead).toBe(length)
    expect(result.eof).toBe(false)
    expect(result.data.byteLength).toBe(first.byteLength + second.byteLength)
    expect(result.data[0]).toBe(1)
    expect(Array.from(result.data.slice(first.byteLength))).toEqual([2, 3, 4])
    expect(requests).toEqual([{ offset: 5n, length }])
  })

  it('reports eof when the file ends before the length', async () => {
    resetReadStreamMock([new Uint8Array([1, 2])], [])

    const handle = new FSHandle(buildResourceRef())
    const result = await handle.readAt(0n, 10n)

    expect(result.bytesRead).toBe(2n)
    expect(result.eof).toBe(true)
  })

  it('reads the remaining file for length zero', async () => {
    const requests: HandleReadStreamRequest[] = []
    resetReadStreamMock([new Uint8Array([5, 6]), new Uint8Array([7])], requests)

    const handle = new FSHandle(buildResourceRef())
    const result = await handle.readAt(10n, 0n)

    expect(result.bytesRead).toBe(3n)
    expect(result.eof).toBe(true)
    expect(Array.from(result.data)).toEqual([5, 6, 7])
    expect(requests).toEqual([{ offset: 10n, length: 0n }])
  })
})

describe('FSHandle uploadFile', () => {
  it('splits oversized stream chunks into bounded data messages', async () => {
    const sent: HandleUploadFileRequest[] = []
    resetUploadFileMock(sent)

    const source = new Uint8Array(uploadDataFrameMaxBytes * 2 + 17)
    const handle = new FSHandle(buildResourceRef())
    const progress: bigint[] = []

    const bytesWritten = await handle.uploadFile(
      'large.bin',
      BigInt(source.byteLength),
      chunkStream([source]),
      0o644,
      (bytes) => progress.push(bytes),
    )

    expect(bytesWritten).toBe(BigInt(source.byteLength))
    expect(uploadFileMock).toHaveBeenCalledTimes(1)
    expect(sent.map((request) => request.data?.byteLength ?? 0)).toEqual([
      uploadDataFrameMaxBytes,
      uploadDataFrameMaxBytes,
      17,
    ])
    expect(sent[0]).toMatchObject({
      name: 'large.bin',
      totalSize: BigInt(source.byteLength),
      mode: 0o644,
    })
    expect(sent.slice(1).map((request) => request.name ?? '')).toEqual(['', ''])
    for (const request of sent) {
      expect(request.data?.byteLength ?? 0).toBeLessThanOrEqual(
        uploadDataFrameMaxBytes,
      )
      expect(request.data?.buffer.byteLength ?? 0).toBe(
        request.data?.byteLength ?? 0,
      )
    }
    expect(progress).toEqual([
      BigInt(uploadDataFrameMaxBytes),
      BigInt(uploadDataFrameMaxBytes * 2),
      BigInt(source.byteLength),
    ])
  })
})

describe('FSHandle uploadTree', () => {
  it('forwards file streams as bounded data messages', async () => {
    const sent: HandleUploadTreeRequest[] = []
    resetUploadTreeMock(sent)

    const handle = new FSHandle(buildResourceRef())
    const fileProgress: bigint[] = []
    const totalProgress: bigint[] = []
    const response = await handle.uploadTree(
      [
        { kind: 'directory', path: 'nested' },
        {
          kind: 'file',
          path: 'nested/large.bin',
          totalSize: 10n,
          stream: chunkStream([
            new Uint8Array([1, 2, 3]),
            new Uint8Array([4, 5, 6, 7]),
            new Uint8Array([8, 9, 10]),
          ]),
          onProgress: (bytes) => fileProgress.push(bytes),
        },
      ],
      (bytes) => totalProgress.push(bytes),
    )

    expect(response).toEqual({
      bytesWritten: 10n,
      filesWritten: 1n,
      directoriesWritten: 1n,
    })
    expect(uploadTreeMock).toHaveBeenCalledTimes(1)
    expect(sent.map((request) => request.body?.case)).toEqual([
      'directory',
      'fileStart',
      'data',
      'data',
      'data',
    ])
    expect(sent[0]?.body).toEqual({
      case: 'directory',
      value: {
        path: 'nested',
        mode: 0o755,
      },
    })
    expect(sent[1]?.body).toEqual({
      case: 'fileStart',
      value: {
        path: 'nested/large.bin',
        totalSize: 10n,
        mode: 0,
      },
    })
    expect(dataMessageFrames(sent).map((frame) => Array.from(frame))).toEqual([
      [1, 2, 3],
      [4, 5, 6, 7],
      [8, 9, 10],
    ])
    expect(fileProgress).toEqual([3n, 7n, 10n])
    expect(totalProgress).toEqual([3n, 7n, 10n])
  })

  it('splits oversized stream chunks into bounded data messages', async () => {
    const sent: HandleUploadTreeRequest[] = []
    resetUploadTreeMock(sent)

    const source = new Uint8Array(uploadDataFrameMaxBytes * 2 + 17)
    const handle = new FSHandle(buildResourceRef())
    const fileProgress: bigint[] = []
    const totalProgress: bigint[] = []

    await handle.uploadTree(
      [
        {
          kind: 'file',
          path: 'large.bin',
          totalSize: BigInt(source.byteLength),
          stream: chunkStream([source]),
          onProgress: (bytes) => fileProgress.push(bytes),
        },
      ],
      (bytes) => totalProgress.push(bytes),
    )

    const frames = dataMessageFrames(sent)
    expect(frames.map((frame) => frame.byteLength)).toEqual([
      uploadDataFrameMaxBytes,
      uploadDataFrameMaxBytes,
      17,
    ])
    for (const frame of frames) {
      expect(frame.byteLength).toBeLessThanOrEqual(uploadDataFrameMaxBytes)
      expect(frame.buffer.byteLength).toBe(frame.byteLength)
    }
    expect(fileProgress).toEqual([
      BigInt(uploadDataFrameMaxBytes),
      BigInt(uploadDataFrameMaxBytes * 2),
      BigInt(source.byteLength),
    ])
    expect(totalProgress).toEqual(fileProgress)
  })
})
