import { useCallback, useEffect, useMemo, useState } from 'react'
import { LuSave, LuTrash2, LuUndo2 } from '@s4wave/web/images/icons.js'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import type { KvStore } from '@s4wave/sdk/kv/kv.js'

import { KvValueEditor } from './KvValueEditor.js'
import {
  detectMode,
  parseValue,
  renderValue,
  type KvDisplayMode,
} from './kv-encoding.js'
import type { KvKeyRow } from './kv-viewer.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { display: 'flex', flexDirection: 'column', height: '100%' },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: {
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
  rule4: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    flex: 1,
    overflow: 'auto',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule7: { flex: 1 },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface KvKeyDetailProps {
  handle: Resource<KvStore>
  row: KvKeyRow
  onSave: (value: Uint8Array) => Promise<void>
  onDelete: () => Promise<void>
  mutationError: string | null
}

// KvKeyDetail shows the selected key's value with a display-mode toggle, an
// update flow with dirty tracking and save/discard, and a delete confirm.
export function KvKeyDetail({
  handle,
  row,
  onSave,
  onDelete,
  mutationError,
}: KvKeyDetailProps) {
  const valueResource = useResource(
    handle,
    async (kv, signal) => {
      if (!kv) return null
      return kv.get(row.key, signal)
    },
    [row.label],
  )

  const loadedValue = valueResource.value?.data ?? null

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>{row.label}</span>
        <span sx={stylexStyles2.rule4}>{row.byteLength} B</span>
      </div>
      <div sx={stylexStyles2.rule5}>
        {valueResource.loading && !loadedValue ? (
          <LoadingInline label="Loading value" tone="muted" />
        ) : null}
        {valueResource.error ? (
          <ErrorState
            variant="inline"
            title="Value unavailable"
            message={String(valueResource.error)}
            onRetry={valueResource.retry}
          />
        ) : null}
        {loadedValue ? (
          <KvKeyEditorPane
            key={row.label}
            loadedValue={loadedValue}
            onSave={onSave}
            onDelete={onDelete}
            mutationError={mutationError}
          />
        ) : null}
      </div>
    </div>
  )
}

interface KvKeyEditorPaneProps {
  loadedValue: Uint8Array
  onSave: (value: Uint8Array) => Promise<void>
  onDelete: () => Promise<void>
  mutationError: string | null
}

function KvKeyEditorPane({
  loadedValue,
  onSave,
  onDelete,
  mutationError,
}: KvKeyEditorPaneProps) {
  const [mode, setMode] = useState<KvDisplayMode>(() => detectMode(loadedValue))
  const savedDraft = useMemo(
    () => renderValue(loadedValue, mode),
    [loadedValue, mode],
  )
  const [draft, setDraft] = useState(savedDraft)
  const [busy, setBusy] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  // Re-render the saved value when the display mode changes, preserving any
  // unsaved edit only while the draft already diverged from the saved form.
  useEffect(() => {
    setDraft(savedDraft)
  }, [savedDraft])

  const dirty = draft !== savedDraft

  const parseError = useMemo(() => {
    if (!dirty) return null
    try {
      parseValue(draft, mode)
      return null
    } catch (err) {
      return err instanceof Error ? err.message : String(err)
    }
  }, [draft, mode, dirty])

  const handleSave = useCallback(async () => {
    let value: Uint8Array
    try {
      value = parseValue(draft, mode)
    } catch {
      return
    }
    setBusy(true)
    try {
      await onSave(value)
    } finally {
      setBusy(false)
    }
  }, [draft, mode, onSave])

  const handleDiscard = useCallback(() => {
    setDraft(savedDraft)
  }, [savedDraft])

  const handleConfirmDelete = useCallback(async () => {
    setBusy(true)
    try {
      await onDelete()
    } finally {
      setBusy(false)
      setConfirmingDelete(false)
    }
  }, [onDelete])

  return (
    <div sx={false}>
      <KvValueEditor
        sx={nativeChildSpacing.s3}
        label="Value"
        ariaLabel="Key value"
        draft={draft}
        onDraftChange={setDraft}
        mode={mode}
        onModeChange={setMode}
        parseError={parseError}
        disabled={busy}
        rows={10}
      />
      {mutationError ? (
        <ErrorState
          sx={nativeChildSpacing.s3}
          variant="inline"
          message={`Save failed: ${mutationError}`}
        />
      ) : null}
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule6]}>
        <Button
          variant="outline"
          onClick={() => void handleSave()}
          disabled={!dirty || busy || parseError != null}
          size="toolbar"
        >
          <LuSave sx={stylexStyles.rule1} />
          Save
        </Button>
        <Button
          variant="ghost"
          onClick={handleDiscard}
          disabled={!dirty || busy}
          size="toolbar"
        >
          <LuUndo2 sx={stylexStyles.rule1} />
          Discard
        </Button>
        <div sx={stylexStyles2.rule7} />
        {confirmingDelete ? (
          <div sx={stylexStyles2.rule6}>
            <span sx={stylexStyles2.rule8}>Delete key?</span>
            <Button
              variant="destructive"
              onClick={() => void handleConfirmDelete()}
              disabled={busy}
              size="toolbar"
            >
              <LuTrash2 sx={stylexStyles.rule1} />
              Confirm
            </Button>
            <Button
              variant="ghost"
              onClick={() => setConfirmingDelete(false)}
              disabled={busy}
              size="toolbarText"
            >
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            onClick={() => setConfirmingDelete(true)}
            disabled={busy}
            variant="dangerGhost"
            size="toolbar"
          >
            <LuTrash2 sx={stylexStyles.rule1} />
            Delete
          </Button>
        )}
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
