import type { Database, Sqlite3Static, SqlValue } from '@sqlite.org/sqlite-wasm'

interface SharedDatabaseEntry {
  path: string
  db: Database
  refCount: number
  logicalIds: Set<number>
}

interface DatabaseHandleEntry {
  path: string
  db: Database
  cacheKey: string | null
}
import type { Message } from '@aptre/protobuf-es-lite'
import type { MessageStream } from 'starpc'
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
  SqlValue as ProtoSqlValue,
} from './sqlite-bridge.pb.js'
import type { SqliteBridge } from './sqlite-bridge_srpc.pb.js'

// sqlValueToProto converts a sqlite.wasm SqlValue to a proto SqlValue.
function sqlValueToProto(v: SqlValue): Message<ProtoSqlValue> {
  if (v === null) {
    return { value: { case: undefined } }
  }
  if (typeof v === 'string') {
    return { value: { case: 'strValue', value: v } }
  }
  if (typeof v === 'number') {
    if (Number.isInteger(v)) {
      return { value: { case: 'intValue', value: BigInt(v) } }
    }
    return { value: { case: 'floatValue', value: v } }
  }
  if (typeof v === 'bigint') {
    return { value: { case: 'intValue', value: v } }
  }
  if (v instanceof Uint8Array || v instanceof Int8Array || v instanceof ArrayBuffer) {
    const bytes = v instanceof ArrayBuffer ? new Uint8Array(v) : new Uint8Array(v.buffer, v.byteOffset, v.byteLength)
    return { value: { case: 'blobValue', value: bytes } }
  }
  return { value: { case: undefined } }
}

// protoToBindable converts proto SqlValue params to a sqlite.wasm BindingSpec.
function protoToBindable(params: Message<ProtoSqlValue>[]): SqlValue[] {
  return params.map((p) => {
    const v = p.value
    if (!v || v.case === undefined) {
      return null
    }
    switch (v.case) {
      case 'intValue':
        return v.value
      case 'floatValue':
        return v.value
      case 'strValue':
        return v.value
      case 'blobValue':
        return v.value
      default:
        return null
    }
  })
}

// SqliteBridgeServer implements the SqliteBridge starpc service.
// Wraps sqlite.wasm's OO1 Database API.
export class SqliteBridgeServer implements SqliteBridge {
  private sqlite3: Sqlite3Static
  private vfsName: string
  private nextId = 1
  private handles = new Map<number, DatabaseHandleEntry>()
  private databasesByPath = new Map<string, SharedDatabaseEntry>()

  constructor(sqlite3: Sqlite3Static, vfsName: string) {
    this.sqlite3 = sqlite3
    this.vfsName = vfsName
  }

  // getCacheKey returns the cache key for a path, or null for paths that must
  // never share a physical database handle.
  private getCacheKey(path: string): string | null {
    const normalized = path || ':memory:'
    if (normalized === ':memory:') {
      return null
    }
    return normalized
  }

  // dispose closes all open physical databases owned by this bridge instance.
  async dispose(): Promise<void> {
    for (const shared of this.databasesByPath.values()) {
      shared.db.close()
    }
    for (const handle of this.handles.values()) {
      if (handle.cacheKey === null) {
        handle.db.close()
      }
    }
    this.handles.clear()
    this.databasesByPath.clear()
  }

  // OpenDb opens or creates a logical database handle for the given path.
  async OpenDb(request: OpenDbRequest): Promise<Message<OpenDbResponse>> {
    const path = request.path || ':memory:'
    const cacheKey = this.getCacheKey(path)
    const id = this.nextId++

    if (cacheKey !== null) {
      let shared = this.databasesByPath.get(cacheKey)
      if (!shared) {
        shared = {
          path,
          db: new this.sqlite3.oo1.DB({
            filename: path,
            flags: 'cw',
            vfs: this.vfsName,
          }),
          refCount: 0,
          logicalIds: new Set<number>(),
        }
        this.databasesByPath.set(cacheKey, shared)
      }
      shared.refCount += 1
      shared.logicalIds.add(id)
      this.handles.set(id, { path, db: shared.db, cacheKey })
      return { dbId: id }
    }

    const db = new this.sqlite3.oo1.DB({
      filename: path,
      flags: 'cw',
      vfs: this.vfsName,
    })
    this.handles.set(id, { path, db, cacheKey: null })
    return { dbId: id }
  }

