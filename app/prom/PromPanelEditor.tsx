import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useReducer,
  useRef,
} from 'react'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { PromDashboard, PromTargets } from '@s4wave/sdk/prom/prom.js'
import { PanelKind, PanelUnit, type Panel } from '@s4wave/sdk/prom/prom.pb.js'
import { useLiveDeps } from '@s4wave/web/hooks/useWatchTypedHandle.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { PromPanel } from './PromPanel.js'
import {
  filterSuggestions,
  suggestContext,
  type SuggestContext,
} from './expr-suggest.js'
import type { TimeRange } from './prom-view.js'

// unitOptions are the units offered for a panel, in menu order.
const unitOptions: readonly [PanelUnit, string][] = [
  [PanelUnit.UNKNOWN, 'Number'],
  [PanelUnit.PERCENT, 'Percent'],
  [PanelUnit.BYTES, 'Bytes'],
  [PanelUnit.BYTES_PER_SECOND, 'Bytes per second'],
  [PanelUnit.SECONDS, 'Seconds'],
  [PanelUnit.CELSIUS, 'Degrees Celsius'],
]

// field styles the editor's textarea and selects like its inputs.
const field =
  'border-foreground/10 bg-background-primary text-foreground rounded border px-2 py-1 text-xs'

// Editor is the state of the panel editor.
type Editor = {
  draft: Panel
  // caret is the expression's caret, for suggestions.
  caret: number
  // focused is set while the expression field has focus.
  focused: boolean
  // preview is the expression the preview runs; it follows the draft when
  // the field loses focus or on Run, not on every keystroke.
  preview: string
  save:
    | { state: 'idle' }
    | { state: 'saving' }
    | { state: 'failed'; reason: string }
}

// EditorEvent moves the editor between states.
type EditorEvent =
  | { kind: 'edit'; patch: Partial<Panel> }
  | { kind: 'expr'; expr: string; caret: number }
  | { kind: 'focus'; focused: boolean }
  | { kind: 'run' }
  | { kind: 'saving' }
  | { kind: 'failed'; reason: string }

// reduce applies one event to the editor.
function reduce(ed: Editor, ev: EditorEvent): Editor {
  switch (ev.kind) {
    case 'edit':
      return { ...ed, draft: { ...ed.draft, ...ev.patch } }
    case 'expr':
      return { ...ed, draft: { ...ed.draft, expr: ev.expr }, caret: ev.caret }
    case 'focus':
      return ev.focused
        ? { ...ed, focused: true }
        : { ...ed, focused: false, preview: ed.draft.expr ?? '' }
    case 'run':
      return { ...ed, preview: ed.draft.expr ?? '' }
    case 'saving':
      return { ...ed, save: { state: 'saving' } }
    case 'failed':
      return { ...ed, save: { state: 'failed', reason: ev.reason } }
  }
}

export interface PromPanelEditorProps {
  dashboard: PromDashboard
  // live is unset while dashboard is a held handle, lost or reloading: the
  // draft stays, no suggestion or preview query runs on it, and Save
  // returns with the next handle.
  live: boolean
  // panel is the panel being edited, or null for a new one.
  panel: Panel | null
  // removed is set when another member removed the edited panel.
  removed: boolean
  targets: PromTargets | null
  range: TimeRange
  nowMs: number
  onClose: () => void
}

// useSuggestions returns the metric or label names to offer at ctx, looked
// up on dashboard while live, and whether the first lookup is under way.
function useSuggestions(
  dashboard: PromDashboard,
  live: boolean,
  ctx: SuggestContext,
): { names: string[]; loading: boolean } {
  const metric = ctx.kind === 'label' ? ctx.metric : null
  const metricNames = useResource(
    async (signal) =>
      live ? dashboard.labelValues('__name__', [], signal) : null,
    useLiveDeps(live, [dashboard]),
  )
  const labelNames = useResource(
    async (signal) => {
      if (!live || metric === null) return null
      return dashboard.labelNames(metric ? [metric] : [], signal)
    },
    useLiveDeps(live, [dashboard, metric]),
  )
  const names = ctx.kind === 'label' ? labelNames : metricNames
  const offered = useMemo(
    () =>
      ctx.kind === 'none'
        ? []
        : filterSuggestions(names.value ?? [], ctx.prefix),
    [ctx, names.value],
  )
  return { names: offered, loading: names.loading && !names.value }
}

