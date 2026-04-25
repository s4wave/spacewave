import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { PersistentExecutionServiceClient } from '@s4wave/sdk/process/process_srpc.pb.js'
import type { ExecuteStatus } from '@s4wave/sdk/process/process.pb.js'

// VmV86TypeID is the type identifier for V86 world objects.
export const VmV86TypeID = 'spacewave/vm/v86'

// IVmV86Handle contains the VmV86Handle interface.
export interface IVmV86Handle {
  // execute starts the persistent VM process and streams status updates.
  execute(abortSignal?: AbortSignal): AsyncIterable<ExecuteStatus>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// VmV86Handle represents a handle to a V86 typed object resource.
// Each instance maps 1:1 to a Go PersistentExecutionService on the backend.
export class VmV86Handle extends Resource implements IVmV86Handle {
  private service: PersistentExecutionServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new PersistentExecutionServiceClient(resourceRef.client)
  }

  // execute starts the persistent VM process and streams status updates.
  public async *execute(
    abortSignal?: AbortSignal,
  ): AsyncIterable<ExecuteStatus> {
    const stream = this.service.Execute({}, abortSignal)
    for await (const status of stream as AsyncIterable<ExecuteStatus>) {
      yield status
    }
  }
}
