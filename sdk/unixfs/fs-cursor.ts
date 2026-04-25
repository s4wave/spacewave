import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { FSCursorServiceClient } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js'
import type { FSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/fs-handle.js'
import { buildFSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js'

// FSCursorResource wraps a remote FSCursorService resource into a Resource.
// On the Go side, this maps 1:1 to an FSCursorResource which serves all 22
// FSCursorService RPCs over a resource mux.
//
// The resource provides two levels of access:
//  - getServiceClient() returns the raw FSCursorServiceClient for direct RPC use
//  - buildFSHandle() constructs a hydra FSHandle backed by the resource's cursor
export class FSCursorResource extends Resource {
  private svc: FSCursorServiceClient
  private fsHandle: FSHandle | null = null
  private fsHandlePromise: Promise<FSHandle> | null = null

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.svc = new FSCursorServiceClient(resourceRef.client)
  }

  // getServiceClient returns the FSCursorServiceClient for direct RPC access.
  public getServiceClient(): FSCursorServiceClient {
    return this.svc
  }

  // getFSHandle returns the cached hydra FSHandle, building it on first access.
  // The handle manages the FSCursorClient session (streaming RPC) and provides
  // the full inode tree with automatic re-resolution on cursor releases.
  public async getFSHandle(abortSignal?: AbortSignal): Promise<FSHandle> {
    if (this.fsHandle) return this.fsHandle
    if (this.fsHandlePromise) return this.fsHandlePromise

    this.fsHandlePromise = buildFSHandle(this.svc, abortSignal).then(
      (handle) => {
        this.fsHandle = handle
        return handle
      },
      (err) => {
        this.fsHandlePromise = null
        throw err
      },
    )
    return this.fsHandlePromise
  }

  // release releases the resource and any cached FSHandle.
  public override release(): void {
    if (this.fsHandle) {
      this.fsHandle.release()
      this.fsHandle = null
    }
    this.fsHandlePromise = null
    super.release()
  }
}
