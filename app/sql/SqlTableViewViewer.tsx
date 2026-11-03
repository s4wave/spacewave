import { LuListFilter, LuRefreshCw } from '@s4wave/web/images/icons.js'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import {
  SqlTableView,
  SqlTableViewTypeID,
} from '@s4wave/sdk/sql/table-view/table-view.js'
import type {
  FetchRowsResponse,
  GetTableViewResponse,
  TableView,
} from '@s4wave/sdk/sql/table-view/table-view.pb.js'

import { flattenRowBatches, type SqlGridData } from './sql-cell.js'
import { SqlResultGrid } from './SqlResultGrid.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: { flex: 1 },
  rule6: { minHeight: 0, flex: 1 },
  rule7: {
    display: 'flex',
    flexShrink: 0,
    flexWrap: 'wrap',
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 4)`,
    rowGap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
  rule9: { color: themeColors['--color-foreground'] },
  rule10: { padding: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export { SqlTableViewTypeID }

// SqlTableViewViewer is the full sql/table-view viewer: the persisted filter
// metadata over a typed row grid that reuses the shared result grid, with a
// refresh that re-runs the saved SELECT.
export function SqlTableViewViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlTableView,
    SqlTableViewTypeID,
  )

  const metaResource = useResource(
    handle,
    async (tableView, signal) => {
      if (!tableView) return null
      return tableView.getTableView(signal)
    },
    [],
  )

  const rowsResource = useResource(
    handle,
    async (tableView, signal) => {
      if (!tableView) return null
      return tableView.fetchRows(signal)
    },
    [],
  )

  const view = metaResource.value?.tableView
  useObjectName(view?.displayName)
  const rows = rowsResource.value
  const data = rows
    ? {
        columns: rows.columns ?? [],
        rows: flattenRowBatches(rows.rowBatches),
        truncated: rows.truncated ?? false,
      }
    : null

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <LuListFilter sx={stylexStyles.rule1} aria-hidden />
        <span sx={stylexStyles2.rule3}>Table View</span>
        {view?.targetTableName ? (
          <span sx={stylexStyles2.rule4}>{view.targetTableName}</span>
        ) : null}
        <div sx={stylexStyles2.rule5} />
        <Button
          variant="ghost"
          onClick={rowsResource.retry}
          disabled={rowsResource.loading}
          size="xs"
        >
          <LuRefreshCw sx={stylexStyles.rule2} />
          Refresh
        </Button>
      </div>

      {view ? <TableViewFilterBar view={view} /> : null}

      <div sx={stylexStyles2.rule6}>
        <TableViewBody
          metaResource={metaResource}
          rowsResource={rowsResource}
          data={data}
          csvFileName={`${objectKey.replaceAll('/', '-')}.csv`}
        />
      </div>
    </div>
  )
}

/** TableViewFilterBar summarizes the projected columns, filter, and row limit of a table view. */
function TableViewFilterBar({ view }: { view: TableView }) {
  const projected = view.projectedColumns ?? []
  return (
    <div sx={stylexStyles2.rule7}>
      <span>
        columns:{' '}
        <span sx={stylexStyles2.rule8}>
          {projected.length ? projected.join(', ') : '*'}
        </span>
      </span>
      {view.whereExpression ? (
        <span>
          where: <span sx={stylexStyles2.rule8}>{view.whereExpression}</span>
        </span>
      ) : null}
      {view.rowLimit ? (
        <span>
          limit: <span sx={stylexStyles2.rule9}>{view.rowLimit}</span>
        </span>
      ) : null}
    </div>
  )
}

interface TableViewBodyProps {
  metaResource: Resource<GetTableViewResponse | null>
  rowsResource: Resource<FetchRowsResponse | null>
  data: SqlGridData | null
  csvFileName: string
}

/** TableViewBody shows the error and loading states, then the row grid once rows arrive. */
function TableViewBody({
  metaResource,
  rowsResource,
  data,
  csvFileName,
}: TableViewBodyProps) {
  return (
    <>
      {metaResource.error ? (
        <div sx={stylexStyles2.rule10}>
          <ErrorState
            title="Table view unavailable"
            message={String(metaResource.error)}
            onRetry={metaResource.retry}
          />
        </div>
      ) : null}
      {(metaResource.loading || rowsResource.loading) && data == null ? (
        <div sx={stylexStyles2.rule10}>
          <LoadingInline label="Loading rows" tone="muted" />
        </div>
      ) : null}
      {rowsResource.error ? (
        <div sx={stylexStyles2.rule10}>
          <ErrorState
            variant="inline"
            title="Could not fetch rows"
            message={String(rowsResource.error)}
            onRetry={rowsResource.retry}
          />
        </div>
      ) : null}
      {data ? (
        <SqlResultGrid
          data={data}
          csvFileName={csvFileName}
          emptyTitle="No matching rows"
          emptyDescription="No rows match this view's filter."
        />
      ) : null}
    </>
  )
}
