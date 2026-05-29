import { describe, expect, it, vi } from 'vitest'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Client as SRPCClient } from 'starpc'
import { FSHandle } from './handle.js'
import type {
  HandleUploadFileRequest,
  HandleUploadTreeRequest,
} from './handle.pb.js'

const uploadFileMock = vi.hoisted(() => vi.fn())
const uploadTreeMock = vi.hoisted(() => vi.fn())
const uploadDataFrameMaxBytes = 64 * 1024

vi.mock('./handle_srpc.pb.js', () => ({
  FSHandleResourceServiceClient: class {
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
