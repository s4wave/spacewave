// comms-table.ts implements the cross-tab message table for plugin communication.
//
// Schema: a single "messages" table in the cross-tab sqlite database. Writers
// (DedicatedWorker plugins) INSERT rows and post BroadcastChannel notifications.
// Readers (other tabs/workers) SELECT new rows by sequence number and DELETE
// consumed ones.
//
// The database file lives in OPFS at .bldr-comms/comms.db. DedicatedWorkers
// access it via sync OPFS VFS. SharedWorker reads it via AsyncOpfsDb.

import type { Database, Sqlite3Static } from '@aptre/sqlite-wasm'
import { COMMS_BROADCAST_CHANNEL, type CommsNotification } from '../runtime/wasm/sqlite/async-opfs.js'

// COMMS_DB_FILENAME is the database filename within the OPFS comms directory.
export const COMMS_DB_FILENAME = 'comms.db'

// MESSAGES_TABLE is the table name for cross-tab plugin messages.
const MESSAGES_TABLE = 'messages'

// MESSAGES_DDL creates the messages table if it does not exist.
const MESSAGES_DDL = `CREATE TABLE IF NOT EXISTS ${MESSAGES_TABLE} (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_plugin_id INTEGER NOT NULL,
  target_plugin_id INTEGER NOT NULL,
  payload BLOB NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
)`

// CommsMessage represents a row in the messages table.
export interface CommsMessage {
  id: number
  sourcePluginId: number
  targetPluginId: number
  payload: Uint8Array
  createdAt: number
}

// CommsWriter writes messages to the cross-tab sqlite database and posts
// BroadcastChannel notifications. Used by DedicatedWorker plugins with sync
// OPFS access.
export class CommsWriter {
  private db: Database
  private channel: BroadcastChannel
  private seq = 0

  constructor(db: Database) {
    this.db = db
    this.channel = new BroadcastChannel(COMMS_BROADCAST_CHANNEL)
    this.db.exec(MESSAGES_DDL)
  }

  // write inserts a message and notifies listeners.
  write(sourcePluginId: number, targetPluginId: number, payload: Uint8Array): number {
    this.db.exec({
      sql: `INSERT INTO ${MESSAGES_TABLE} (source_plugin_id, target_plugin_id, payload) VALUES (?, ?, ?)`,
      bind: [sourcePluginId, targetPluginId, payload],
    })
    const id = Number(this.db.exec({
      sql: 'SELECT last_insert_rowid()',
      returnValue: 'resultRows',
    })[0][0])

    this.seq++
    const notification: CommsNotification = {
      table: MESSAGES_TABLE,
      seq: this.seq,
    }
    this.channel.postMessage(notification)
    return id
  }

  // close releases the BroadcastChannel.
  close(): void {
    this.channel.close()
  }
}

// CommsReader reads messages from the cross-tab sqlite database. Works with
// both sync databases (DedicatedWorker) and AsyncOpfsDb (SharedWorker).
export class CommsReader {
  private lastId = 0

  // readNew returns messages with id > lastId for the given target plugin.
  // Updates lastId to the highest id seen.
  readNew(db: Database, targetPluginId: number): CommsMessage[] {
    const rows = db.exec({
      sql: `SELECT id, source_plugin_id, target_plugin_id, payload, created_at
            FROM ${MESSAGES_TABLE}
            WHERE target_plugin_id = ? AND id > ?
            ORDER BY id ASC`,
      bind: [targetPluginId, this.lastId],
      returnValue: 'resultRows',
    })

    const messages: CommsMessage[] = []
    for (const row of rows) {
      const msg: CommsMessage = {
        id: row[0] as number,
        sourcePluginId: row[1] as number,
        targetPluginId: row[2] as number,
        payload: row[3] as Uint8Array,
        createdAt: row[4] as number,
      }
      messages.push(msg)
      if (msg.id > this.lastId) {
        this.lastId = msg.id
      }
    }
    return messages
  }

  // deleteConsumed removes messages that have been read (id <= lastId).
  // Call periodically to prevent unbounded table growth.
  deleteConsumed(db: Database, targetPluginId: number): void {
    db.exec({
      sql: `DELETE FROM ${MESSAGES_TABLE} WHERE target_plugin_id = ? AND id <= ?`,
      bind: [targetPluginId, this.lastId],
    })
  }
}

// initCommsSchema ensures the messages table exists. Call from any context
// that opens the comms database.
export function initCommsSchema(db: Database): void {
  db.exec(MESSAGES_DDL)
}
