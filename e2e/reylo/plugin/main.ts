import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { connectUnixResourceClient } from '@aptre/bldr-sdk/resource/unix-client.js'

import { reyloClient } from '../../../sdk/sql/reylo.js'
import { SqlDatabase, type SqlTransaction } from '../../../sdk/sql/sql.js'
import getItem from './queries/get_item.sql'
import insertItem from './queries/insert_item.sql'
import listItems from './queries/list_items.sql'
import renameItems from './queries/rename_items.sql'

// Equal is true when two types are identical.
type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends <T>() => T extends B ? 1 : 2
    ? true
    : false

// Row is a row of list_items as the engine reports it: the build fails when
// Reylo's type for the query changes.
type Row = {
  id: bigint
  name: string
  note: string | null
  big: bigint
  price: string
  data: Uint8Array
  flag: number
  meta: unknown
  created: string
  day: string
}
const typesAgree: [
  Equal<(typeof listItems)['~reylo']['element'], Row>,
  Equal<(typeof getItem)['~reylo']['element'], { name: string }>,
  Equal<(typeof getItem)['~reylo']['cardinality'], 'maybeOne'>,
  Equal<(typeof renameItems)['~reylo']['cardinality'], 'none'>,
] = [true, true, true, true]

// first and second are the rows the committed transaction writes. big is past
// 2^53, where a number stops being exact.
const first = {
  id: 1n,
  name: 'ada',
  note: null,
  big: 9007199254740993n,
  price: '12.30',
  data: new Uint8Array([0, 1, 254, 255]),
  flag: true,
  meta: { tags: ['a', 'b'], n: 1 },
  created: '2026-10-23 12:34:56',
  day: '2026-10-23',
}
const second = {
  id: 2n,
  name: 'grace',
  note: 'second',
  big: -5n,
  price: '-0.01',
  data: new Uint8Array(),
  flag: false,
  meta: null,
  created: '1999-12-31 23:59:59',
  day: '2000-01-01',
}

// main runs the plugin's queries against the sql/db served on the socket, one
// transaction per step, in the order the Go test expects.
async function main() {
  // Connect to the database the test serves and open it as the SDK does.
  const socket = process.argv[2]
  const conn = await connectUnixResourceClient(
    `unix://${socket}`,
    AbortSignal.timeout(30_000),
  )
  const db = new SqlDatabase(resourceRef(conn.rpc), '/app')
  const write = <T>(
    fn: (tx: SqlTransaction) => Promise<T>,
    signal?: AbortSignal,
  ) => db.withTransaction(true, '/app', fn, signal)
  const read = <T>(fn: (tx: SqlTransaction) => Promise<T>) =>
    db.withTransaction(false, '/app', fn)

  // Report the compiled statements so the Go test can match its log.
  console.log(
    JSON.stringify({
      statements: {
        insertItem: insertItem.sql,
        listItems: listItems.sql,
        getItem: getItem.sql,
        renameItems: renameItems.sql,
      },
    }),
  )

  // Commit two rows, and read them back inside the same transaction.
  const inside = await write(async (tx) => {
    const c = reyloClient(tx)
    await insertItem.run(c, first)
    await insertItem.run(c, second)
    return listItems.run(c)
  })
  expect(inside.length === 2, 'the transaction sees its own writes')

  // Read the committed rows: every value comes back as its type says.
  const rows = await read((tx) => listItems.run(reyloClient(tx)))
  expect(Array.isArray(rows), 'a buffered result is an array, not a stream')
  expect(
    rows.map((r) => r.id).join() === '2,1',
    `row order follows ORDER BY: ${rows.map((r) => r.id)}`,
  )
  const [g, a] = rows

  // Reylo reads a DATETIME back as ISO 8601 wall clock text, and a BOOLEAN
  // column is a TINYINT.
  checkRow(a, { ...first, flag: 1, created: '2026-10-23T12:34:56' })
  checkRow(g, { ...second, flag: 0, created: '1999-12-31T23:59:59' })

  // A thrown error rolls the transaction back.
  await rejects(
    write(async (tx) => {
      await insertItem.run(reyloClient(tx), { ...first, id: 3n })
      throw new Error('roll back')
    }),
    /roll back/,
  )
  const rolledBack = await read((tx) =>
    getItem.run(reyloClient(tx), { id: 3n }),
  )
  expect(rolledBack === null, 'the rolled back row is absent')

  // The database handle resolves an unqualified table in its default database.
  const standalone = await getItem.run(reyloClient(db), { id: 1n })
  expect(standalone?.name === first.name, 'the handle sees the committed row')

  // An engine error rejects the query with the engine's message.
  await rejects(
    write((tx) => insertItem.run(reyloClient(tx), first)),
    /duplicate/i,
  )

  // A write reports its affected rows through the adapter.
  const affected = await write(async (tx) => {
    const res = await reyloClient(tx).query(renameItems.sql, ['renamed', 1n])
    return 'rowsAffected' in res ? res.rowsAffected : undefined
  })
  expect(affected === 2n, `affected rows: ${affected}`)

  // A value that cannot be sent exactly fails before it reaches the engine.
  await rejects(
    write((tx) =>
      insertItem.run(reyloClient(tx), { ...first, id: 4n, big: 2 ** 60 }),
    ),
    /without loss/,
  )

  // An abort ends the transaction and discards its writes.
  const abort = new AbortController()
  await rejects(
    write(async (tx) => {
      const c = reyloClient(tx, abort.signal)
      await insertItem.run(c, { ...first, id: 5n })
      abort.abort()
      await getItem.run(c, { id: 5n })
    }, abort.signal),
    /abort/i,
  )
  const aborted = await read((tx) => getItem.run(reyloClient(tx), { id: 5n }))
  expect(aborted === null, 'the aborted row is absent')

  // Report success and close the connection.
  console.log(JSON.stringify({ ok: typesAgree.every(Boolean) }))
  conn.close()
}

