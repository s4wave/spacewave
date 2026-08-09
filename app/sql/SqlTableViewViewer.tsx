import { LuListFilter, LuRefreshCw } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import {
  SqlTableView,
  SqlTableViewTypeID,
} from '@s4wave/sdk/sql/table-view/table-view.js'

import { flattenRowBatches } from './sql-cell.js'
import { SqlResultGrid } from './SqlResultGrid.js'

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
  const rows = rowsResource.value
  const data = rows
    ? {
        columns: rows.columns ?? [],
        rows: flattenRowBatches(rows.rowBatches),
        truncated: rows.truncated ?? false,
      }
    : null

  const projected = view?.projectedColumns ?? []

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuListFilter className="text-foreground-alt size-3.5" aria-hidden />
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Table View
        </span>
        {view?.targetTableName ? (
          <span className="text-foreground-alt/50 truncate font-mono text-xs">
            {view.targetTableName}
          </span>
        ) : null}
        <div className="flex-1" />
        <Button
          variant="ghost"
          size="sm"
          onClick={rowsResource.retry}
          disabled={rowsResource.loading}
          className="h-6 gap-1 px-2 text-xs"
        >
          <LuRefreshCw className="size-3" />
          Refresh
        </Button>
      </div>

      {view ? (
        <div className="border-foreground/8 text-foreground-alt/70 flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-b px-4 py-1.5 text-xs">
          <span>
            columns:{' '}
            <span className="text-foreground font-mono">
              {projected.length ? projected.join(', ') : '*'}
            </span>
          </span>
          {view.whereExpression ? (
            <span>
              where:{' '}
              <span className="text-foreground font-mono">
                {view.whereExpression}
              </span>
            </span>
          ) : null}
          {view.rowLimit ? (
            <span>
              limit: <span className="text-foreground">{view.rowLimit}</span>
            </span>
          ) : null}
        </div>
      ) : null}

      <div className="min-h-0 flex-1">
        {metaResource.error ? (
          <div className="p-4">
            <ErrorState
              title="Table view unavailable"
              message={String(metaResource.error)}
              onRetry={metaResource.retry}
            />
          </div>
        ) : null}
        {(metaResource.loading || rowsResource.loading) && data == null ? (
          <div className="p-4">
            <LoadingInline label="Loading rows" tone="muted" />
          </div>
        ) : null}
        {rowsResource.error ? (
          <div className="p-4">
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
            csvFileName={`${objectKey.replaceAll('/', '-')}.csv`}
            emptyTitle="No matching rows"
            emptyDescription="No rows match this view's filter."
          />
        ) : null}
      </div>
    </div>
  )
}
