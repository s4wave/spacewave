import { use, useCallback, useMemo, useState } from 'react'
import {
  LuFileText,
  LuPlay,
  LuPlus,
  LuSave,
  LuTrash2,
  LuUndo2,
} from '@s4wave/web/images/icons.js'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'

import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'
import { SqlDbTypeID } from '@s4wave/sdk/sql/sql.js'
import { SqlQuery, SqlQueryTypeID } from '@s4wave/sdk/sql/query/query.js'
import type { GetQueryTextResponse } from '@s4wave/sdk/sql/query/query.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'

import {
  buildParam,
  paramKind,
  paramText,
  type SqlParamKind,
} from './sql-cell.js'
import { SqlWorkbenchTargetDbContext } from './sql-workbench-context.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
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
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    marginInline: 'auto',
    display: 'flex',
    maxWidth: 'var(--container-4xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-primary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule11: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-primary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule12: {
    minWidth: 0,
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-primary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule13: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule16: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule17: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule18: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    textAlign: 'right',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule19: {
    flexShrink: 0,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-primary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule20: {
    minWidth: 0,
    flex: 1,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-primary'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({ rule1: { flexShrink: 0 } })

export { SqlQueryTypeID }

interface ParamRow {
  id: number
  kind: SqlParamKind
  text: string
}

interface TargetResolution {
  loading: boolean
  targetDbObjectKey: string
  hint: string
}

/**
 * resolveTargetDb picks the database a query runs against: the persisted
 * target, then the workbench target, then the Space's only database.
 */
function resolveTargetDb(
  persistedTarget: string | undefined,
  workbenchTarget: string,
  databaseKeysResource: Resource<string[] | null>,
): TargetResolution {
  if (persistedTarget) {
    return { loading: false, targetDbObjectKey: '', hint: '' }
  }
  if (workbenchTarget) {
    return { loading: false, targetDbObjectKey: workbenchTarget, hint: '' }
  }
  if (databaseKeysResource.error) {
    return {
      loading: false,
      targetDbObjectKey: '',
      hint: `Could not resolve a default database target: ${String(databaseKeysResource.error)}`,
    }
  }
  const databaseKeys = databaseKeysResource.value ?? []
  if (databaseKeys.length === 1) {
    return {
      loading: false,
      targetDbObjectKey: databaseKeys[0] ?? '',
      hint: '',
    }
  }
  if (databaseKeysResource.loading && databaseKeysResource.value == null) {
    return { loading: true, targetDbObjectKey: '', hint: '' }
  }
  const hint =
    databaseKeys.length === 0
      ? 'No SQL database is available in this Space. Create or open a database before running this query.'
      : 'Multiple SQL databases are available. Choose a target database before running.'
  return { loading: false, targetDbObjectKey: '', hint }
}

// SqlQueryViewer is the full sql/query editor: a SQL text area, typed parameter
// rows, dialect and target-database fields, dirty-tracked save, and a run action
// that executes the persisted query, creates a sql/query-result, and routes to
// it. The run RPC reads its inputs from the persisted query object, so a dirty
// editor saves before running.
export function SqlQueryViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlQuery,
    SqlQueryTypeID,
  )

  const workbenchTargetDb = use(SqlWorkbenchTargetDbContext)

  const queryResource = useResource(
    handle,
    async (query, signal) => {
      if (!query) return null
      return query.getQueryText(signal)
    },
    [],
  )

  const loaded = queryResource.value
  const needsSpaceDefaultTarget =
    !!loaded && !loaded.targetDbObjectKey && !workbenchTargetDb
  const databaseKeysResource = useResource(
    worldState,
    async (world, signal) => {
      if (!world) return null
      return listObjectsWithType(world, SqlDbTypeID, signal)
    },
    [],
    { enabled: needsSpaceDefaultTarget },
  )

  const targetResolution = resolveTargetDb(
    loaded?.targetDbObjectKey,
    workbenchTargetDb,
    databaseKeysResource,
  )

  return (
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <LuFileText sx={stylexStyles2.rule1} aria-hidden />
        <span sx={stylexStyles3.rule3}>SQL Query</span>
      </div>
      <div sx={stylexStyles3.rule4}>
        <SqlQueryBody
          objectKey={objectKey}
          handle={handle}
          queryResource={queryResource}
          targetResolution={targetResolution}
        />
      </div>
    </div>
  )
}

