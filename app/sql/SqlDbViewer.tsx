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
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
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

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold tracking-tight select-none">
          <LuDatabase className="text-foreground-alt size-3.5" aria-hidden />
          SQL Database
          <span className="text-foreground-alt/50 text-xs">
            {schemas.length} {schemas.length === 1 ? 'schema' : 'schemas'}
          </span>
        </div>
        <DashboardButton
          icon={<LuPlus className="size-3.5" />}
          onClick={handleOpenQueryEditor}
          disabled={!container || creating}
        >
          Query Editor
        </DashboardButton>
      </div>

      {createError ? (
        <div className="px-4 pt-3">
          <ErrorState
            variant="inline"
            title="Could not open query editor"
            message={createError}
          />
        </div>
      ) : null}

      <div className="min-h-0 flex-1 overflow-auto p-3">
        {schemasResource.loading && schemas.length === 0 ? (
          <LoadingInline label="Loading schemas" tone="muted" />
        ) : null}
        {schemasResource.error ? (
          <ErrorState
            title="SQL database unavailable"
            message={String(schemasResource.error)}
            onRetry={schemasResource.retry}
          />
        ) : null}
        {!schemasResource.loading &&
        !schemasResource.error &&
        schemas.length === 0 ? (
          <EmptyState
            variant="compact"
            icon={<LuDatabase className="text-foreground-alt size-5" />}
            title="No schemas"
            description="This database has no schemas yet."
          />
        ) : null}
        {schemas.length > 0 ? (
          <div role="tree" aria-label="Schemas" className="space-y-0.5">
            {schemas.map((schema) => (
              <SchemaTreeNode key={schema} handle={handle} schema={schema} />
            ))}
          </div>
        ) : null}
      </div>
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