  // closeHandle closes a logical database handle and releases the underlying
  // physical database when the last shared reference is dropped.
  private closeHandle(dbId: number | undefined): void {
    const id = dbId || 0
    const handle = this.handles.get(id)
    if (!handle) {
      return
    }
    this.handles.delete(id)

    if (handle.cacheKey === null) {
      handle.db.close()
      return
    }

    const shared = this.databasesByPath.get(handle.cacheKey)
    if (!shared) {
      return
    }
    shared.logicalIds.delete(id)
    shared.refCount -= 1
    if (shared.refCount <= 0) {
      shared.db.close()
      this.databasesByPath.delete(handle.cacheKey)
    }
  }

  // CloseDb closes a logical database handle.
  async CloseDb(request: CloseDbRequest): Promise<Message<CloseDbResponse>> {
    this.closeHandle(request.dbId)
    return {}
  }

  // getDb returns the database for the given ID or throws.
  private getDb(dbId: number | undefined): Database {
    const handle = this.handles.get(dbId || 0)
    if (!handle) {
      throw new Error(`database ${dbId} not found`)
    }
    return handle.db
  }

  // Exec executes a DDL/DML statement.
  async Exec(request: ExecRequest): Promise<Message<ExecResponse>> {
    const db = this.getDb(request.dbId)
    const sql = request.sql || ''
    const bind = request.params?.length ? protoToBindable(request.params) : undefined
    db.exec({ sql, bind })
    const ptr = db.pointer
    return {
      changes: BigInt(db.changes()),
      lastInsertRowId: ptr
        ? BigInt(this.sqlite3.capi.sqlite3_last_insert_rowid(ptr))
        : 0n,
    }
  }

  // Query executes a SELECT and streams rows lazily as the driver consumes them.
  async *Query(request: QueryRequest): MessageStream<QueryResponse> {
    const db = this.getDb(request.dbId)
    const sql = request.sql || ''
    const bind = request.params?.length ? protoToBindable(request.params) : undefined
    const columnNames: string[] = []
    const stmt = db.prepare(sql)
    try {
      if (bind) {
        stmt.bind(bind)
      }
      const colCount = stmt.columnCount
      for (let i = 0; i < colCount; i++) {
        columnNames.push(stmt.getColumnName(i) ?? `col${i}`)
      }

      // First message: column names.
      yield { columnNames, row: [] }

      while (stmt.step()) {
        const row: SqlValue[] = new Array(colCount)
        for (let i = 0; i < colCount; i++) {
          row[i] = stmt.get(i) as SqlValue
        }
        yield { columnNames: [], row: row.map(sqlValueToProto) }
      }
    } finally {
      stmt.finalize()
    }
  }

  // DeleteDb deletes a database file.
  async DeleteDb(request: DeleteDbRequest): Promise<Message<DeleteDbResponse>> {
    const path = request.path || ''
    const cacheKey = this.getCacheKey(path)

    if (cacheKey !== null) {
      const shared = this.databasesByPath.get(cacheKey)
      if (shared) {
        shared.db.close()
        for (const logicalId of shared.logicalIds) {
          this.handles.delete(logicalId)
        }
        this.databasesByPath.delete(cacheKey)
      }
    }

    for (const [id, handle] of this.handles) {
      if (handle.path === path) {
        handle.db.close()
        this.handles.delete(id)
      }
    }

    // Use capi to unlink the database file from the VFS.
    const capi = this.sqlite3.capi as Record<string, unknown>
    const unlink = capi['sqlite3_wasm_vfs_unlink'] as
      | ((vfs: string, path: string) => number)
      | undefined
    if (unlink) {
      unlink(this.vfsName, path)
    }
    return {}
  }
}