interface SqlQueryBodyProps {
  objectKey: string
  handle: ReturnType<typeof useAccessTypedHandle<SqlQuery>>
  queryResource: Resource<GetQueryTextResponse | null>
  targetResolution: TargetResolution
}

/** SqlQueryBody shows the loading and error states, then the editor once the query and its target resolve. */
function SqlQueryBody({
  objectKey,
  handle,
  queryResource,
  targetResolution,
}: SqlQueryBodyProps) {
  const loaded = queryResource.value
  return (
    <>
      {queryResource.loading && loaded == null ? (
        <LoadingInline label="Loading query" tone="muted" />
      ) : null}
      {queryResource.error ? (
        <ErrorState
          title="SQL query unavailable"
          message={String(queryResource.error)}
          onRetry={queryResource.retry}
        />
      ) : null}
      {loaded && targetResolution.loading ? (
        <LoadingInline label="Resolving SQL database target" tone="muted" />
      ) : null}
      {loaded && !targetResolution.loading ? (
        <ResolvedSqlQueryEditor
          objectKey={objectKey}
          handle={handle}
          loaded={loaded}
          targetResolution={targetResolution}
        />
      ) : null}
    </>
  )
}

interface ResolvedSqlQueryEditorProps {
  objectKey: string
  handle: ReturnType<typeof useAccessTypedHandle<SqlQuery>>
  loaded: GetQueryTextResponse
  targetResolution: TargetResolution
}

/** ResolvedSqlQueryEditor mounts the editor for a loaded query and routes opens to the Space container. */
function ResolvedSqlQueryEditor({
  objectKey,
  handle,
  loaded,
  targetResolution,
}: ResolvedSqlQueryEditorProps) {
  const container = SpaceContainerContext.useContextSafe()
  const targetDbObjectKey =
    loaded.targetDbObjectKey || targetResolution.targetDbObjectKey
  return (
    <SqlQueryEditor
      key={`${objectKey}:${targetDbObjectKey}`}
      handle={handle}
      sqlText={loaded.sqlText ?? ''}
      dialectHint={loaded.dialectHint ?? ''}
      targetDbObjectKey={targetDbObjectKey}
      persistedTargetDbObjectKey={loaded.targetDbObjectKey ?? ''}
      targetDbHint={targetResolution.hint}
      parameters={loaded.parameters ?? []}
      onOpenTargetDb={
        container ? (key) => container.navigateToObjects([key]) : undefined
      }
      onResult={
        container
          ? (resultKey) => container.navigateToObjects([resultKey])
          : undefined
      }
    />
  )
}

interface SqlQueryEditorProps {
  handle: ReturnType<typeof useAccessTypedHandle<SqlQuery>>
  sqlText: string
  dialectHint: string
  targetDbObjectKey: string
  persistedTargetDbObjectKey: string
  targetDbHint: string
  parameters: SqlValue[]
  onOpenTargetDb?: (targetDbObjectKey: string) => void
  onResult?: (resultObjectKey: string) => void
}

let nextParamId = 1

