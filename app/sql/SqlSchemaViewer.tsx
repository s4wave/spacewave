import { useCallback, useState } from 'react'
import { LuTable } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { createWorldObject } from '@s4wave/sdk/world/utils.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'

import { SqlDatabase } from '@s4wave/sdk/sql/sql.js'
import { SqlSchema, SqlSchemaTypeID } from '@s4wave/sdk/sql/schema/schema.js'
import { SqlTableViewTypeID } from '@s4wave/sdk/sql/table-view/table-view.js'
import { TableView } from '@s4wave/sdk/sql/table-view/table-view.pb.js'

export { SqlSchemaTypeID }

// SqlSchemaViewer is the full sql/schema viewer: a table list with live row
// counts queried from the target database, and an action that opens a table as
// a sql/table-view child object.
export function SqlSchemaViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const container = SpaceContainerContext.useContextSafe()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlSchema,
    SqlSchemaTypeID,
  )

  const schemaResource = useResource(
    handle,
    async (schema, signal) => {
      if (!schema) return null
      const [metadata, tables] = await Promise.all([
        schema.getSchema(signal),
        schema.listTables(signal),
      ])
      return { metadata: metadata.schema, tables: tables.tables ?? [] }
    },
    [],
  )

  // The row-count query targets the schema's owning database object. The handle
  // resolves to null until the schema metadata names a target db.
  const targetDbKey = schemaResource.value?.metadata?.targetDbObjectKey ?? ''
  const dbHandle = useAccessTypedHandle(worldState, targetDbKey, SqlDatabase)

  const [openError, setOpenError] = useState<string | null>(null)

  // handleOpenTable creates a sql/table-view child bound to this schema and
  // table, then routes to it.
  const handleOpenTable = useCallback(
    async (tableName: string) => {
      if (!container) return
      setOpenError(null)
      try {
        const spaceWorld = container.spaceWorld
        const viewKey = `${objectKey}/table/${tableName}/${Date.now().toString(36)}`
        using cursor = await spaceWorld.buildStorageCursor()
        const created = await createWorldObject(
          spaceWorld,
          cursor,
          viewKey,
          (objectCursor) =>
            objectCursor.setBlock({
              data: TableView.toBinary({
                targetSchemaObjectKey: objectKey,
                targetTableName: tableName,
              }),
              markDirty: true,
            }),
        )
        try {
          await setObjectType(spaceWorld, viewKey, SqlTableViewTypeID)
        } finally {
          created.objectState.release()
        }
        container.navigateToObjects([viewKey])
      } catch (err) {
        setOpenError(err instanceof Error ? err.message : String(err))
      }
    },
    [container, objectKey],
  )

  const schema = schemaResource.value
  const schemaName = schema?.metadata?.schemaName ?? ''

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuTable className="text-foreground-alt size-3.5" aria-hidden />
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          SQL Schema
        </span>
        {schemaName ? (
          <span className="text-foreground-alt/50 truncate font-mono text-xs">
            {schemaName}
          </span>
        ) : null}
        {schema ? (
          <span className="text-foreground-alt/50 text-xs">
            {schema.tables.length}{' '}
            {schema.tables.length === 1 ? 'table' : 'tables'}
          </span>
        ) : null}
      </div>

      {openError ? (
        <div className="px-4 pt-3">
          <ErrorState
            variant="inline"
            title="Could not open table"
            message={openError}
          />
        </div>
      ) : null}

      <div className="min-h-0 flex-1 overflow-auto p-3">
        {schemaResource.loading && schema == null ? (
          <LoadingInline label="Loading schema" tone="muted" />
        ) : null}
        {schemaResource.error ? (
          <ErrorState
            title="SQL schema unavailable"
            message={String(schemaResource.error)}
            onRetry={schemaResource.retry}
          />
        ) : null}
        {schema && schema.tables.length === 0 ? (
          <EmptyState
            variant="compact"
            icon={<LuTable className="text-foreground-alt size-5" />}
            title="No tables"
            description="This schema has no tables yet."
          />
        ) : null}
        {schema && schema.tables.length > 0 ? (
          <div className="border-foreground/8 divide-foreground/8 divide-y rounded-lg border">
            {schema.tables.map((table) => (
              <TableRow
                key={table.name ?? ''}
                tableName={table.name ?? ''}
                schemaName={schemaName}
                dbHandle={dbHandle}
                canOpen={container != null}
                onOpen={handleOpenTable}
              />
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

interface TableRowProps {
  tableName: string
  schemaName: string
  dbHandle: ReturnType<typeof useAccessTypedHandle<SqlDatabase>>
  canOpen: boolean
  onOpen: (tableName: string) => void
}

// TableRow renders one table with a row count queried from the target database
// and an open action that materializes a sql/table-view.
function TableRow({
  tableName,
  schemaName,
  dbHandle,
  canOpen,
  onOpen,
}: TableRowProps) {
  const countResource = useResource(
    dbHandle,
    async (db, signal) => {
      if (!db || !tableName) return null
      const qualified = schemaName
        ? `${quote(schemaName)}.${quote(tableName)}`
        : quote(tableName)
      const result = await db.query(
        `SELECT COUNT(*) FROM ${qualified}`,
        [],
        signal,
      )
      const cell = result.rows[0]?.values?.[0]?.value
      if (cell?.case === 'intValue') return cell.value
      return null
    },
    [tableName, schemaName],
  )

  const handleOpen = useCallback(() => onOpen(tableName), [onOpen, tableName])

  const count = countResource.value

  return (
    <button
      type="button"
      onClick={handleOpen}
      disabled={!canOpen}
      className="hover:bg-foreground/5 flex w-full items-center gap-2 px-3 py-2 text-left transition-colors disabled:cursor-default"
    >
      <LuTable className="text-foreground-alt/60 size-3.5 shrink-0" />
      <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
        {tableName || 'Unnamed table'}
      </span>
      {countResource.loading && count == null ? (
        <span className="text-foreground-alt/40 text-[0.6rem]">…</span>
      ) : count != null ? (
        <span className="text-foreground-alt/50 shrink-0 text-xs tabular-nums">
          {count.toString()} rows
        </span>
      ) : null}
    </button>
  )
}

function quote(identifier: string): string {
  return `\`${identifier.replaceAll('`', '``')}\``
}
