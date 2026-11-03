import { useCallback, useState } from 'react'
import {
  LuChevronDown,
  LuChevronRight,
  LuDatabase,
  LuPlus,
  LuTable,
} from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'

import { SqlDatabase, SqlDbTypeID } from '@s4wave/sdk/sql/sql.js'
import { SqlQueryTypeID } from '@s4wave/sdk/sql/query/query.js'

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
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
  },
  rule7: {
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule8: {
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
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    marginLeft: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: { paddingBlock: themeLayout['--spacing'] },
  rule11: {
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule13: {
    flex: 1,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <LuDatabase sx={stylexStyles.rule1} aria-hidden />
          SQL Database
          <span sx={stylexStyles2.rule4}>
            {schemas.length} {schemas.length === 1 ? 'schema' : 'schemas'}
          </span>
        </div>
        <DashboardButton
          icon={<LuPlus sx={stylexStyles.rule2} />}
          onClick={() => void handleOpenQueryEditor()}
          disabled={!container || creating}
        >
          Query Editor
        </DashboardButton>
      </div>

      {createError ? (
        <div sx={stylexStyles2.rule5}>
          <ErrorState
            variant="inline"
            title="Could not open query editor"
            message={createError}
          />
        </div>
      ) : null}

      <div sx={stylexStyles2.rule6}>
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
            icon={<LuDatabase sx={stylexStyles.rule3} />}
            title="No schemas"
            description="This database has no schemas yet."
          />
        ) : null}
        {schemas.length > 0 ? (
          <div role="tree" aria-label="Schemas" sx={false}>
            {schemas.map((schema) => (
              <SchemaTreeNode
                sx={nativeChildSpacing.s0_5}
                key={schema}
                handle={handle}
                schema={schema}
              />
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
function SchemaTreeNode({
  sx,
  handle,
  schema,
}: SchemaTreeNodeProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
    <div sx={sx}>
      <button
        type="button"
        role="treeitem"
        aria-expanded={expanded}
        onClick={toggle}
        sx={[stylexStyles2.rule7]}
      >
        {expanded ? (
          <LuChevronDown sx={stylexStyles.rule4} />
        ) : (
          <LuChevronRight sx={stylexStyles.rule4} />
        )}
        <LuDatabase sx={stylexStyles.rule5} />
        <span sx={stylexStyles2.rule8}>{schema}</span>
      </button>
      {expanded ? (
        <div sx={stylexStyles2.rule9}>
          {tablesResource.loading && tables.length === 0 ? (
            <div sx={stylexStyles2.rule10}>
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
            <div sx={stylexStyles2.rule11}>No tables</div>
          ) : null}
          {tables.map((table) => (
            <div key={table} role="treeitem" sx={stylexStyles2.rule12}>
              <LuTable sx={stylexStyles.rule4} />
              <span sx={stylexStyles2.rule13} title={table}>
                {table}
              </span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s0_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
  },
})
