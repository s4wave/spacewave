import { useCallback, useState } from 'react'
import { LuTable } from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { createWorldObject } from '@s4wave/sdk/world/utils.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'

import { SqlDatabase } from '@s4wave/sdk/sql/sql.js'
import { SqlSchema, SqlSchemaTypeID } from '@s4wave/sdk/sql/schema/schema.js'
import { SqlTableViewTypeID } from '@s4wave/sdk/sql/table-view/table-view.js'
import { TableView } from '@s4wave/sdk/sql/table-view/table-view.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import { radii as themeRadii } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
})

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
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule6: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'default' },
  },
  rule9: {
    minWidth: 0,
    flex: 1,
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
    color: themeColors['--color-foreground'],
  },
  rule10: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule11: {
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

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
  useObjectName(schemaResource.value?.metadata?.displayName)

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
        using stage = await spaceWorld.stageWorldState()
        using cursor = await stage.buildStorageCursor()
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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <LuTable sx={stylexStyles.rule1} aria-hidden />
        <span sx={stylexStyles2.rule3}>SQL Schema</span>
        {schemaName ? <span sx={stylexStyles2.rule4}>{schemaName}</span> : null}
        {schema ? (
          <span sx={stylexStyles2.rule5}>
            {schema.tables.length}{' '}
            {schema.tables.length === 1 ? 'table' : 'tables'}
          </span>
        ) : null}
      </div>

      {openError ? (
        <div sx={stylexStyles2.rule6}>
          <ErrorState
            variant="inline"
            title="Could not open table"
            message={openError}
          />
        </div>
      ) : null}

      <div sx={stylexStyles2.rule7}>
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
            icon={<LuTable sx={stylexStyles.rule2} />}
            title="No tables"
            description="This schema has no tables yet."
          />
        ) : null}
        {schema && schema.tables.length > 0 ? (
          <div sx={stylexStyles3.rule1}>
            {schema.tables.map((table) => (
              <TableRow
                sx={nativeDividers.t8}
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
  onOpen: (tableName: string) => Promise<void>
}

// TableRow renders one table with a row count queried from the target database
// and an open action that materializes a sql/table-view.
function TableRow({
  sx,
  tableName,
  schemaName,
  dbHandle,
  canOpen,
  onOpen,
}: TableRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
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

  const handleOpen = useCallback(
    () => void onOpen(tableName),
    [onOpen, tableName],
  )

  const count = countResource.value

  return (
    <button
      type="button"
      onClick={handleOpen}
      disabled={!canOpen}
      sx={[stylexStyles2.rule8, sx]}
    >
      <LuTable sx={stylexStyles.rule3} />
      <span sx={stylexStyles2.rule9}>{tableName || 'Unnamed table'}</span>
      {countResource.loading && count == null ? (
        <span sx={stylexStyles2.rule10}>…</span>
      ) : count != null ? (
        <span sx={stylexStyles2.rule11}>{count.toString()} rows</span>
      ) : null}
    </button>
  )
}

function quote(identifier: string): string {
  return `\`${identifier.replaceAll('`', '``')}\``
}

// nativeDividers applies the list border only to its nonfinal physical rows.
const nativeDividers = stylex.create({
  t8: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 8%, transparent)',
    },
  },
})
