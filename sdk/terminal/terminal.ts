import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type { MessageStream } from 'starpc'

import { TerminalResourceServiceClient } from './terminal_srpc.pb.js'
import type { Terminal, TerminalFrame } from './terminal.pb.js'

// TerminalTypeID is the type identifier for remote Terminal objects.
export const TerminalTypeID = 'spacewave/terminal'

// RemoteShellProtocolID is the Bifrost protocol used for PTY-backed terminals.
export const RemoteShellProtocolID = 'alpha/remote-shell/v0'

// ITerminalHandle contains the TerminalHandle interface.
export interface ITerminalHandle {
  // watchTerminalState streams Terminal state changes.
  watchTerminalState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Terminal | undefined>

  // connectTerminal opens the terminal byte/control stream.
  connectTerminal(
    frames: MessageStream<TerminalFrame>,
    abortSignal?: AbortSignal,
  ): MessageStream<TerminalFrame>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// TerminalHandle represents a handle to a Terminal resource.
export class TerminalHandle extends Resource implements ITerminalHandle {
  private service: TerminalResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new TerminalResourceServiceClient(resourceRef.client)
  }

  // watchTerminalState streams Terminal state changes.
  public async *watchTerminalState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Terminal | undefined> {
    const stream = this.service.WatchTerminalState({}, abortSignal)
    for await (const resp of stream) {
      yield resp.state
    }
  }

  // connectTerminal opens the terminal byte/control stream.
  public connectTerminal(
    frames: MessageStream<TerminalFrame>,
    abortSignal?: AbortSignal,
  ): MessageStream<TerminalFrame> {
    return this.service.ConnectTerminal(frames, abortSignal)
  }
}
