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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'default' },
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule9: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
  },
  rule11: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 8%, transparent)',
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})
