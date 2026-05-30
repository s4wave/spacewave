import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { SshHostResourceServiceClient } from './sshhost_srpc.pb.js'
import type { SshHost, WatchSshHostStateResponse } from './sshhost.pb.js'

// SshHostTypeID is the type identifier for SSH-only Host objects.
export const SshHostTypeID = 'spacewave/ssh-host'

// ISshHostHandle contains the SshHostHandle interface.
export interface ISshHostHandle {
  // watchSshHostState streams SSH Host state changes.
  watchSshHostState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<SshHost | undefined>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// SshHostHandle represents a handle to an SSH Host resource.
export class SshHostHandle extends Resource implements ISshHostHandle {
  private service: SshHostResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SshHostResourceServiceClient(resourceRef.client)
  }

  // watchSshHostState streams SSH Host state changes.
  public async *watchSshHostState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<SshHost | undefined> {
    const stream = this.service.WatchSshHostState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchSshHostStateResponse>) {
      yield resp.state
    }
  }
}
