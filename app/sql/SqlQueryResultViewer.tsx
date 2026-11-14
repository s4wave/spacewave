import { LuExternalLink, LuTableProperties } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

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
      <ViewerBar
        icon={<LuTableProperties />}
        title="Query Result"
        description={
          grid?.rowCount != null
            ? `${grid.rowCount.toString()} rows`
            : undefined
        }
        actions={[
          ...(container && grid?.sourceQueryObjectKey
            ? [
                {
                  id: 'query',
                  label: 'Query',
                  icon: <LuExternalLink />,
                  onClick: () => navigate(grid.sourceQueryObjectKey),
                },
              ]
            : []),
          ...(container && grid?.targetDbObjectKey
            ? [
                {
                  id: 'database',
                  label: 'Database',
                  icon: <LuExternalLink />,
                  onClick: () => navigate(grid.targetDbObjectKey),
                },
              ]
            : []),
        ]}
      />

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
