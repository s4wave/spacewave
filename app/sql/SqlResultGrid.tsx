import { useCallback, useMemo, useState } from 'react'
import { LuDownload } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'

import {
  columnName,
  isNullCell,
  formatCell,
  SQL_NULL,
  toCsv,
  type SqlGridData,
} from './sql-cell.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    minHeight: 0,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: { flex: 1 },
  rule5: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
  },
  rule6: { flex: 1, overflow: 'auto', minHeight: 0 },
  rule7: {
    borderCollapse: 'collapse',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    width: '100%',
  },
  rule8: {
    backgroundColor: themeColors['--color-background-primary'],
    position: 'sticky',
    zIndex: 10,
    top: 0,
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-numeric-spacing': 'tabular-nums',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    fontWeight: themeFonts['--font-weight-medium'],
    textAlign: 'right',
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
    whiteSpace: 'nowrap',
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule13: {
    '--tw-numeric-spacing': 'tabular-nums',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textAlign: 'right',
  },
  rule14: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    maxWidth: 'var(--container-md)',
  },
  rule15: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontStyle: 'italic',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const PAGE_SIZE = 100

interface SqlResultGridProps {
  data: SqlGridData
  // emptyTitle labels the empty grid state.
  emptyTitle?: string
  // emptyDescription describes the empty grid state.
  emptyDescription?: string
  // csvFileName names the downloaded CSV; omit to hide the export action.
  csvFileName?: string
}

// SqlResultGrid renders a typed SQL result as a paged table, styling NULL cells
// distinctly and offering a client-side CSV export of the visible result. Rows
// are buffered by the caller; paging is local navigation, not server fetch.
export function SqlResultGrid({
  data,
  emptyTitle = 'No rows',
  emptyDescription = 'The query returned no rows.',
  csvFileName,
}: SqlResultGridProps) {
  const [page, setPage] = useState(0)

  const pageCount = Math.max(1, Math.ceil(data.rows.length / PAGE_SIZE))
  const safePage = Math.min(page, pageCount - 1)
  const pageRows = useMemo(() => {
    const start = safePage * PAGE_SIZE
    return data.rows.slice(start, start + PAGE_SIZE)
  }, [data.rows, safePage])

  const handlePrev = useCallback(() => setPage((p) => Math.max(0, p - 1)), [])
  const handleNext = useCallback(
    () => setPage((p) => Math.min(pageCount - 1, p + 1)),
    [pageCount],
  )

  const handleExport = useCallback(() => {
    if (!csvFileName) return
    const blob = new Blob([toCsv(data)], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = csvFileName
    link.click()
    URL.revokeObjectURL(url)
  }, [csvFileName, data])

  if (data.columns.length === 0 && data.rows.length === 0) {
    return (
      <EmptyState
        variant="compact"
        title={emptyTitle}
        description={emptyDescription}
      />
    )
  }

  const firstRow = data.rows.length === 0 ? 0 : safePage * PAGE_SIZE + 1
  const lastRow = Math.min(data.rows.length, (safePage + 1) * PAGE_SIZE)

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>
          {data.rows.length === 0
            ? '0 rows'
            : `${firstRow}–${lastRow} of ${data.rows.length}`}
          {data.truncated ? ' (truncated)' : ''}
        </span>
        <div sx={stylexStyles2.rule4} />
        {pageCount > 1 ? (
          <div sx={stylexStyles2.rule5}>
            <Button
              variant="ghost"
              onClick={handlePrev}
              disabled={safePage === 0}
              size="toolbarSm"
            >
              Prev
            </Button>
            <span sx={stylexStyles2.rule3}>
              {safePage + 1}/{pageCount}
            </span>
            <Button
              variant="ghost"
              onClick={handleNext}
              disabled={safePage >= pageCount - 1}
              size="toolbarSm"
            >
              Next
            </Button>
          </div>
        ) : null}
        {csvFileName ? (
          <Button
            variant="outline"
            onClick={handleExport}
            disabled={data.rows.length === 0}
            size="xs"
          >
            <LuDownload sx={stylexStyles.rule1} />
            CSV
          </Button>
        ) : null}
      </div>
      <div sx={stylexStyles2.rule6}>
        <table sx={stylexStyles2.rule7}>
          <thead sx={stylexStyles2.rule8}>
            <tr sx={stylexStyles2.rule9}>
              <th aria-label="Row number" sx={stylexStyles2.rule10}>
                #
              </th>
              {data.columns.map((column, index) => (
                <th
                  // Result columns are a fixed schema and may share names (a
                  // join projecting the same column twice), so the positional
                  // index is the stable column identity.
                  // eslint-disable-next-line react-doctor/no-array-index-as-key
                  key={`${columnName(column, index)}-${index}`}
                  sx={stylexStyles2.rule11}
                  title={column.databaseTypeName || undefined}
                >
                  {columnName(column, index)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageRows.map((row, rowIndex) => (
              <tr
                key={safePage * PAGE_SIZE + rowIndex}
                sx={stylexStyles2.rule12}
              >
                <td sx={stylexStyles2.rule13}>
                  {safePage * PAGE_SIZE + rowIndex + 1}
                </td>
                {data.columns.map((_column, colIndex) => {
                  const value = row.values?.[colIndex]
                  const isNull = isNullCell(value)
                  return (
                    <td
                      key={colIndex}
                      sx={[
                        stylexStyles2.rule14,
                        !!isNull && stylexStyles2.rule15,
                      ]}
                      title={formatCell(value)}
                    >
                      {isNull ? SQL_NULL : formatCell(value)}
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
