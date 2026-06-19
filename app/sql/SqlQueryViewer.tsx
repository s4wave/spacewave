import { useCallback, useMemo, useState } from 'react'
import {
  LuFileText,
  LuPlay,
  LuPlus,
  LuSave,
  LuTrash2,
  LuUndo2,
} from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'
import { SqlQuery, SqlQueryTypeID } from '@s4wave/sdk/sql/query/query.js'

import { buildParam, type SqlParamKind } from './sql-cell.js'

export { SqlQueryTypeID }

interface ParamRow {
  id: number
  kind: SqlParamKind
  text: string
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
  const container = SpaceContainerContext.useContextSafe()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SqlQuery,
    SqlQueryTypeID,
  )

  const queryResource = useResource(
    handle,
    async (query, signal) => {
      if (!query) return null
      return query.getQueryText(signal)
    },
    [],
  )

  const loaded = queryResource.value

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center gap-2 border-b px-4">
        <LuFileText className="text-foreground-alt size-3.5" aria-hidden />
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          SQL Query
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-4">
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
        {loaded ? (
          <SqlQueryEditor
            key={objectKey}
            handle={handle}
            sqlText={loaded.sqlText ?? ''}
            dialectHint={loaded.dialectHint ?? ''}
            targetDbObjectKey={loaded.targetDbObjectKey ?? ''}
            onOpenTargetDb={
              container && loaded.targetDbObjectKey
                ? () =>
                    container.navigateToObjects([
                      loaded.targetDbObjectKey ?? '',
                    ])
                : undefined
            }
            onResult={
              container
                ? (resultKey) => container.navigateToObjects([resultKey])
                : undefined
            }
          />
        ) : null}
      </div>
    </div>
  )
}

interface SqlQueryEditorProps {
  handle: ReturnType<typeof useAccessTypedHandle<SqlQuery>>
  sqlText: string
  dialectHint: string
  targetDbObjectKey: string
  onOpenTargetDb?: () => void
  onResult?: (resultObjectKey: string) => void
}

let nextParamId = 1

