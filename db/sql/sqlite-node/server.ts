import { rmSync } from 'node:fs'
import { DatabaseSync } from 'node:sqlite'
import type { SQLInputValue, SQLOutputValue } from 'node:sqlite'
import type { MessageStream } from 'starpc'

import type { SqlValue } from '../sql.pb.js'
import type { SqliteBridge } from '../sqlite-wasm/rpc/sqlite-bridge_srpc.pb.js'
import type {
  CloseDbRequest,
  CloseDbResponse,
  DeleteDbRequest,
  DeleteDbResponse,
  ExecRequest,
  ExecResponse,
  OpenDbRequest,
  OpenDbResponse,
  QueryRequest,
  QueryResponse,
} from '../sqlite-wasm/rpc/sqlite-bridge.pb.js'

// encodeValue retains SQLite nulls, integers, and binary values on the bridge.
function encodeValue(value: SQLOutputValue): SqlValue {
  if (value === null) return {}
  if (typeof value === 'string') return { value: { case: 'strValue', value } }
  if (typeof value === 'bigint') return { value: { case: 'intValue', value } }
  if (typeof value === 'number') return { value: { case: 'floatValue', value } }
  return { value: { case: 'blobValue', value: new Uint8Array(value) } }
}

// decodeValue maps the generated SQL union to a positional SQLite binding.
function decodeValue(parameter: SqlValue): SQLInputValue {
  const value = parameter.value
  if (!value || value.case === undefined) return null
  return value.value
}

// SqliteNodeServer owns independent physical connections for one engine Worker.
// Every connection uses WAL and FULL synchronization before serving SQL.
export class SqliteNodeServer implements SqliteBridge {
  private nextId = 1
  private readonly handles = new Map<
    number,
    { path: string; db: DatabaseSync }
  >()
  private closed = false

  // OpenDb opens a connection without sharing another handle's transaction.
  async OpenDb(
    request: OpenDbRequest,
    signal?: AbortSignal,
  ): Promise<OpenDbResponse> {
    signal?.throwIfAborted()
    if (this.closed) throw new Error('SQLite bridge is closed')
    if (!request.path) throw new Error('SQLite path is required')
    const db = new DatabaseSync(request.path, {
      enableForeignKeyConstraints: true,
    })
    try {
      // Busy waits must not block the Worker that owns another transaction.
      db.exec(
        'PRAGMA busy_timeout=0; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL',
      )
      const id = this.nextId++
      this.handles.set(id, { db, path: request.path })
      return { dbId: id }
    } catch (error) {
      db.close()
      throw error
    }
  }

  // CloseDb rolls back an unfinished transaction and releases its connection.
  async CloseDb(request: CloseDbRequest): Promise<CloseDbResponse> {
    const handle = this.handles.get(request.dbId ?? 0)
    if (handle) {
      handle.db.close()
      this.handles.delete(request.dbId ?? 0)
    }
    return {}
  }

  // Exec executes one parameterized statement on its owning connection.
  async Exec(
    request: ExecRequest,
    signal?: AbortSignal,
  ): Promise<ExecResponse> {
    signal?.throwIfAborted()
    const statement = this.database(request.dbId).prepare(request.sql ?? '')
    statement.setReadBigInts(true)
    const result = statement.run(...(request.params ?? []).map(decodeValue))
    return {
      changes: BigInt(result.changes),
      lastInsertRowId: BigInt(result.lastInsertRowid),
    }
  }

  // Query streams positional rows and releases its active cursor on cancellation.
  async *Query(
    request: QueryRequest,
    signal?: AbortSignal,
  ): MessageStream<QueryResponse> {
    signal?.throwIfAborted()
    const statement = this.database(request.dbId).prepare(request.sql ?? '')
    statement.setReadBigInts(true)
    statement.setReturnArrays(true)
    const iterator = statement.iterate(
      ...(request.params ?? []).map(decodeValue),
    )
    try {
      yield { columnNames: statement.columns().map((column) => column.name) }
      for (const row of iterator) {
        signal?.throwIfAborted()
        yield { row: (row as unknown as SQLOutputValue[]).map(encodeValue) }
      }
    } finally {
      iterator.return?.()
    }
  }

  // DeleteDb requires all handles to be closed before removing a database.
  async DeleteDb(
    request: DeleteDbRequest,
    signal?: AbortSignal,
  ): Promise<DeleteDbResponse> {
    signal?.throwIfAborted()
    const path = request.path
    if (!path || path === ':memory:')
      throw new Error('A persistent SQLite path is required')
    for (const handle of this.handles.values()) {
      if (handle.path === path)
        throw new Error('Close the SQLite database before deleting it')
    }
    for (const suffix of ['', '-wal', '-shm'])
      rmSync(path + suffix, { force: true })
    return {}
  }

  // close releases every owned connection, including abandoned transactions.
  close(): void {
    this.closed = true
    const failures: unknown[] = []
    for (const [id, { db }] of this.handles) {
      try {
        db.close()
        this.handles.delete(id)
      } catch (error) {
        failures.push(error)
      }
    }
    if (failures.length)
      throw new AggregateError(failures, 'Could not close SQLite connections')
  }

  // database resolves the physical connection represented by a bridge handle.
  private database(id?: number): DatabaseSync {
    const handle = this.handles.get(id ?? 0)
    if (!handle) throw new Error('SQLite connection is closed or unknown')
    return handle.db
  }
}
