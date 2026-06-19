import { LuExternalLink, LuTableProperties } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import {
  SqlQueryResult,
  SqlQueryResultTypeID,
} from '@s4wave/sdk/sql/query-result/query-result.js'

import { flattenRowBatches } from './sql-cell.js'
import { SqlResultGrid } from './SqlResultGrid.js'

export { SqlQueryResultTypeID }

// SqlQueryResultViewer is the full sql/query-result viewer: a typed result grid
// over the persisted rows with NULL rendering, paging, and CSV export, plus
// links back to the source query and target database.
export function SqlQueryResultViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const container = SpaceContainerContext.useContextSafe()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlQueryResult,
    SqlQueryResultTypeID,
  )

  const gridResource = useResource(
    handle,
    async (result, signal) => {
      if (!result) return null
      return result.getResultGrid(signal)
    },
    [],
  )

  const grid = gridResource.value
  const data = grid
    ? {
        columns: grid.columns ?? [],
        rows: flattenRowBatches(grid.rowBatches),
        truncated: grid.truncated ?? false,
      }
    : null

  const navigate = (key: string | undefined) => {
    if (container && key) container.navigateToObjects([key])
  }

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuTableProperties
          className="text-foreground-alt size-3.5"
          aria-hidden
        />
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          Query Result
        </span>
        {grid?.rowCount != null ? (
          <span className="text-foreground-alt/50 text-xs">
            {grid.rowCount.toString()} rows
          </span>
        ) : null}
        <div className="flex-1" />
        {container && grid?.sourceQueryObjectKey ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate(grid.sourceQueryObjectKey)}
            className="h-6 gap-1 px-2 text-xs"
          >
            <LuExternalLink className="size-3" />
            Query
          </Button>
        ) : null}
        {container && grid?.targetDbObjectKey ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate(grid.targetDbObjectKey)}
            className="h-6 gap-1 px-2 text-xs"
          >
            <LuExternalLink className="size-3" />
            Database
          </Button>
        ) : null}
      </div>

      <div className="min-h-0 flex-1">
        {gridResource.loading && grid == null ? (
          <div className="p-4">
            <LoadingInline label="Loading result" tone="muted" />
          </div>
        ) : null}
        {gridResource.error ? (
          <div className="p-4">
            <ErrorState
              title="Query result unavailable"
              message={String(gridResource.error)}
              onRetry={gridResource.retry}
            />
          </div>
        ) : null}
        {grid?.error?.message ? (
          <div className="p-4">
            <ErrorState
              variant="inline"
              title="Query execution failed"
              message={grid.error.message}
            />
          </div>
        ) : null}
        {data && !grid?.error?.message ? (
          <SqlResultGrid
            data={data}
            csvFileName={`${objectKey.replaceAll('/', '-')}.csv`}
            emptyDescription="The query completed without returning rows."
          />
        ) : null}
      </div>
    </div>
  )
}