function SqlQueryEditor({
  handle,
  sqlText: savedSql,
  dialectHint: savedDialect,
  targetDbObjectKey: savedTargetDb,
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
  /* eslint-enable react-doctor/no-derived-useState */
  // Parameters are not exposed by GetQueryText, so the editor starts each
  // session with an empty list; saving persists the current rows.
  const [params, setParams] = useState<ParamRow[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const textDirty =
    sql !== savedSql || dialect !== savedDialect || targetDb !== savedTargetDb
  const paramsTouched = params.length > 0

  const buildParams = useCallback((): SqlValue[] => {
    return params.map((row) => buildParam(row.kind, row.text))
  }, [params])

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
      if (paramsTouched) {
        await query.setParameters(buildParams())
      }
    },
    [buildParams, dialect, paramsTouched, sql, targetDb, textDirty],
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
    if (!query || paramError) return
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
  }, [handle.value, onResult, paramError, persist])

  const handleDiscard = useCallback(() => {
    setSql(savedSql)
    setDialect(savedDialect)
    setTargetDb(savedTargetDb)
    setParams([])
    setError(null)
  }, [savedDialect, savedSql, savedTargetDb])

  const addParam = useCallback(() => {
    setParams((rows) => [
      ...rows,
      { id: nextParamId++, kind: 'text', text: '' },
    ])
  }, [])

  const updateParam = useCallback((id: number, patch: Partial<ParamRow>) => {
    setParams((rows) =>
      rows.map((row) => (row.id === id ? { ...row, ...patch } : row)),
    )
  }, [])

  const removeParam = useCallback((id: number) => {
    setParams((rows) => rows.filter((row) => row.id !== id))
  }, [])

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button
          variant="default"
          size="sm"
          onClick={handleRun}
          disabled={busy || paramError != null || !sql.trim()}
          className="h-7 gap-1 text-xs"
        >
          <LuPlay className="size-3.5" />
          Run
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={handleSave}
          disabled={busy || paramError != null || !(textDirty || paramsTouched)}
          className="h-7 gap-1 text-xs"
        >
          <LuSave className="size-3.5" />
          Save
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={handleDiscard}
          disabled={busy || !(textDirty || paramsTouched)}
          className="h-7 gap-1 text-xs"
        >
          <LuUndo2 className="size-3.5" />
          Discard
        </Button>
        {busy ? <LoadingInline label="Running" tone="muted" /> : null}
      </div>

      <label className="flex flex-col gap-1">
        <span className="text-foreground-alt/70 text-xs font-medium">SQL</span>
        <textarea
          aria-label="SQL text"
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          disabled={busy}
          spellCheck={false}
          rows={10}
          className={cn(
            'border-foreground/10 bg-background-primary text-foreground rounded-md border px-3 py-2 font-mono text-xs',
            'focus-visible:ring-ring focus-visible:ring-1 focus-visible:outline-none',
          )}
        />
      </label>

      <div className="grid gap-4 md:grid-cols-2">
        <label className="flex flex-col gap-1">
          <span className="text-foreground-alt/70 text-xs font-medium">
            Dialect Hint
          </span>
          <input
            aria-label="Dialect hint"
            value={dialect}
            onChange={(e) => setDialect(e.target.value)}
            disabled={busy}
            spellCheck={false}
            className="border-foreground/10 bg-background-primary text-foreground focus-visible:ring-ring rounded-md border px-3 py-1.5 font-mono text-xs focus-visible:ring-1 focus-visible:outline-none"
          />
        </label>
        <div className="flex flex-col gap-1">
          <span className="text-foreground-alt/70 text-xs font-medium">
            Target Database
          </span>
          <div className="flex items-center gap-2">
            <input
              aria-label="Target database object key"
              value={targetDb}
              onChange={(e) => setTargetDb(e.target.value)}
              disabled={busy}
              spellCheck={false}
              className="border-foreground/10 bg-background-primary text-foreground focus-visible:ring-ring min-w-0 flex-1 rounded-md border px-3 py-1.5 font-mono text-xs focus-visible:ring-1 focus-visible:outline-none"
            />
            {onOpenTargetDb ? (
              <Button
                variant="ghost"
                size="sm"
                onClick={onOpenTargetDb}
                className="h-7 shrink-0 text-xs"
              >
                Open
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      <div className="border-foreground/8 rounded-lg border">
        <div className="border-foreground/8 flex items-center justify-between border-b px-3 py-2">
          <span className="text-foreground text-xs font-medium">
            Parameters
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={addParam}
            disabled={busy}
            className="h-6 gap-1 px-2 text-xs"
          >
            <LuPlus className="size-3" />
            Add
          </Button>
        </div>
        {params.length === 0 ? (
          <div className="text-foreground-alt/40 p-3 text-xs">
            No bind parameters. Use positional placeholders in the SQL.
          </div>
        ) : (
          <div className="divide-foreground/8 divide-y">
            {params.map((row, index) => (
              <ParamEditorRow
                key={row.id}
                index={index}
                row={row}
                disabled={busy}
                onChange={updateParam}
                onRemove={removeParam}
              />
            ))}
          </div>
        )}
      </div>

      {paramError ? <ErrorState variant="inline" message={paramError} /> : null}
      {error ? (
        <ErrorState variant="inline" title="Run failed" message={error} />
      ) : null}
    </div>
  )
}

interface ParamEditorRowProps {
  index: number
  row: ParamRow
  disabled: boolean
  onChange: (id: number, patch: Partial<ParamRow>) => void
  onRemove: (id: number) => void
}

function ParamEditorRow({
  index,
  row,
  disabled,
  onChange,
  onRemove,
}: ParamEditorRowProps) {
  return (
    <div className="flex items-center gap-2 px-3 py-1.5">
      <span className="text-foreground-alt/50 w-6 shrink-0 text-right text-[0.6rem] tabular-nums">
        {index + 1}
      </span>
      <select
        aria-label={`Parameter ${index + 1} type`}
        value={row.kind}
        onChange={(e) =>
          onChange(row.id, { kind: e.target.value as SqlParamKind })
        }
        disabled={disabled}
        className="border-foreground/10 bg-background-primary text-foreground shrink-0 rounded border px-2 py-1 text-xs"
      >
        <option value="text">text</option>
        <option value="int">int</option>
        <option value="float">float</option>
        <option value="null">null</option>
      </select>
      <input
        aria-label={`Parameter ${index + 1} value`}
        value={row.text}
        onChange={(e) => onChange(row.id, { text: e.target.value })}
        disabled={disabled || row.kind === 'null'}
        spellCheck={false}
        placeholder={row.kind === 'null' ? 'NULL' : ''}
        className="border-foreground/10 bg-background-primary text-foreground focus-visible:ring-ring min-w-0 flex-1 rounded border px-2 py-1 font-mono text-xs focus-visible:ring-1 focus-visible:outline-none disabled:opacity-50"
      />
      <Button
        variant="ghost"
        size="sm"
        onClick={() => onRemove(row.id)}
        disabled={disabled}
        className="text-destructive/70 hover:text-destructive hover:bg-destructive/10 size-6 shrink-0 p-0"
      >
        <LuTrash2 className="size-3.5" />
      </Button>
    </div>
  )
}
