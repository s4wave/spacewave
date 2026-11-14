import { type ReactNode, useCallback, useMemo } from 'react'
import { LuChevronDown, LuChevronUp } from 'react-icons/lu'

import {
  coarsePointerQuery,
  useMediaQuery,
} from '@s4wave/web/hooks/useMediaQuery.js'
import { cn } from '@s4wave/web/style/utils.js'
import { FactList, FactRow } from '@s4wave/web/ui/FactList.js'
import { MiddleEllipsis } from '@s4wave/web/ui/MiddleEllipsis.js'
import {
  List,
  type ListItem,
  type ListSortFn,
  type ListStateHandle,
  ListRow,
  type RenderHeaderProps,
  type RowComponentProps,
} from '@s4wave/web/ui/list'

// TableStep is a width of the table's own container, in rem, from which a
// column shows: DESIGN.md's table steps. 0 is every width.
export type TableStep = 0 | 16 | 22 | 28 | 40

// tableSteps are the steps a column can show from, narrowest first.
export const tableSteps: readonly TableStep[] = [16, 22, 28, 40]

// The class that shows a column's header and cells from its step, and the one
// that hides what stands in for it there. Literal, so Tailwind emits them.
const showFrom: Record<TableStep, string> = {
  0: 'flex',
  16: 'hidden @table-16:flex',
  22: 'hidden @table-22:flex',
  28: 'hidden @table-28:flex',
  40: 'hidden @table-40:flex',
}
const hideFrom: Record<TableStep, string> = {
  0: 'hidden',
  16: '@table-16:hidden',
  22: '@table-22:hidden',
  28: '@table-28:hidden',
  40: '@table-40:hidden',
}

// TableColumn is one column of a Table over records of type T.
export interface TableColumn<T> {
  // key names the column in the sort state; unique in the table.
  key: string
  // label heads the column and names its value among the shed facts.
  label: string
  // text is the record's value as text: the cell, its title, the fact the
  // column leaves when it sheds, and the sort unless compare is given.
  text: (row: T) => string
  // cell draws the value when plain text is not enough, such as a glyph.
  cell?: (row: T) => ReactNode
  // kind says how the value gives way: an id (a hash, key, path or URL)
  // shortens in its middle; text ends in an ellipsis. Defaults to text.
  kind?: 'id' | 'text'
  // minWidth is the column's width in rem. It is not used for the first
  // column, which takes the rest of the row and keeps viewer-name.
  minWidth?: number
  // priority orders the columns by when they appear as the table widens,
  // lowest first; defaults to the column's place. Not used for the first.
  priority?: number
  // end aligns the value to the row's end, as for a number.
  end?: boolean
  // compare orders two records for a sort by this column.
  compare?: (a: T, b: T) => number
}

// Widths in rem the step of each column is reckoned from: the row's inline
// padding, the list's, and the classic scrollbar's 6px, the identifying
// column's floor (viewer-name), and the gap before each further column.
const rowPadRem = 1.5 + 0.25 + 0.375
const nameRem = 7.5
const gapRem = 0.75

// columnSteps gives each column the step it shows from: the first column
// always; then, in priority order, the first step that holds the column with
// every column before it. A column no step holds gets null and shows only as
// a fact of the selected row.
export function columnSteps<T>(
  columns: readonly TableColumn<T>[],
): (TableStep | null)[] {
  // Walk the further columns by priority, adding each one's width.
  const order = columns
    .map((column, index) => ({ column, index }))
    .slice(1)
    .sort(
      (a, b) =>
        (a.column.priority ?? a.index) - (b.column.priority ?? b.index) ||
        a.index - b.index,
    )
  const steps: (TableStep | null)[] = columns.map(() => 0)
  let need = rowPadRem + nameRem
  for (const { column, index } of order) {
    need += gapRem + (column.minWidth ?? 6)
    steps[index] = tableSteps.find((step) => step >= need) ?? null
  }
  return steps
}

// sortedColumn is the index of the column a sort key names. A key no column
// has, such as one a stored state kept after its column left, sorts by the
// first column, so the header and the order agree.
export function sortedColumn<T>(
  columns: readonly TableColumn<T>[],
  sortKey: string | undefined,
): number {
  return Math.max(
    columns.findIndex((c) => c.key === sortKey),
    0,
  )
}