function SqlQueryEditor({
  handle,
  sqlText: savedSql,
  dialectHint: savedDialect,
  targetDbObjectKey: savedTargetDb,
  persistedTargetDbObjectKey: persistedTargetDb,
  targetDbHint,
  parameters: savedParameters,
  onOpenTargetDb,
  onResult,
}: SqlQueryEditorProps) {
  // The editor seeds its draft state from the persisted query once; the parent
  // keys this component on the object key, so a different query remounts and
  // reseeds rather than silently overwriting an in-progress edit.
  /* eslint-disable react-doctor/no-derived-useState */
  const [sql, setSql] = useState(savedSql)
  const [dialect, setDialect] = useState(savedDialect)
  const [targetDb, setTargetDb] = useState(savedTargetDb)
  const [params, setParams] = useState<ParamRow[]>(() =>
    paramRowsFromValues(savedParameters),
  )
  /* eslint-enable react-doctor/no-derived-useState */
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const textDirty =
    sql !== savedSql ||
    dialect !== savedDialect ||
    targetDb !== persistedTargetDb
  const buildParams = useCallback((): SqlValue[] => {
    return params.map((row) => buildParam(row.kind, row.text))
  }, [params])
  const paramsDirty = useMemo(() => {
    try {
      return !sqlValuesEqual(buildParams(), savedParameters)
    } catch {
      return true
    }
  }, [buildParams, savedParameters])

  const paramError = useMemo(() => {
    try {
      buildParams()
      return null
    } catch (err) {
      return err instanceof Error ? err.message : String(err)
    }
  }, [buildParams])

  const persist = useCallback(
    async (query: SqlQuery): Promise<void> => {
      if (textDirty) {
        await query.setQueryText(sql, dialect, targetDb)
      }
      if (paramsDirty) {
        await query.setParameters(buildParams())
      }
    },
    [buildParams, dialect, paramsDirty, sql, targetDb, textDirty],
  )

  const handleSave = useCallback(async () => {
    const query = handle.value
    if (!query || paramError) return
    setBusy(true)
    setError(null)
    try {
      await persist(query)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [handle.value, paramError, persist])

  const handleRun = useCallback(async () => {
    const query = handle.value
    if (!query || paramError || !targetDb.trim()) return
    setBusy(true)
    setError(null)
    try {
      await persist(query)
      const resp = await query.run(0)
      if (resp.error) {
        setError(resp.error)
      }
      if (resp.resultObjectKey) {
        onResult?.(resp.resultObjectKey)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }, [handle.value, onResult, paramError, persist, targetDb])

  const handleDiscard = useCallback(() => {
    setSql(savedSql)
    setDialect(savedDialect)
    setTargetDb(savedTargetDb)
    setParams(paramRowsFromValues(savedParameters))
    setError(null)
  }, [savedDialect, savedParameters, savedSql, savedTargetDb])

  const addParam = useCallback(() => {
    const id = nextParamId++
    setParams((rows) => [...rows, { id, kind: 'text', text: '' }])
  }, [])

  const updateParam = useCallback((id: number, patch: Partial<ParamRow>) => {
    setParams((rows) =>
      rows.map((row) => (row.id === id ? { ...row, ...patch } : row)),
    )
  }, [])

  const removeParam = useCallback((id: number) => {
    setParams((rows) => rows.filter((row) => row.id !== id))
  }, [])

  const canRun =
    !busy && paramError == null && !!sql.trim() && !!targetDb.trim()

  return (
    <div sx={stylexStyles3.rule5}>
      <QueryToolbar
        busy={busy}
        canRun={canRun}
        canSave={!busy && paramError == null && (textDirty || paramsDirty)}
        canDiscard={!busy && (textDirty || paramsDirty)}
        onRun={() => void handleRun()}
        onSave={() => void handleSave()}
        onDiscard={handleDiscard}
      />

      <label sx={stylexStyles3.rule6}>
        <span sx={stylexStyles3.rule7}>SQL</span>
        <textarea
          aria-label="SQL text"
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          disabled={busy}
          spellCheck={false}
          rows={10}
          sx={[stylexStyles3.rule8]}
        />
      </label>

      <QueryTargetFields
        dialect={dialect}
        targetDb={targetDb}
        busy={busy}
        onDialectChange={setDialect}
        onTargetDbChange={setTargetDb}
        onOpenTargetDb={onOpenTargetDb}
      />

      <ParametersPanel
        params={params}
        busy={busy}
        onAdd={addParam}
        onChange={updateParam}
        onRemove={removeParam}
      />

      {paramError ? <ErrorState variant="inline" message={paramError} /> : null}
      {!targetDb.trim() && targetDbHint ? (
        <ErrorState
          variant="inline"
          title="Target database required"
          message={targetDbHint}
        />
      ) : null}
      {error ? (
        <ErrorState variant="inline" title="Run failed" message={error} />
      ) : null}
    </div>
  )
}

interface QueryToolbarProps {
  busy: boolean
  canRun: boolean
  canSave: boolean
  canDiscard: boolean
  onRun: () => void
  onSave: () => void
  onDiscard: () => void
}

/** QueryToolbar renders the run, save, and discard actions of the query editor. */
function QueryToolbar({
  busy,
  canRun,
  canSave,
  canDiscard,
  onRun,
  onSave,
  onDiscard,
}: QueryToolbarProps) {
  return (
    <div sx={stylexStyles3.rule9}>
      <Button
        variant="default"
        onClick={onRun}
        disabled={!canRun}
        size="toolbar"
      >
        <LuPlay sx={stylexStyles2.rule2} />
        Run
      </Button>
      <Button
        variant="outline"
        onClick={onSave}
        disabled={!canSave}
        size="toolbar"
      >
        <LuSave sx={stylexStyles2.rule2} />
        Save
      </Button>
      <Button
        variant="ghost"
        onClick={onDiscard}
        disabled={!canDiscard}
        size="toolbar"
      >
        <LuUndo2 sx={stylexStyles2.rule2} />
        Discard
      </Button>
      {busy ? <LoadingInline label="Running" tone="muted" /> : null}
    </div>
  )
}

interface QueryTargetFieldsProps {
  dialect: string
  targetDb: string
  busy: boolean
  onDialectChange: (dialect: string) => void
  onTargetDbChange: (targetDb: string) => void
  onOpenTargetDb?: (targetDbObjectKey: string) => void
}

/** QueryTargetFields edits the dialect hint and the target database key. */
function QueryTargetFields({
  dialect,
  targetDb,
  busy,
  onDialectChange,
  onTargetDbChange,
  onOpenTargetDb,
}: QueryTargetFieldsProps) {
  return (
    <div sx={stylexStyles3.rule10}>
      <label sx={stylexStyles3.rule6}>
        <span sx={stylexStyles3.rule7}>Dialect Hint</span>
        <input
          aria-label="Dialect hint"
          value={dialect}
          onChange={(e) => onDialectChange(e.target.value)}
          disabled={busy}
          spellCheck={false}
          sx={stylexStyles3.rule11}
        />
      </label>
      <div sx={stylexStyles3.rule6}>
        <span sx={stylexStyles3.rule7}>Target Database</span>
        <div sx={stylexStyles3.rule9}>
          <input
            aria-label="Target database object key"
            value={targetDb}
            onChange={(e) => onTargetDbChange(e.target.value)}
            disabled={busy}
            spellCheck={false}
            sx={stylexStyles3.rule12}
          />
          {onOpenTargetDb && targetDb.trim() ? (
            <Button
              variant="ghost"
              onClick={() => onOpenTargetDb(targetDb)}
              sx={stylexStyles.rule1}
              size="toolbarText"
            >
              Open
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  )
}

interface ParametersPanelProps {
  params: ParamRow[]
  busy: boolean
  onAdd: () => void
  onChange: (id: number, patch: Partial<ParamRow>) => void
  onRemove: (id: number) => void
}

/** ParametersPanel lists the typed bind parameters and adds new ones. */
function ParametersPanel({
  params,
  busy,
  onAdd,
  onChange,
  onRemove,
}: ParametersPanelProps) {
  return (
    <div sx={stylexStyles3.rule13}>
      <div sx={stylexStyles3.rule14}>
        <span sx={stylexStyles3.rule15}>Parameters</span>
        <Button variant="ghost" onClick={onAdd} disabled={busy} size="xs">
          <LuPlus sx={stylexStyles2.rule3} />
          Add
        </Button>
      </div>
      {params.length === 0 ? (
        <div sx={stylexStyles3.rule16}>
          No bind parameters. Use positional placeholders in the SQL.
        </div>
      ) : (
        <div>
          {params.map((row, index) => (
            <ParamEditorRow
              sx={nativeDividers.t8}
              key={row.id}
              index={index}
              row={row}
              disabled={busy}
              onChange={onChange}
              onRemove={onRemove}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function paramRowsFromValues(values: SqlValue[]): ParamRow[] {
  const rows: ParamRow[] = []
  for (const value of values) {
    rows.push({
      id: nextParamId++,
      kind: paramKind(value),
      text: paramText(value),
    })
  }
  return rows
}

function sqlValuesEqual(left: SqlValue[], right: SqlValue[]): boolean {
  if (left.length !== right.length) return false
  for (let i = 0; i < left.length; i++) {
    if (!sqlValueEqual(left[i], right[i])) return false
  }
  return true
}

function sqlValueEqual(
  left: SqlValue | undefined,
  right: SqlValue | undefined,
): boolean {
  const leftValue = left?.value
  const rightValue = right?.value
  if (leftValue?.case !== rightValue?.case) return false
  switch (leftValue?.case) {
    case undefined:
      return true
    case 'intValue':
      return (
        rightValue?.case === 'intValue' && leftValue.value === rightValue.value
      )
    case 'floatValue':
      return (
        rightValue?.case === 'floatValue' &&
        leftValue.value === rightValue.value
      )
    case 'strValue':
      return (
        rightValue?.case === 'strValue' && leftValue.value === rightValue.value
      )
    case 'blobValue':
      return (
        rightValue?.case === 'blobValue' &&
        bytesEqual(leftValue.value, rightValue.value)
      )
  }
}

function bytesEqual(left: Uint8Array, right: Uint8Array | undefined): boolean {
  if (!right || left.length !== right.length) return false
  for (let i = 0; i < left.length; i++) {
    if (left[i] !== right[i]) return false
  }
  return true
}

function paramKindFromInput(value: string): SqlParamKind {
  switch (value) {
    case 'text':
    case 'int':
    case 'float':
    case 'blob':
    case 'null':
      return value
    default:
      return 'text'
  }
}

interface ParamEditorRowProps {
  index: number
  row: ParamRow
  disabled: boolean
  onChange: (id: number, patch: Partial<ParamRow>) => void
  onRemove: (id: number) => void
}

function ParamEditorRow({
  sx,
  index,
  row,
  disabled,
  onChange,
  onRemove,
}: ParamEditorRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles3.rule17, sx]}>
      <span sx={stylexStyles3.rule18}>{index + 1}</span>
      <select
        aria-label={`Parameter ${index + 1} type`}
        value={row.kind}
        onChange={(e) =>
          onChange(row.id, { kind: paramKindFromInput(e.target.value) })
        }
        disabled={disabled}
        sx={stylexStyles3.rule19}
      >
        <option value="text">text</option>
        <option value="int">int</option>
        <option value="float">float</option>
        <option value="blob">blob</option>
        <option value="null">null</option>
      </select>
      <input
        aria-label={`Parameter ${index + 1} value`}
        value={row.text}
        onChange={(e) => onChange(row.id, { text: e.target.value })}
        disabled={disabled || row.kind === 'null'}
        spellCheck={false}
        placeholder={row.kind === 'null' ? 'NULL' : ''}
        sx={stylexStyles3.rule20}
      />
      <Button
        onClick={() => onRemove(row.id)}
        disabled={disabled}
        variant="dangerGhost"
        size="iconXs"
        sx={stylexStyles.rule1}
      >
        <LuTrash2 sx={stylexStyles2.rule2} />
      </Button>
    </div>
  )
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
