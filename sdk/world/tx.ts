import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { TxResourceServiceClient } from './world_srpc.pb.js'
import { WorldStateResource } from './world-state.js'

// Tx represents a transaction against the world state.
// Tx implements the world state transaction interfaces (maps to Tx in Go).
//
// In the Go implementation (hydra/world/tx.go), Tx provides:
// - WorldState: full state read/write interface (inherited from WorldStateResource)
// - tx.Tx: Commit, Discard operations
//
// A Tx maintains state across multiple RPC calls, enabling complex multi-step
// operations within a single transaction. Always call discard() when done.
//
// Concurrent calls to WorldState functions should be supported.
export class Tx extends WorldStateResource {
  private txService: TxResourceServiceClient

  constructor(resourceRef: ClientResourceRef, meta?: { readOnly?: boolean }) {
    super(resourceRef, meta)
    this.txService = new TxResourceServiceClient(resourceRef.client)
  }

  // Transaction operations (tx.Tx interface)

  // Commit commits the transaction.
  // After commit, the transaction should be discarded.
  public async commit(abortSignal?: AbortSignal): Promise<void> {
    await this.txService.Commit({}, abortSignal)
  }

  // Discard discards the transaction without committing changes.
  // Always call this when done with the transaction.
  public async discard(abortSignal?: AbortSignal): Promise<void> {
    await this.txService.Discard({}, abortSignal)
  }
}
