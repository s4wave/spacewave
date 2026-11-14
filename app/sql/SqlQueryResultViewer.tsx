import { LuExternalLink, LuTableProperties } from 'react-icons/lu'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

import {
  SqlQueryResult,
  SqlQueryResultTypeID,
} from '@s4wave/sdk/sql/query-result/query-result.js'
import type { GetResultGridResponse } from '@s4wave/sdk/sql/query-result/query-result.pb.js'

import { flattenRowBatches } from './sql-cell.js'
import { SqlResultGrid } from './SqlResultGrid.js'

export { SqlQueryResultTypeID }

// QueryResultBody shows the result's waiting or failed state, the query's own
// failure, or the result grid.
function QueryResultBody({
  gridResource,
  csvFileName,
}: {
  gridResource: Resource<GetResultGridResponse | null>
  csvFileName: string
}) {
  // A failed or pending read of the grid shows in place of the result.
  const grid = gridResource.value
  if (gridResource.error) {
    return (
      <ViewerStatus
        state="failed"
        message={`Could not read the query result: ${gridResource.error.message}`}
        retry={gridResource.retry}
      />
    )
  }
  if (!grid) {
    if (!gridResource.loading) return null
    return <ViewerStatus state="waiting" what="the query result" />
  }
  if (grid.error?.message) {
    return (
      <ViewerStatus
        state="failed"
        message={`The query failed: ${grid.error.message}`}
      />
    )
  }
  return (
    <SqlResultGrid
      data={{
        columns: grid.columns ?? [],
        rows: flattenRowBatches(grid.rowBatches),
        truncated: grid.truncated ?? false,
      }}
      csvFileName={csvFileName}
      emptyDescription="The query completed without returning rows."
    />
  )
}

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
        <QueryResultBody
          gridResource={gridResource}
          csvFileName={`${objectKey.replaceAll('/', '-')}.csv`}
        />
      </div>
    </div>
  )
}
