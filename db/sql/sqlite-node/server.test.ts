import assert from 'node:assert/strict'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'

import { SqliteNodeServer } from './server.js'
import type { QueryResponse } from '../sqlite-wasm/rpc/sqlite-bridge.pb.js'

// snapshot consumes the same column-first stream as the Go SQL driver.
async function snapshot(
  server: SqliteNodeServer,
  dbId: number,
  sql: string,
): Promise<QueryResponse[]> {
  const messages: QueryResponse[] = []
  for await (const message of server.Query({ dbId, sql }))
    messages.push(message)
  return messages
}

test('Node SQLite retains committed values and isolates physical transactions', async (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'spacewave-sqlite-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const path = join(directory, 'records.db')
  const server = new SqliteNodeServer()
  t.after(() => server.close())
  const first = (await server.OpenDb({ path })).dbId!
  const second = (await server.OpenDb({ path })).dbId!
  await server.Exec({
    dbId: first,
    sql: 'CREATE TABLE records (id INTEGER PRIMARY KEY, text TEXT, data BLOB)',
  })
  await server.Exec({ dbId: first, sql: 'BEGIN IMMEDIATE' })
  await server.Exec({
    dbId: first,
    sql: 'INSERT INTO records VALUES (?, ?, ?)',
    params: [
      { value: { case: 'intValue', value: 9007199254740993n } },
      {},
      { value: { case: 'blobValue', value: new Uint8Array([0, 128, 255]) } },
    ],
  })
  assert.equal(
    (await snapshot(server, second, 'SELECT * FROM records')).length,
    1,
  )
  await assert.rejects(
    server.Exec({ dbId: second, sql: 'BEGIN IMMEDIATE' }),
    /locked/,
  )
  await server.Exec({ dbId: first, sql: 'COMMIT' })
  const values = await snapshot(
    server,
    second,
    'SELECT id AS value, text AS value, data FROM records',
  )
  assert.deepEqual(values[0].columnNames, ['value', 'value', 'data'])
  assert.deepEqual(values[1].row, [
    { value: { case: 'intValue', value: 9007199254740993n } },
    {},
    { value: { case: 'blobValue', value: new Uint8Array([0, 128, 255]) } },
  ])
  for (const id of [first, second]) {
    assert.equal(
      (await snapshot(server, id, 'PRAGMA synchronous'))[1].row![0].value!
        .value,
      2n,
    )
    assert.equal(
      (await snapshot(server, id, 'PRAGMA journal_mode'))[1].row![0].value!
        .value,
      'wal',
    )
  }
  server.close()

  const reopened = new SqliteNodeServer()
  t.after(() => reopened.close())
  const id = (await reopened.OpenDb({ path })).dbId!
  assert.deepEqual(
    (await snapshot(reopened, id, 'SELECT * FROM records'))[1].row,
    values[1].row,
  )
  await reopened.Exec({ dbId: id, sql: 'BEGIN IMMEDIATE' })
  await reopened.Exec({ dbId: id, sql: 'DELETE FROM records' })
  await reopened.CloseDb({ dbId: id })
  const afterRollback = (await reopened.OpenDb({ path })).dbId!
  assert.equal(
    (await snapshot(reopened, afterRollback, 'SELECT * FROM records')).length,
    2,
  )
})

test('Node SQLite releases interrupted query cursors and propagates failures', async (t) => {
  const server = new SqliteNodeServer()
  t.after(() => server.close())
  const id = (await server.OpenDb({ path: ':memory:' })).dbId!
  await assert.rejects(
    server.Exec({ dbId: id, sql: 'INSERT INTO missing VALUES (1)' }),
    /no such table/,
  )
  const abort = new AbortController()
  const stream = server
    .Query({ dbId: id, sql: 'SELECT 1 UNION ALL SELECT 2' }, abort.signal)
    [Symbol.asyncIterator]()
  await stream.next()
  abort.abort(new Error('consumer stopped'))
  await assert.rejects(stream.next(), /consumer stopped/)
  await server.Exec({ dbId: id, sql: 'BEGIN IMMEDIATE' })
  await server.Exec({ dbId: id, sql: 'ROLLBACK' })
  server.close()
  await assert.rejects(server.Exec({ dbId: id, sql: 'SELECT 1' }), /closed/)
  await assert.rejects(server.OpenDb({ path: ':memory:' }), /closed/)
})
