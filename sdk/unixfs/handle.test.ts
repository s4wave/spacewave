import { describe, expect, it, vi } from 'vitest'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Client as SRPCClient } from 'starpc'
import { FSHandle } from './handle.js'
import type { HandleUploadTreeRequest } from './handle.pb.js'

const uploadTreeMock = vi.hoisted(() => vi.fn())

vi.mock('./handle_srpc.pb.js', () => ({
  FSHandleResourceServiceClient: class {
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

function resetUploadTreeMock(
  sent: HandleUploadTreeRequest[],
): void {
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

function chunkStream(chunks: number[][]): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(new Uint8Array(chunk))
      }
      controller.close()
    },
  })
}

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
            [1, 2, 3],
            [4, 5, 6, 7],
            [8, 9, 10],
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
    expect(
      sent.flatMap((request) =>
        request.body?.case === 'data' ? [Array.from(request.body.value)] : [],
      ),
    ).toEqual([
      [1, 2, 3],
      [4, 5, 6, 7],
      [8, 9, 10],
    ])
    expect(fileProgress).toEqual([3n, 7n, 10n])
    expect(totalProgress).toEqual([3n, 7n, 10n])
  })
})
