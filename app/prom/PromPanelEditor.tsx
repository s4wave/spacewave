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

import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { PromPanel } from './PromPanel.js'
import { filterSuggestions, suggestContext } from './expr-suggest.js'
import type { TimeRange } from './prom-view.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: { flex: 1, color: themeColors['--color-foreground'] },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridColumnEnd: '-1',
    gridColumnStart: '1',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
  },
  rule5: {
    gap: themeLayout['--spacing'],
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background-primary'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule9: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'flex' },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'block',
    fontFamily: themeFonts['--font-mono'],
    textAlign: 'left',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: '100%',
  },
  rule14: {
    flex: 1,
    gap: themeLayout['--spacing'],
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({ rule1: { marginLeft: 'auto' } })

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
  // panel is the panel being edited, or null for a new one.
  panel: Panel | null
  // removed is set when another member removed the edited panel.
  removed: boolean
  targets: PromTargets | null
  range: TimeRange
  nowMs: number
  onClose: () => void
}

// PromPanelEditor edits a panel's title, expression, unit and kind with
// metric and label suggestions and a live preview, and stores it on Save.
export function PromPanelEditor({
  dashboard,
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
  const metric = ctx.kind === 'label' ? ctx.metric : null
  const metricNames = useResource(
    (signal) => dashboard.labelValues('__name__', [], signal),
    [dashboard],
  )
  const labelNames = useResource(
    async (signal) => {
      if (metric === null) return null
      return dashboard.labelNames(metric ? [metric] : [], signal)
    },
    [dashboard, metric],
  )
  const names = ctx.kind === 'label' ? labelNames : metricNames
  const suggestions = useMemo(
    () =>
      ctx.kind === 'none'
        ? []
        : filterSuggestions(names.value ?? [], ctx.prefix),
    [ctx, names.value],
  )

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
      <section sx={stylexStyles2.rule1}>
        <p sx={stylexStyles2.rule2}>Another member removed this panel.</p>
        <Button variant="outline" size="sm" onClick={onClose}>
          Close
        </Button>
      </section>
    )
  }

  const titleMissing = !ed.draft.title?.trim()
  const saving = ed.save.state === 'saving'
  return (
    <section aria-label="Panel editor" sx={stylexStyles2.rule3}>
      <div sx={stylexStyles2.rule4}>
        <label sx={stylexStyles2.rule5}>
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
          <p sx={stylexStyles2.rule6}>A panel needs a title.</p>
        ) : null}

        <label sx={stylexStyles2.rule5}>
          Expression
          <textarea
            ref={exprRef}
            rows={3}
            spellCheck={false}
            value={ed.draft.expr ?? ''}
            sx={[stylexStyles2.rule7, stylexStyles2.rule8]}
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
            loading={names.loading && !names.value}
            names={suggestions}
            onPick={pick}
          />
        ) : null}

        <FormatFields
          draft={ed.draft}
          onChange={(patch) => dispatch({ kind: 'edit', patch })}
        />

        {ed.save.state === 'failed' ? (
          <p sx={stylexStyles2.rule9}>
            Could not save the panel: {ed.save.reason}.
          </p>
        ) : null}
        <div sx={stylexStyles2.rule10}>
          <Button
            size="sm"
            disabled={titleMissing || saving}
            onClick={() => void save()}
          >
            {ed.save.state === 'failed' ? 'Retry' : 'Save'}
          </Button>
          <Button variant="outline" size="sm" onClick={onClose}>
            {ed.save.state === 'failed' ? 'Discard' : 'Cancel'}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            sx={stylexStyles.rule1}
            onClick={() => dispatch({ kind: 'run' })}
          >
            Run
          </Button>
        </div>
      </div>

      <PromPanel
        query={dashboard}
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
    <div sx={stylexStyles2.rule11}>
      {loading ? (
        <div sx={stylexStyles2.rule12}>
          <LoadingInline label="Loading names" />
        </div>
      ) : (
        names.map((name) => (
          <button
            key={name}
            type="button"
            sx={stylexStyles2.rule13}
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
    <div sx={stylexStyles2.rule10}>
      <label sx={stylexStyles2.rule14}>
        Unit
        <select
          value={draft.unit ?? 0}
          sx={stylexStyles2.rule7}
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
      <label sx={stylexStyles2.rule14}>
        Show as
        <select
          value={draft.kind ?? 0}
          sx={stylexStyles2.rule7}
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