// PromPanelEditor edits a panel's title, expression, unit and kind with
// metric and label suggestions and a live preview, and stores it on Save.
export function PromPanelEditor({
  dashboard,
  live,
  panel,
  removed,
  targets,
  range,
  nowMs,
  onClose,
}: PromPanelEditorProps) {
  const [ed, dispatch] = useReducer(reduce, panel, (p): Editor => {
    const draft: Panel = p
      ? { ...p }
      : { title: '', expr: '', kind: PanelKind.CHART }
    return {
      draft,
      caret: draft.expr?.length ?? 0,
      focused: false,
      preview: draft.expr ?? '',
      save: { state: 'idle' },
    }
  })
  const exprRef = useRef<HTMLTextAreaElement>(null)
  const pendingCaret = useRef<number | null>(null)

  // Look up the names a suggestion can offer at the caret.
  const ctx = suggestContext(ed.draft.expr ?? '', ed.caret)
  const suggestions = useSuggestions(dashboard, live, ctx)

  // Replace the word at the caret with a picked name.
  const pick = useCallback(
    (name: string) => {
      if (ctx.kind === 'none') return
      const expr = ed.draft.expr ?? ''
      const next = expr.slice(0, ctx.start) + name + expr.slice(ed.caret)
      const caret = ctx.start + name.length
      pendingCaret.current = caret
      dispatch({ kind: 'expr', expr: next, caret })
    },
    [ctx, ed.draft.expr, ed.caret],
  )
  useLayoutEffect(() => {
    const caret = pendingCaret.current
    if (caret === null || !exprRef.current) return
    pendingCaret.current = null
    exprRef.current.focus()
    exprRef.current.setSelectionRange(caret, caret)
  })

  // Store the draft; the editor closes once the edit lands.
  const save = useCallback(async () => {
    dispatch({ kind: 'saving' })
    try {
      await dashboard.putPanel(ed.draft)
      onClose()
    } catch (err) {
      dispatch({
        kind: 'failed',
        reason: err instanceof Error ? err.message : String(err),
      })
    }
  }, [dashboard, ed.draft, onClose])

  const previewPanel = useMemo<Panel>(
    () => ({
      ...ed.draft,
      title: ed.draft.title || 'Preview',
      expr: ed.preview,
    }),
    [ed.draft, ed.preview],
  )

  if (removed) {
    return (
      <section className="border-foreground/8 flex items-center gap-3 rounded-md border p-3 text-sm">
        <p className="text-foreground flex-1">
          Another member removed this panel.
        </p>
        <Button variant="outline" size="sm" onClick={onClose}>
          Close
        </Button>
      </section>
    )
  }

  const titleMissing = !ed.draft.title?.trim()
  const saving = ed.save.state === 'saving'
  return (
    <section
      aria-label="Panel editor"
      className="border-brand/30 col-span-full grid gap-3 rounded-md border p-3 lg:grid-cols-2"
    >
      <div className="flex min-w-0 flex-col gap-2">
        <label className="text-foreground-alt flex flex-col gap-1 text-xs">
          Title
          <Input
            variant="toolbar"
            value={ed.draft.title ?? ''}
            onChange={(e) =>
              dispatch({ kind: 'edit', patch: { title: e.target.value } })
            }
          />
        </label>
        {titleMissing ? (
          <p className="text-foreground-alt text-xs">A panel needs a title.</p>
        ) : null}

        <label className="text-foreground-alt flex flex-col gap-1 text-xs">
          Expression
          <textarea
            ref={exprRef}
            rows={3}
            spellCheck={false}
            value={ed.draft.expr ?? ''}
            className={cn(field, 'font-mono')}
            onChange={(e) =>
              dispatch({
                kind: 'expr',
                expr: e.target.value,
                caret: e.target.selectionStart,
              })
            }
            onSelect={(e) =>
              dispatch({
                kind: 'expr',
                expr: e.currentTarget.value,
                caret: e.currentTarget.selectionStart,
              })
            }
            onFocus={() => dispatch({ kind: 'focus', focused: true })}
            onBlur={() => dispatch({ kind: 'focus', focused: false })}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault()
                dispatch({ kind: 'run' })
              }
            }}
          />
        </label>
        {ed.focused && ctx.kind !== 'none' ? (
          <Suggestions
            loading={suggestions.loading}
            names={suggestions.names}
            onPick={pick}
          />
        ) : null}

        <FormatFields
          draft={ed.draft}
          onChange={(patch) => dispatch({ kind: 'edit', patch })}
        />

        {ed.save.state === 'failed' ? (
          <p className="text-destructive text-xs">
            Could not save the panel: {ed.save.reason}.
          </p>
        ) : null}
        {!live ? (
          <p className="text-foreground-alt text-xs">
            Save returns once the Dashboard is read again.
          </p>
        ) : null}
        <div className="flex gap-2">
          {live ? (
            <Button
              size="sm"
              disabled={titleMissing || saving}
              onClick={() => void save()}
            >
              {ed.save.state === 'failed' ? 'Retry' : 'Save'}
            </Button>
          ) : null}
          <Button variant="outline" size="sm" onClick={onClose}>
            {ed.save.state === 'failed' ? 'Discard' : 'Cancel'}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto"
            onClick={() => dispatch({ kind: 'run' })}
          >
            Run
          </Button>
        </div>
      </div>

      <PromPanel
        query={dashboard}
        live={live}
        panel={previewPanel}
        range={range}
        targets={targets}
        nowMs={nowMs}
        hoverMs={null}
        onHover={noHover}
      />
    </section>
  )
}

