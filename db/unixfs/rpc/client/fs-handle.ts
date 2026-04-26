import { FSHandle } from '../../fs-handle.js'
import type { FSCursorService } from '../rpc_srpc.pb.js'
import { RpcFSCursor } from './fs-cursor.js'

// FSHandleBuilder constructs a new FSHandle from a signal and release callback.
export type FSHandleBuilder = (
  signal: AbortSignal,
  released: () => void,
) => Promise<{ handle: FSHandle; release: () => void }>

// buildFSHandle constructs a root FSHandle from a FSCursorService client.
// The signal must remain active for the duration of the FSHandle's lifetime.
export function buildFSHandle(
  svcClient: FSCursorService,
  signal?: AbortSignal,
): Promise<FSHandle> {
  const cursor = new RpcFSCursor(
    signal ?? new AbortController().signal,
    svcClient,
  )
  try {
    return Promise.resolve(FSHandle.create(cursor))
  } catch (e) {
    cursor.release()
    throw e
  }
}

// newFSHandleBuilder constructs a new FSHandleBuilder for a FSCursorService client.
export function newFSHandleBuilder(
  svcClient: FSCursorService,
): FSHandleBuilder {
  return async (signal: AbortSignal, released: () => void) => {
    const handle = await buildFSHandle(svcClient, signal)
    handle.addReleaseCallback(released)
    return { handle, release: () => handle.release() }
  }
}
