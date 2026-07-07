import { describe, expect, it } from 'vitest'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

import {
  buildParam,
  flattenRowBatches,
  isNullCell,
  paramKind,
  paramText,
  formatCell,
  SQL_NULL,
  toCsv,
} from './sql-cell.js'

const intVal = (n: bigint): SqlValue => ({
  value: { case: 'intValue', value: n },
})
const strVal = (s: string): SqlValue => ({
  value: { case: 'strValue', value: s },
})
const blobVal = (bytes: number[]): SqlValue => ({
  value: { case: 'blobValue', value: new Uint8Array(bytes) },
})
const nullVal: SqlValue = {}

describe('formatCell', () => {
  it('renders each oneof branch', () => {
    expect(formatCell(intVal(42n))).toBe('42')
    expect(formatCell({ value: { case: 'floatValue', value: 1.5 } })).toBe(
      '1.5',
    )
    expect(formatCell(strVal('hello'))).toBe('hello')
    expect(formatCell(blobVal([0, 255]))).toBe('0x00ff')
  })

  it('renders an unset oneof as NULL', () => {
    expect(formatCell(nullVal)).toBe(SQL_NULL)
    expect(formatCell(undefined)).toBe(SQL_NULL)
    expect(isNullCell(nullVal)).toBe(true)
    expect(isNullCell(intVal(0n))).toBe(false)
  })
})

describe('flattenRowBatches', () => {
  it('concatenates rows across batches in order', () => {
    const rows = flattenRowBatches([
      { rows: [{ values: [intVal(1n)] }, { values: [intVal(2n)] }] },
      { rows: [{ values: [intVal(3n)] }] },
    ])
    expect(rows.map((r) => formatCell(r.values?.[0]))).toEqual(['1', '2', '3'])
  })

  it('returns empty for undefined batches', () => {
    expect(flattenRowBatches(undefined)).toEqual([])
  })
})

describe('toCsv', () => {
  it('renders header, escapes special fields, and blanks NULL cells', () => {
    const csv = toCsv({
      columns: [{ name: 'a' }, { name: 'b,c' }, { name: 'n' }],
      rows: [
        { values: [strVal('x'), strVal('has "quote"'), nullVal] },
        { values: [intVal(7n), strVal('line\nbreak'), strVal('')] },
      ],
      truncated: false,
    })
    const lines = csv.split('\r\n')
    expect(lines[0]).toBe('a,"b,c",n')
    expect(lines[1]).toBe('x,"has ""quote""",')
    expect(lines[2]).toBe('7,"line\nbreak",')
  })
})

describe('parameters', () => {
  it('classifies and renders stored parameters', () => {
    expect(paramKind(intVal(3n))).toBe('int')
    expect(paramKind(strVal('s'))).toBe('text')
    expect(paramKind(blobVal([10, 11]))).toBe('blob')
    expect(paramKind(nullVal)).toBe('null')
    expect(paramText(intVal(3n))).toBe('3')
    expect(paramText(strVal('s'))).toBe('s')
    expect(paramText(blobVal([10, 11]))).toBe('0x0a0b')
    expect(paramText(nullVal)).toBe('')
  })

  it('builds typed SqlValues and rejects malformed numbers', () => {
    expect(buildParam('null', '')).toEqual({})
    expect(buildParam('text', 'hi')).toEqual(strVal('hi'))
    expect(buildParam('int', '-5')).toEqual(intVal(-5n))
    expect(buildParam('float', '2.5')).toEqual({
      value: { case: 'floatValue', value: 2.5 },
    })
    expect(buildParam('blob', '0x0a0b')).toEqual(blobVal([10, 11]))
    expect(buildParam('blob', '0A0B')).toEqual(blobVal([10, 11]))
    expect(() => buildParam('int', '1.2')).toThrow()
    expect(() => buildParam('float', 'abc')).toThrow()
    expect(() => buildParam('blob', 'abc')).toThrow()
  })
})
