import { useCallback, useState } from 'react'
import {
  LuChevronDown,
  LuChevronRight,
  LuDatabase,
  LuPlus,
  LuTable,
} from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  ViewerStatus,
  resourceStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'

import { SqlDatabase, SqlDbTypeID } from '@s4wave/sdk/sql/sql.js'
import { SqlQueryTypeID } from '@s4wave/sdk/sql/query/query.js'

export { SqlDbTypeID }

// SqlDbViewer is the full sql/db viewer: a lazily expanded schema tree and an
// action that creates a sql/query child targeting this database, then routes to
// the new query editor.
export function SqlDbViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const container = SpaceContainerContext.useContextSafe()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlDatabase,
    SqlDbTypeID,
  )

  const schemasResource = useResource(
    handle,
    async (sql, signal) => {
      if (!sql) return null
      return sql.listSchemas(signal)
    },
    [],
  )

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  // handleOpenQueryEditor creates a sql/query child object bound to this
  // database and navigates to it. The query run RPC reads the target db key
  // from the persisted query object, so the binding is set at creation.
  const handleOpenQueryEditor = useCallback(async () => {
    if (!container || creating) return
    setCreating(true)
    setCreateError(null)
    try {
      const spaceWorld = container.spaceWorld
      const queryKey = `${objectKey}/query/${Date.now().toString(36)}`
      const objectState = await spaceWorld.createObject(queryKey, {})
      try {
        await setObjectType(spaceWorld, queryKey, SqlQueryTypeID)
      } finally {
        objectState.release()
      }
      container.navigateToObjects([queryKey])
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : String(err))
    } finally {
      setCreating(false)
    }
  }, [container, creating, objectKey])

  const schemas = schemasResource.value ?? []
  const read = resourceStatus('the database', schemasResource)
  let status: ViewerStatusProps | null = null
  if (read && (read.state === 'failed' || schemas.length === 0)) status = read
  else if (schemas.length === 0) {
    status = {
      state: 'empty',
      icon: <LuDatabase />,
      message: container
        ? 'This database has no schemas yet. Open the Query Editor and run CREATE DATABASE to add one.'
        : 'This database has no schemas yet.',
    }
  }

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuDatabase />}
        title="SQL Database"
        description={`${schemas.length} ${schemas.length === 1 ? 'schema' : 'schemas'}`}
        actions={
          container
            ? [
                {
                  id: 'query-editor',
                  label: creating ? 'Opening Query Editor…' : 'Query Editor',
                  icon: <LuPlus />,
                  onClick: () => void handleOpenQueryEditor(),
                  disabled: creating,
                },
              ]
            : []
        }
      />

      {createError ? (
        <div className="viewer-inset-x pt-3">
          <ErrorState
            variant="inline"
            title="Could not open query editor"
            message={createError}
          />
        </div>
      ) : null}

      {status ? (
        <div className="min-h-0 flex-1">
          <ViewerStatus {...status} />
        </div>
      ) : (
        <div className="viewer-inset min-h-0 flex-1 overflow-auto">
          <div role="tree" aria-label="Schemas" className="space-y-0.5">
            {schemas.map((schema) => (
              <SchemaTreeNode key={schema} handle={handle} schema={schema} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

interface SchemaTreeNodeProps {
  handle: ReturnType<typeof useAccessTypedHandle<SqlDatabase>>
  schema: string
}

// SchemaTreeNode renders one schema row and lazily loads its tables when
// expanded, never fetching tables for collapsed schemas.
function SchemaTreeNode({ handle, schema }: SchemaTreeNodeProps) {
  const [expanded, setExpanded] = useState(false)

  const tablesResource = useResource(
    handle,
    async (sql, signal) => {
      if (!sql || !expanded) return null
      return sql.listTables(schema, signal)
    },
    [expanded, schema],
  )

  const toggle = useCallback(() => setExpanded((value) => !value), [])

  const tables = tablesResource.value ?? []

  return (
    <div>
      <button
        type="button"
        role="treeitem"
        aria-expanded={expanded}
        onClick={toggle}
        className={cn(
          'flex w-full items-center gap-1.5 rounded px-2 py-1 text-left transition-colors',
          'hover:bg-foreground/5',
        )}
      >
        {expanded ? (
          <LuChevronDown className="text-foreground-alt/60 size-3.5 shrink-0" />
        ) : (
          <LuChevronRight className="text-foreground-alt/60 size-3.5 shrink-0" />
        )}
        <LuDatabase className="text-foreground-alt/70 size-3.5 shrink-0" />
        <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
          {schema}
        </span>
      </button>
      {expanded ? (
        <div className="border-foreground/8 ml-3.5 border-l pl-2">
          {tablesResource.loading && tables.length === 0 ? (
            <div className="py-1">
              <LoadingInline label="Loading tables" tone="muted" />
            </div>
          ) : null}
          {tablesResource.error ? (
            <ErrorState
              variant="inline"
              message={String(tablesResource.error)}
              onRetry={tablesResource.retry}
            />
          ) : null}
          {!tablesResource.loading &&
          !tablesResource.error &&
          tables.length === 0 ? (
            <div className="text-foreground-alt/40 px-2 py-1 text-xs">
              No tables
            </div>
          ) : null}
          {tables.map((table) => (
            <div
              key={table}
              role="treeitem"
              className="text-foreground flex items-center gap-1.5 px-2 py-1 font-mono text-xs"
            >
              <LuTable className="text-foreground-alt/60 size-3.5 shrink-0" />
              <span className="min-w-0 flex-1 truncate" title={table}>
                {table}
              </span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