// Suggestions lists the metric or label names matching the word at the
// caret. A pick keeps the field's focus.
function Suggestions({
  loading,
  names,
  onPick,
}: {
  loading: boolean
  names: readonly string[]
  onPick: (name: string) => void
}) {
  return (
    <div className="border-foreground/8 rounded border text-xs">
      {loading ? (
        <div className="px-2 py-1">
          <LoadingInline label="Loading names" />
        </div>
      ) : (
        names.map((name) => (
          <button
            key={name}
            type="button"
            className="hover:bg-foreground/5 text-foreground block w-full px-2 py-0.5 text-left font-mono"
            onMouseDown={(e) => {
              e.preventDefault()
              onPick(name)
            }}
          >
            {name}
          </button>
        ))
      )}
    </div>
  )
}

// FormatFields picks the panel's unit and whether it shows a chart or a
// value.
function FormatFields({
  draft,
  onChange,
}: {
  draft: Panel
  onChange: (patch: Partial<Panel>) => void
}) {
  return (
    <div className="flex gap-2">
      <label className="text-foreground-alt flex flex-1 flex-col gap-1 text-xs">
        Unit
        <select
          value={draft.unit ?? 0}
          className={field}
          onChange={(e) =>
            onChange({ unit: Number(e.target.value) as PanelUnit })
          }
        >
          {unitOptions.map(([unit, label]) => (
            <option key={unit} value={unit}>
              {label}
            </option>
          ))}
        </select>
      </label>
      <label className="text-foreground-alt flex flex-1 flex-col gap-1 text-xs">
        Show as
        <select
          value={draft.kind ?? 0}
          className={field}
          onChange={(e) =>
            onChange({ kind: Number(e.target.value) as PanelKind })
          }
        >
          <option value={PanelKind.CHART}>Chart</option>
          <option value={PanelKind.VALUE}>Value</option>
        </select>
      </label>
    </div>
  )
}

// noHover ignores the preview's hover; it does not share the Dashboard's.
function noHover() {}