// checkRow checks each value of a row against the value written, by its type.
function checkRow(got: Row, want: Record<string, unknown>) {
  for (const [key, value] of Object.entries(want)) {
    const actual = (got as Record<string, unknown>)[key]
    const same =
      value instanceof Uint8Array
        ? actual instanceof Uint8Array && value.join() === actual.join()
        : typeof value === 'object' && value !== null
          ? canonical(actual) === canonical(value)
          : actual === value
    expect(same, `${key}: got ${canonical(actual)}, want ${canonical(value)}`)
  }
}

// canonical writes a value as JSON with sorted object keys, since MySQL
// stores a JSON object's keys in its own order, and a bigint as its digits.
function canonical(v: unknown): string {
  return JSON.stringify(v, (_, x: unknown) => {
    if (typeof x === 'bigint') return x.toString()
    if (x === null || typeof x !== 'object' || Array.isArray(x)) return x
    return Object.fromEntries(
      Object.entries(x).sort(([a], [b]) => a.localeCompare(b)),
    )
  })
}

// resourceRef wraps a client for the SDK handle, standing in for the Resource
// client a plugin gets from its host.
function resourceRef(client: ClientResourceRef['client']): ClientResourceRef {
  const ref: ClientResourceRef = {
    resourceId: 1,
    released: false,
    releaseReason: null,
    onReleased: () => () => {},
    client,
    createRef: () => ref,
    createResource: () => {
      throw new Error('not a host resource')
    },
    release: () => {},
    [Symbol.dispose]: () => {},
  }
  return ref
}

// expect throws when a check fails.
function expect(ok: boolean, what: string) {
  if (!ok) throw new Error(`check failed: ${what}`)
}

// rejects waits for a promise to reject with an error matching pattern.
async function rejects(p: Promise<unknown>, pattern: RegExp) {
  try {
    await p
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    expect(pattern.test(msg), `error ${JSON.stringify(msg)} matches ${pattern}`)
    return
  }
  throw new Error(`check failed: expected a rejection matching ${pattern}`)
}

await main()
