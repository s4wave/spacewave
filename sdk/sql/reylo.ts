import type { ColumnSchema, SqlValue } from '../../db/sql/sql.pb.js'
import type { SqlQueryResult } from './sql.js'
import type { SqlExecResponse } from '../../db/sql/rpc/sql.pb.js'

// SqlStatementRunner runs statements on a sql/db: an open SqlTransaction, or a
// SqlDatabase, which runs each statement in its own transaction using the default
// database selected by the DSN passed when the handle was opened.
export interface SqlStatementRunner {
  exec(
    query: string,
    args?: SqlValue[],
    abortSignal?: AbortSignal,
  ): Promise<SqlExecResponse>
  query(
    query: string,
    args?: SqlValue[],
    abortSignal?: AbortSignal,
  ): Promise<SqlQueryResult>
}

// ReyloClient is a sql/db in the shape of a mariadb connection, one of the
// clients a Reylo query's run() takes. query() resolves to the rows as
// objects keyed by column name, or to the exec result for a write.
export interface ReyloClient {
  query(
    statement: string | { sql: string },
    values?: unknown[],
  ): Promise<Record<string, unknown>[] | SqlExecResponse>
  batch(statement: string | { sql: string }, values?: unknown[]): Promise<never>
}

// reyloClient adapts a SqlDatabase or SqlTransaction for Reylo. Queries run
// inside a transaction stay on it, so every query passed the same transaction
// commits or discards together.
export function reyloClient(
  runner: SqlStatementRunner,
  abortSignal?: AbortSignal,
): ReyloClient {
  return {
    async query(statement, values = []) {
      // Bind the parameters and run a read as a query, anything else as exec.
      const text = typeof statement === 'string' ? statement : statement.sql
      const args = values.map(toSqlValue)
      if (!isReadStatement(text)) {
        return runner.exec(text, args, abortSignal)
      }

      // Decode each row by its column types.
      const result = await runner.query(text, args, abortSignal)
      const decoders = result.columns.map(columnDecoder)
      return result.rows.map((row) => {
        const out: Record<string, unknown> = {}
        result.columns.forEach((col, i) => {
          out[col.name ?? ''] = decoders[i](row.values?.[i])
        })
        return out
      })
    },
    batch() {
      return Promise.reject(new Error('sql/db: batch is not supported'))
    },
  }
}

// readKeywords are the leading keywords of statements that return rows.
const readKeywords = new Set([
  'select',
  'with',
  'show',
  'explain',
  'describe',
  'desc',
  'table',
  'values',
])

// isReadStatement reports whether a statement returns rows, by its first
// keyword after any comments and opening parentheses.
function isReadStatement(text: string): boolean {
  const stripped = text.replace(
    /^(?:\s+|--[^\n]*\n?|#[^\n]*\n?|\/\*[\s\S]*?\*\/|\()*/,
    '',
  )
  const keyword = /^[a-z]+/i.exec(stripped)?.[0].toLowerCase() ?? ''
  return readKeywords.has(keyword)
}

// toSqlValue converts a parameter Reylo binds to the wire value. It rejects a
// value it cannot send without loss: an unsafe or non-finite number, a bigint
// outside int64, or a Date, whose wall clock text depends on the time zone.
function toSqlValue(v: unknown): SqlValue {
  // Encode supported parameter types without losing their values.
  if (v === null || v === undefined) return {}
  if (typeof v === 'bigint') {
    if (BigInt.asIntN(64, v) !== v) {
      throw new RangeError(`sql/db: ${v} does not fit in int64`)
    }
    return { value: { case: 'intValue', value: v } }
  }
  if (typeof v === 'number') {
    if (Number.isSafeInteger(v)) {
      return { value: { case: 'intValue', value: BigInt(v) } }
    }
    if (!Number.isFinite(v) || Number.isInteger(v)) {
      throw new RangeError(`sql/db: ${v} cannot be sent without loss`)
    }
    return { value: { case: 'floatValue', value: v } }
  }
  if (v instanceof Date) {
    throw new TypeError('sql/db: pass a date as text, not a Date')
  }
  if (typeof v === 'boolean') {
    return { value: { case: 'intValue', value: v ? 1n : 0n } }
  }
  if (typeof v === 'string') return { value: { case: 'strValue', value: v } }
  if (v instanceof Uint8Array) {
    return { value: { case: 'blobValue', value: v } }
  }
  return { value: { case: 'strValue', value: JSON.stringify(v) } }
}

// Decoder turns a wire value into the value a mariadb connection returns
// with Reylo's options: a BIGINT as a bigint, other integers as numbers, a
// DECIMAL as its text, a date or time as its text, parsed JSON, and binary
// types as bytes.
type Decoder = (v: SqlValue | undefined) => unknown

// columnDecoder returns the decoder for a column's MySQL type name.
function columnDecoder(col: ColumnSchema): Decoder {
  switch (col.databaseTypeName) {
    case 'BIGINT':
    case 'UNSIGNED BIGINT':
      return (v) => scalar(v)
    case 'DATE':
      return (v) => dateText(scalar(v), -1)
    case 'DATETIME':
    case 'TIMESTAMP':
      return (v) => dateText(scalar(v), col.scale ?? 0)
    case 'JSON':
      return (v) => {
        const s = textOf(scalar(v))
        return s === null ? null : JSON.parse(s)
      }
    case 'BINARY':
    case 'VARBINARY':
    case 'BLOB':
    case 'GEOMETRY':
    case 'BIT':
      return (v) => {
        const s = scalar(v)
        return typeof s === 'string' ? new TextEncoder().encode(s) : s
      }
    default:
      return (v) => {
        const s = scalar(v)
        if (typeof s === 'bigint') return Number(s)
        if (s instanceof Uint8Array) return new TextDecoder().decode(s)
        return s
      }
  }
}

// scalar returns the wire value as it is, null for NULL.
function scalar(v: SqlValue | undefined): unknown {
  return v?.value?.value ?? null
}

// textOf returns a text or bytes value as text.
function textOf(s: unknown): string | null {
  if (s === null) return null
  if (s instanceof Uint8Array) return new TextDecoder().decode(s)
  return String(s)
}

// dateText turns the engine's RFC 3339 time into MySQL's wall clock text: the
// date alone for a DATE (scale -1), and otherwise the fraction padded to the
// column's scale. RFC 3339 trims trailing zeros from the fraction.
function dateText(s: unknown, scale: number): unknown {
  // Keep the wall clock text and normalize its fractional precision.
  if (typeof s !== 'string') return s
  const wall = s.replace(/(?:Z|[+-]00:00)$/, '')
  if (scale < 0) return wall.slice(0, 10)
  if (scale === 0) return wall
  const [whole, fraction = ''] = wall.split('.')
  return `${whole}.${fraction.padEnd(scale, '0')}`
}
