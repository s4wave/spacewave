import { useCallback, useMemo, useState } from 'react'
import { LuDownload } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="flex h-full min-h-0 flex-col">
      <div className="border-foreground/8 flex shrink-0 items-center gap-2 border-b px-3 py-1.5">
        <span className="text-foreground-alt/60 text-xs tabular-nums">
          {data.rows.length === 0
            ? '0 rows'
            : `${firstRow}–${lastRow} of ${data.rows.length}`}
          {data.truncated ? ' (truncated)' : ''}
        </span>
        <div className="flex-1" />
        {pageCount > 1 ? (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={handlePrev}
              disabled={safePage === 0}
              className="h-6 px-2 text-xs"
            >
              Prev
            </Button>
            <span className="text-foreground-alt/60 text-xs tabular-nums">
              {safePage + 1}/{pageCount}
            </span>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleNext}
              disabled={safePage >= pageCount - 1}
              className="h-6 px-2 text-xs"
            >
              Next
            </Button>
          </div>
        ) : null}
        {csvFileName ? (
          <Button
            variant="outline"
            size="sm"
            onClick={handleExport}
            disabled={data.rows.length === 0}
            className="h-6 gap-1 px-2 text-xs"
          >
            <LuDownload className="size-3" />
            CSV
          </Button>
        ) : null}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full border-collapse text-left text-xs">
          <thead className="bg-background-primary sticky top-0 z-10">
            <tr className="border-foreground/8 border-b">
              <th className="text-foreground-alt/40 w-10 px-2 py-1.5 text-right font-medium tabular-nums">
                #
              </th>
              {data.columns.map((column, index) => (
                <th
                  // Result columns are a fixed schema and may share names (a
                  // join projecting the same column twice), so the positional
                  // index is the stable column identity.
                  // eslint-disable-next-line react-doctor/no-array-index-as-key
                  key={`${columnName(column, index)}-${index}`}
                  className="text-foreground px-3 py-1.5 font-medium whitespace-nowrap"
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
                className="border-foreground/5 hover:bg-foreground/5 border-b"
              >
                <td className="text-foreground-alt/40 px-2 py-1 text-right tabular-nums">
                  {safePage * PAGE_SIZE + rowIndex + 1}
                </td>
                {data.columns.map((_column, colIndex) => {
                  const value = row.values?.[colIndex]
                  const isNull = isNullCell(value)
                  return (
                    <td
                      key={colIndex}
                      className={cn(
                        'text-foreground max-w-md truncate px-3 py-1 font-mono',
                        isNull && 'text-foreground-alt/40 italic',
                      )}
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