// compareText orders two values as a person reads them: numbers by value.
function compareText(a: string, b: string): number {
  return a.localeCompare(b, undefined, { numeric: true })
}

// compareBy orders two rows by a column: its compare, or else its text. A row
// with no record ties.
function compareBy<T>(column: TableColumn<T>) {
  return (a: ListItem<T>, b: ListItem<T>) => {
    if (a.data === undefined || b.data === undefined) return 0
    if (column.compare) return column.compare(a.data, b.data)
    return compareText(column.text(a.data), column.text(b.data))
  }
}

// TableProps are the props for Table.
export interface TableProps<T> {
  // rows are the records, each with an id that is stable across changes.
  rows: ListItem<T>[]
  // columns are the table's columns in their order; the first identifies a
  // row and never sheds. The array must keep its identity across renders, as
  // a module constant or a memo: a new array remounts every row and drops
  // focus.
  columns: readonly TableColumn<T>[]
  // state is the selection, focus and sort, from useListState.
  state: ListStateHandle
  // onOpen gets the rows a double click, Enter or Ctrl+O opens.
  onOpen?: (rows: ListItem<T>[]) => void
  // placeholder shows when there are no rows.
  placeholder?: ReactNode
  // label is the table's accessible name, the List's by default.
  label?: string
  className?: string
}

// Table draws records as List rows under a header of sortable columns. It is
// the inline-size container table and sheds by its own width, never by
// clipping a cell: each further column shows from the step columnSteps gives
// it. A shed column's value shows as a fact of the one selected row, under
// the rows, until the column returns. A sort by a shed column reads "by" and
// its label beside the first column's header. The rows keep the List's keys,
// selection and look. Its states are in Table.tla.
export function Table<T>({
  rows,
  columns,
  state,
  onOpen,
  placeholder,
  label,
  className,
}: TableProps<T>) {
  // Each column's step, and the widest, from which no fact is left.
  const steps = useMemo(() => columnSteps(columns), [columns])
  const widest = steps.includes(null)
    ? null
    : (Math.max(...steps.map((step) => step ?? 0)) as TableStep)

  // A coarse pointer needs taller rows.
  const coarsePointer = useMediaQuery(coarsePointerQuery)
  const rowHeight = coarsePointer ? 44 : 24

  // Sort by the column's compare, or by its text.
  const sortFn = useCallback<ListSortFn<T>>(
    (items, sortKey, direction) => {
      const column = columns[sortedColumn(columns, sortKey)]
      const sorted = [...items].sort(compareBy(column))
      return direction === 'desc' ? sorted.reverse() : sorted
    },
    [columns],
  )

  // Draw the header from the columns and the sort.
  const renderHeader = useCallback(
    (props: RenderHeaderProps) => (
      <TableHeader columns={columns} steps={steps} {...props} />
    ),
    [columns, steps],
  )

  // Draw each row's cells.
  const rowComponent = useMemo(
    () =>
      function TableRow(props: RowComponentProps<T>) {
        return <TableRowCells columns={columns} steps={steps} {...props} />
      },
    [columns, steps],
  )

  // The selected row whose shed values show as facts.
  const [listState] = state
  const selectedId =
    listState.selectedIds?.length === 1 ? listState.selectedIds[0] : undefined
  const selected = rows.find((row) => row.id === selectedId)?.data

  return (
    <div className={cn('@container/table flex min-h-0 flex-col', className)}>
      <List
        items={rows}
        rowComponent={rowComponent}
        rowHeight={rowHeight}
        onRowDefaultAction={onOpen}
        state={state}
        renderHeader={renderHeader}
        sortFn={sortFn}
        placeholder={placeholder}
        label={label}
      />
      {selected !== undefined && widest !== 0 && (
        <ShedFacts
          columns={columns}
          steps={steps}
          widest={widest}
          row={selected}
        />
      )}
    </div>
  )
}

