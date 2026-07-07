import type {
  ColumnSchema,
  Row,
  RowBatch,
  SqlValue,
} from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

// SqlParamKind names the SqlValue oneof branch a parameter edits.
export type SqlParamKind = 'text' | 'int' | 'float' | 'blob' | 'null'

// SqlGridData is the normalized shape consumed by SqlResultGrid: column headers
// plus flattened rows, decoupled from the per-RPC response envelope.
export interface SqlGridData {
  columns: ColumnSchema[]
  rows: Row[]
  truncated: boolean
}

// flattenRowBatches concatenates the rows from a repeated RowBatch field into a
// single ordered row list. The wire format groups rows into batches; viewers
// render one flat grid.
export function flattenRowBatches(rowBatches: RowBatch[] | undefined): Row[] {
  if (!rowBatches) return []
  const rows: Row[] = []
  for (const batch of rowBatches) {
    if (batch.rows) rows.push(...batch.rows)
  }
  return rows
}

// SQL_NULL is the display token for an unset SqlValue oneof (NULL cell).
export const SQL_NULL = 'NULL'

// formatCell formats one SqlValue oneof cell for display. An unset oneof is a
// SQL NULL and renders as the SQL_NULL token; callers style NULL distinctly.
export function formatCell(value: SqlValue | undefined): string {
  const cell = value?.value
  if (!cell || cell.case === undefined) return SQL_NULL
  switch (cell.case) {
    case 'intValue':
      return cell.value.toString()
    case 'floatValue':
      return cell.value.toString()
    case 'strValue':
      return cell.value
    case 'blobValue':
      return blobToHex(cell.value)
    default:
      return SQL_NULL
  }
}

// isNullCell reports whether a SqlValue is an unset oneof (SQL NULL).
export function isNullCell(value: SqlValue | undefined): boolean {
  return value?.value?.case === undefined
}

// blobToHex renders a byte cell as a lowercase 0x-prefixed hex string.
function blobToHex(bytes: Uint8Array): string {
  let hex = '0x'
  for (const byte of bytes) {
    hex += byte.toString(16).padStart(2, '0')
  }
  return hex
}

function blobFromHex(text: string): Uint8Array {
  const hex = text.trim().replace(/^0x/i, '')
  if (hex.length % 2 !== 0 || /[^0-9a-f]/i.test(hex)) {
    throw new Error(`invalid blob parameter: ${text}`)
  }
  const bytes = new Uint8Array(hex.length / 2)
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16)
  }
  return bytes
}

// columnName resolves a column header label, falling back to a positional name
// when the driver did not report one.
export function columnName(
  column: ColumnSchema | undefined,
  index: number,
): string {
  return column?.name || `column_${index + 1}`
}

// toCsv renders grid data as RFC 4180 CSV text. NULL cells render as empty
// fields so a re-import can distinguish them from an explicit empty string only
// by position; this is the documented limitation of CSV export.
export function toCsv(data: SqlGridData): string {
  const header = data.columns.map((column, index) =>
    csvField(columnName(column, index)),
  )
  const lines = [header.join(',')]
  for (const row of data.rows) {
    const cells = data.columns.map((_column, index) => {
      const value = row.values?.[index]
      return isNullCell(value) ? '' : csvField(formatCell(value))
    })
    lines.push(cells.join(','))
  }
  return lines.join('\r\n')
}

// csvField quotes a CSV field when it contains a comma, quote, or newline,
// escaping embedded quotes by doubling them.
function csvField(value: string): string {
  if (/[",\r\n]/.test(value)) {
    return `"${value.replaceAll('"', '""')}"`
  }
  return value
}

// paramKind reports the SqlParamKind of a stored SqlValue parameter.
export function paramKind(value: SqlValue | undefined): SqlParamKind {
  switch (value?.value?.case) {
    case 'intValue':
      return 'int'
    case 'floatValue':
      return 'float'
    case 'strValue':
      return 'text'
    case 'blobValue':
      return 'blob'
    default:
      return 'null'
  }
}

// paramText renders the editable text of a stored SqlValue parameter; NULL has
// no text.
export function paramText(value: SqlValue | undefined): string {
  const cell = value?.value
  if (!cell || cell.case === undefined) return ''
  switch (cell.case) {
    case 'intValue':
    case 'floatValue':
      return cell.value.toString()
    case 'strValue':
      return cell.value
    case 'blobValue':
      return blobToHex(cell.value)
    default:
      return ''
  }
}

// buildParam constructs a SqlValue from an editable kind and text, throwing on a
// malformed numeric or blob literal so the editor can surface the parse error.
export function buildParam(kind: SqlParamKind, text: string): SqlValue {
  switch (kind) {
    case 'null':
      return {}
    case 'text':
      return { value: { case: 'strValue', value: text } }
    case 'int': {
      const trimmed = text.trim()
      if (!/^[+-]?\d+$/.test(trimmed)) {
        throw new Error(`invalid integer parameter: ${text}`)
      }
      return { value: { case: 'intValue', value: BigInt(trimmed) } }
    }
    case 'float': {
      const parsed = Number(text.trim())
      if (text.trim() === '' || Number.isNaN(parsed)) {
        throw new Error(`invalid float parameter: ${text}`)
      }
      return { value: { case: 'floatValue', value: parsed } }
    }
    case 'blob':
      return { value: { case: 'blobValue', value: blobFromHex(text) } }
  }
}
