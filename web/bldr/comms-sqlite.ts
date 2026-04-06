// comms-sqlite.ts manages the cross-tab communication sqlite database connection
// for plugin workers. Initializes AsyncOpfsDb, listens on BroadcastChannel for
// change notifications, and provides access to the CommsReader for consuming
// cross-tab messages.
//
// Used by both SharedWorker and DedicatedWorker plugin hosts. In DedicatedWorker
// context, can also write via CommsWriter with sync OPFS. In SharedWorker
// context, read-only via async OPFS deserialization.

import type { Sqlite3Static } from '@aptre/sqlite-wasm'

import {
  AsyncOpfsDb,
  COMMS_BROADCAST_CHANNEL,
  type CommsNotification,
} from '../runtime/wasm/sqlite/async-opfs.js'
import { CommsReader, COMMS_DB_FILENAME } from './comms-table.js'

// CommsSqliteOpts configures the cross-tab communication database.
export interface CommsSqliteOpts {
  // sqlite3 is the initialized sqlite3 API.
  sqlite3: Sqlite3Static
  // pluginId is this plugin's numeric ID for message filtering.
  pluginId: number
  // onNewMessages is called when new messages arrive for this plugin.
  onNewMessages?: () => void
}

// CommsSqlite manages the cross-tab sqlite communication database.
export class CommsSqlite {
  private asyncDb: AsyncOpfsDb
  private reader: CommsReader
  private channel: BroadcastChannel | null = null
  private pluginId: number
  private onNewMessages?: () => void
  private closed = false

  constructor(opts: CommsSqliteOpts) {
    this.asyncDb = new AsyncOpfsDb(opts.sqlite3, COMMS_DB_FILENAME)
    this.reader = new CommsReader()
    this.pluginId = opts.pluginId
    this.onNewMessages = opts.onNewMessages
  }

  // open initializes the database and starts listening for notifications.
  async open(): Promise<void> {
    await this.asyncDb.open()

    this.channel = new BroadcastChannel(COMMS_BROADCAST_CHANNEL)
    this.channel.onmessage = (ev: MessageEvent<CommsNotification>) => {
      if (this.closed) return
      if (ev.data?.table === 'messages') {
        this.handleNotification()
      }
    }

    console.log('comms-sqlite: opened, pluginId:', this.pluginId)
  }

  // handleNotification refreshes the database and checks for new messages.
  private handleNotification(): void {
    this.asyncDb
      .refresh()
      .then(() => {
        const db = this.asyncDb.getDb()
        if (!db) return
        const messages = this.reader.readNew(db, this.pluginId)
        if (messages.length > 0) {
          console.log(
            'comms-sqlite: received',
            messages.length,
            'new messages for plugin',
            this.pluginId,
          )
          this.onNewMessages?.()
        }
      })
      .catch((err) => {
        console.warn('comms-sqlite: refresh failed:', err)
      })
  }

  // getReader returns the CommsReader for accessing messages.
  getReader(): CommsReader {
    return this.reader
  }

  // getDb returns the current async database, or null if not loaded.
  getDb(): AsyncOpfsDb {
    return this.asyncDb
  }

  // close disposes the database and BroadcastChannel.
  close(): void {
    this.closed = true
    if (this.channel) {
      this.channel.close()
      this.channel = null
    }
    this.asyncDb.close()
  }
}

// initCommsSqlite creates and opens a CommsSqlite instance.
export async function initCommsSqlite(
  opts: CommsSqliteOpts,
): Promise<CommsSqlite> {
  const comms = new CommsSqlite(opts)
  await comms.open()
  return comms
}