// TableHeader is the row of column headers; each sorts by its column.
function TableHeader<T>({
  columns,
  steps,
  state,
  dispatch,
}: RenderHeaderProps & {
  columns: readonly TableColumn<T>[]
  steps: (TableStep | null)[]
}) {
  // Read the sort the header shows, defaulting to the first column.
  const [first, ...rest] = columns
  const Chevron = state.sortDirection === 'desc' ? LuChevronUp : LuChevronDown
  const sortedIndex = sortedColumn(columns, state.sortKey)
  const sortedStep = sortedIndex > 0 ? steps[sortedIndex] : 0

  // Sort by a column; the List flips direction when it is already sorted.
  const sortBy = (key: string) => dispatch({ type: 'SET_SORT', sortKey: key })

  return (
    <div className="bg-panel-header text-foreground-alt border-foreground/8 flex items-center gap-3 border-b px-3.5 text-xs select-none">
      <button
        type="button"
        className={cn(
          'min-w-viewer-name flex flex-1 cursor-pointer items-center gap-1 py-1.5 text-left [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-0',
          sortedIndex === 0 && 'text-foreground',
        )}
        onClick={() => sortBy(first.key)}
      >
        <span>{first.label}</span>
        {sortedIndex === 0 && <Chevron aria-hidden className="size-3" />}
        {sortedIndex > 0 && (
          <span
            className={cn(
              'flex items-center gap-1',
              sortedStep !== null && hideFrom[sortedStep],
            )}
          >
            by {columns[sortedIndex].label.toLowerCase()}
            <Chevron aria-hidden className="size-3" />
          </span>
        )}
      </button>
      {rest.map((column, i) => {
        const step = steps[i + 1]
        if (step === null) return null
        const sorted = sortedIndex === i + 1
        return (
          <button
            key={column.key}
            type="button"
            style={{ '--table-column': `${column.minWidth ?? 6}rem` }}
            className={cn(
              'w-(--table-column) shrink-0 cursor-pointer items-center gap-1 py-1.5 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-0',
              showFrom[step],
              column.end && 'flex-row-reverse',
              sorted && 'text-foreground',
            )}
            onClick={() => sortBy(column.key)}
          >
            <span className="truncate">{column.label}</span>
            {sorted && <Chevron aria-hidden className="size-3 shrink-0" />}
          </button>
        )
      })}
    </div>
  )
}

// TableRowCells is one record's row: the List row with a cell per column
// the table's width shows.
function TableRowCells<T>({
  columns,
  steps,
  ...props
}: RowComponentProps<T> & {
  columns: readonly TableColumn<T>[]
  steps: (TableStep | null)[]
}) {
  // A row with no record draws nothing.
  const row = props.item.data
  if (row === undefined) return null
  const [first, ...rest] = columns

  return (
    <ListRow {...props} className="gap-3">
      <div
        role="cell"
        className="min-w-viewer-name flex flex-1 items-center gap-2 overflow-hidden"
      >
        <CellValue column={first} row={row} />
      </div>
      {rest.map((column, i) => {
        const step = steps[i + 1]
        if (step === null) return null
        return (
          <div
            key={column.key}
            role="cell"
            style={{ '--table-column': `${column.minWidth ?? 6}rem` }}
            className={cn(
              'text-foreground-alt/70 w-(--table-column) shrink-0 items-center overflow-hidden',
              showFrom[step],
              column.end && 'justify-end tabular-nums',
            )}
          >
            <CellValue column={column} row={row} />
          </div>
        )
      })}
    </ListRow>
  )
}

// CellValue draws one value: the column's own cell, an id shortened in its
// middle, or text that ends in an ellipsis.
function CellValue<T>({ column, row }: { column: TableColumn<T>; row: T }) {
  if (column.cell) return column.cell(row)
  const text = column.text(row)
  if (column.kind === 'id') {
    return <MiddleEllipsis value={text} className="min-w-0 font-mono" />
  }
  return (
    <span className="truncate" title={text}>
      {text}
    </span>
  )
}

// ShedFacts lists the selected row's values whose columns the width sheds,
// each leaving as its column returns, and all of them from the widest step.
function ShedFacts<T>({
  columns,
  steps,
  widest,
  row,
}: {
  columns: readonly TableColumn<T>[]
  steps: (TableStep | null)[]
  widest: TableStep | null
  row: T
}) {
  return (
    <div
      className={cn(
        'border-foreground/8 shrink-0 border-t px-1 py-0.5',
        widest !== null && hideFrom[widest],
      )}
    >
      <FactList dense>
        {columns.slice(1).map((column, i) => {
          const step = steps[i + 1]
          return (
            <div
              key={column.key}
              className={cn(step !== null && hideFrom[step])}
            >
              <FactRow
                label={column.label}
                value={column.text(row)}
                kind={column.kind ?? 'text'}
              />
            </div>
          )
        })}
      </FactList>
    </div>
  )
}
