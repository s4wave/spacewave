// comms-stream.ts implements a PacketStream backed by the cross-tab sqlite
// communication database. Used for cross-tab plugin-to-plugin RPC routing.
//
// Write path: CommsWriter INSERTs message rows and posts BroadcastChannel.
// Read path: BroadcastChannel notification triggers AsyncOpfsDb refresh,
// CommsReader reads new messages for this plugin and pushes to the source.
//
// This stream has higher latency than SAB (~13-20ms per OPFS write) but works
// across tabs where SAB sharing is not possible.

import type { Sink, Source, Duplex } from 'it-stream-types'
import { pushable } from 'it-pushable'
import type { Pushable } from 'it-pushable'

import type { Database } from '@aptre/sqlite-wasm'
import {
  COMMS_BROADCAST_CHANNEL,
  type CommsNotification,
} from '../runtime/wasm/sqlite/async-opfs.js'
import type { AsyncOpfsDb } from '../runtime/wasm/sqlite/async-opfs.js'
import { CommsWriter, CommsReader, initCommsSchema } from './comms-table.js'

// SqliteCommsStreamOpts configures a cross-tab sqlite-backed stream.
export interface SqliteCommsStreamOpts {
  // writeDb is a sync OPFS sqlite database for writes. Required for sending.
  // Null if this context is read-only (SharedWorker without sync OPFS).
  writeDb: Database | null
  // asyncDb is the async OPFS database for reading cross-tab messages.
  asyncDb: AsyncOpfsDb
  // sourcePluginId is this plugin's ID.
  sourcePluginId: number
  // targetPluginId is the remote plugin's ID.
  targetPluginId: number
}

// SqliteCommsStream implements a PacketStream over the sqlite cross-tab
// communication table. Satisfies the same Duplex interface as ChannelStream
// and SabRingStream for transparent StarPC integration.
export class SqliteCommsStream
  implements
    Duplex<AsyncGenerator<Uint8Array>, Source<Uint8Array>, Promise<void>>
{
  public source: AsyncGenerator<Uint8Array>
  public sink: Sink<Source<Uint8Array>, Promise<void>>

  private readonly _source: Pushable<Uint8Array>
  private readonly writer: CommsWriter | null
  private readonly reader: CommsReader
  private readonly asyncDb: AsyncOpfsDb
  private readonly sourcePluginId: number
  private readonly targetPluginId: number
  private channel: BroadcastChannel | null
  private closed = false

  constructor(opts: SqliteCommsStreamOpts) {
    this.sourcePluginId = opts.sourcePluginId
    this.targetPluginId = opts.targetPluginId
    this.asyncDb = opts.asyncDb
    this.reader = new CommsReader()

    // Set up writer if we have a writable database.
    if (opts.writeDb) {
      initCommsSchema(opts.writeDb)
      this.writer = new CommsWriter(opts.writeDb)
    } else {
      this.writer = null
    }

    // Set up pushable source for the read side.
    const source = pushable<Uint8Array>({ objectMode: true })
    this._source = source
    this.source = source
    this.sink = this._createSink()

    // Listen for BroadcastChannel notifications.
    this.channel = new BroadcastChannel(COMMS_BROADCAST_CHANNEL)
    this.channel.onmessage = (ev: MessageEvent<CommsNotification>) => {
      if (this.closed) return
      if (ev.data?.table === 'messages') {
        this._handleNotification()
      }
    }
  }

  // _handleNotification refreshes the async DB and pushes new messages.
  private _handleNotification(): void {
    this.asyncDb
      .refresh()
      .then(() => {
        const db = this.asyncDb.getDb()
        if (!db || this.closed) return
        const messages = this.reader.readNew(db, this.sourcePluginId)
        for (const msg of messages) {
          if (msg.sourcePluginId === this.targetPluginId) {
            this._source.push(msg.payload)
          }
        }
      })
      .catch((err) => {
        if (!this.closed) {
          console.warn('comms-stream: refresh failed:', err)
        }
      })
  }

  private _createSink(): Sink<Source<Uint8Array>, Promise<void>> {
    return async (source: Source<Uint8Array>) => {
      try {
        for await (const msg of source) {
          if (!this.writer) {
            throw new Error('comms-stream: no writable database')
          }
          this.writer.write(
            this.sourcePluginId,
            this.targetPluginId,
            msg instanceof Uint8Array ? msg : new Uint8Array(msg),
          )
        }
      } catch (err) {
        this.close(err instanceof Error ? err : new Error(String(err)))
      }
    }
  }

  // close tears down this stream.
  public close(error?: Error): void {
    if (this.closed) return
    this.closed = true
    if (this.channel) {
      this.channel.close()
      this.channel = null
    }
    if (this.writer) {
      this.writer.close()
    }
    this._source.end(error)
  }
}
