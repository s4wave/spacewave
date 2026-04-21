import { describe, it, expect, beforeAll, afterAll } from 'vitest'
import sqlite3Init from '@sqlite.org/sqlite-wasm'
import type { Sqlite3Static } from '@sqlite.org/sqlite-wasm'
import { SqliteBridgeServer } from './server.js'
import type { QueryResponse } from './sqlite-bridge.pb.js'

interface SqliteBridgeServerInternals {
  databasesByPath: Map<string, { refCount: number }>
  handles: Map<number, unknown>
}

describe('SqliteBridgeServer', () => {
  let sqlite3: Sqlite3Static
  let server: SqliteBridgeServer

  beforeAll(async () => {
    sqlite3 = await sqlite3Init()
    server = new SqliteBridgeServer(sqlite3, '')
  })

  afterAll(() => {
    // Close any open databases.
  })

  it('opens a database, creates table, inserts, and queries', async () => {
    // Open an in-memory database.
    const openResp = await server.OpenDb({ path: ':memory:' })
    const dbId = openResp.dbId
    expect(dbId).toBeGreaterThan(0)

    // Create table.
    const createResp = await server.Exec({
      dbId,
      sql: 'CREATE TABLE test (key BLOB PRIMARY KEY, value BLOB)',
      params: [],
    })
    expect(createResp).toBeDefined()

    // Insert rows.
    const key1 = new Uint8Array([1, 2, 3])
    const val1 = new Uint8Array([10, 20, 30])
    const insert1 = await server.Exec({
      dbId,
      sql: 'INSERT INTO test (key, value) VALUES (?, ?)',
      params: [
        { value: { case: 'blobValue', value: key1 } },
        { value: { case: 'blobValue', value: val1 } },
      ],
    })
    expect(Number(insert1.changes)).toBe(1)

    const key2 = new Uint8Array([4, 5, 6])
    const val2 = new Uint8Array([40, 50, 60])
    await server.Exec({
      dbId,
      sql: 'INSERT INTO test (key, value) VALUES (?, ?)',
      params: [
        { value: { case: 'blobValue', value: key2 } },
        { value: { case: 'blobValue', value: val2 } },
      ],
    })

    // Query rows back via streaming.
    const stream = server.Query({
      dbId,
      sql: 'SELECT key, value FROM test ORDER BY key',
      params: [],
    })

    const messages: QueryResponse[] = []
    for await (const msg of stream) {
      messages.push(msg)
    }

    // First message: column names.
    expect(messages[0].columnNames).toEqual(['key', 'value'])
    expect(messages[0].row?.length).toBe(0)

    // Two data rows.
    expect(messages.length).toBe(3)

    // Row 1: key1, val1.
    const row1 = messages[1].row!
    expect(row1[0].value?.case).toBe('blobValue')
    expect(row1[1].value?.case).toBe('blobValue')

    // Row 2: key2, val2.
    const row2 = messages[2].row!
    expect(row2[0].value?.case).toBe('blobValue')
    expect(row2[1].value?.case).toBe('blobValue')

    // Close database.
    await server.CloseDb({ dbId })
  })

  it('handles integer and text values', async () => {
    const openResp = await server.OpenDb({ path: ':memory:' })
    const dbId = openResp.dbId

    await server.Exec({
      dbId,
      sql: 'CREATE TABLE nums (id INTEGER PRIMARY KEY, name TEXT, score REAL)',
    })

    await server.Exec({
      dbId,
      sql: 'INSERT INTO nums (id, name, score) VALUES (?, ?, ?)',
      params: [
        { value: { case: 'intValue', value: 42n } },
        { value: { case: 'strValue', value: 'alice' } },
        { value: { case: 'floatValue', value: 3.14 } },
      ],
    })

    const stream = server.Query({
      dbId,
      sql: 'SELECT id, name, score FROM nums',
    })

    const messages: QueryResponse[] = []
    for await (const msg of stream) {
      messages.push(msg)
    }

    expect(messages[0].columnNames).toEqual(['id', 'name', 'score'])
    expect(messages.length).toBe(2)

    const row = messages[1].row!
    expect(row[0].value?.case).toBe('intValue')
    expect(Number(row[0].value?.value)).toBe(42)
    expect(row[1].value?.case).toBe('strValue')
    expect(row[1].value?.value).toBe('alice')
    expect(row[2].value?.case).toBe('floatValue')
    expect(row[2].value?.value).toBeCloseTo(3.14)

    await server.CloseDb({ dbId })
  })

  it('preserves bigint bind parameters without narrowing to number', async () => {
    const openResp = await server.OpenDb({ path: ':memory:' })
    const dbId = openResp.dbId

    await server.Exec({
      dbId,
      sql: 'CREATE TABLE bigs (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)',
    })

    const expected = 9007199254740993n
    await server.Exec({
      dbId,
      sql: 'INSERT INTO bigs (id, value) VALUES (?, ?)',
      params: [
        { value: { case: 'intValue', value: 1n } },
        { value: { case: 'intValue', value: expected } },
      ],
    })

    const stream = server.Query({
      dbId,
      sql: 'SELECT value FROM bigs WHERE id = ?',
      params: [{ value: { case: 'intValue', value: 1n } }],
    })

    const messages: QueryResponse[] = []
    for await (const msg of stream) {
      messages.push(msg)
    }

    expect(messages[0].columnNames).toEqual(['value'])
    expect(messages.length).toBe(2)
    expect(messages[1].row?.[0].value?.case).toBe('intValue')
    expect(messages[1].row?.[0].value?.value).toBe(expected)

    await server.CloseDb({ dbId })
  })

  it('handles transactions via exec', async () => {
    const openResp = await server.OpenDb({ path: ':memory:' })
    const dbId = openResp.dbId

    await server.Exec({ dbId, sql: 'CREATE TABLE txtest (k TEXT)' })

    // Begin, insert, rollback.
    await server.Exec({ dbId, sql: 'BEGIN IMMEDIATE' })
    await server.Exec({
      dbId,
      sql: 'INSERT INTO txtest (k) VALUES (?)',
      params: [{ value: { case: 'strValue', value: 'should-not-exist' } }],
    })
    await server.Exec({ dbId, sql: 'ROLLBACK' })

    // Verify row was rolled back.
    const stream = server.Query({
      dbId,
      sql: 'SELECT COUNT(*) as cnt FROM txtest',
    })
    const messages: QueryResponse[] = []
    for await (const msg of stream) {
      messages.push(msg)
    }
    expect(Number(messages[1].row![0].value?.value)).toBe(0)

    await server.CloseDb({ dbId })
  })

  it('reuses one physical database per path with logical handle refcounts', async () => {
    const open1 = await server.OpenDb({ path: '/shared.db' })
    const open2 = await server.OpenDb({ path: '/shared.db' })
    const dbId1 = open1.dbId!
    const dbId2 = open2.dbId!
    const internals = server as unknown as SqliteBridgeServerInternals

    expect(internals.databasesByPath.size).toBe(1)
    const shared = internals.databasesByPath.get('/shared.db')
    expect(shared).toBeTruthy()
    expect(shared.refCount).toBe(2)

    await server.Exec({ dbId: dbId1, sql: 'CREATE TABLE t (v TEXT)' })
    await server.Exec({ dbId: dbId2, sql: "INSERT INTO t(v) VALUES ('x')" })

    const rows: string[] = []
    for await (const msg of server.Query({
      dbId: dbId1,
      sql: 'SELECT v FROM t ORDER BY rowid',
    })) {
      if (msg.row?.length) {
        const value = msg.row[0]?.value
        if (value?.case === 'strValue') {
          rows.push(value.value)
        }
      }
    }
    expect(rows).toEqual(['x'])

    await server.CloseDb({ dbId: dbId1 })
    expect(internals.databasesByPath.get('/shared.db')?.refCount).toBe(1)

    await server.Exec({ dbId: dbId2, sql: "INSERT INTO t(v) VALUES ('y')" })

    const rowsAfterClose: string[] = []
    for await (const msg of server.Query({
      dbId: dbId2,
      sql: 'SELECT v FROM t ORDER BY rowid',
    })) {
      if (msg.row?.length) {
        const value = msg.row[0]?.value
        if (value?.case === 'strValue') {
          rowsAfterClose.push(value.value)
        }
      }
    }
    expect(rowsAfterClose).toEqual(['x', 'y'])

    await server.CloseDb({ dbId: dbId2 })
    expect(internals.databasesByPath.size).toBe(0)
    expect(internals.handles.size).toBe(0)
  })
})
