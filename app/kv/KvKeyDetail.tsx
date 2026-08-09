import { useCallback, useEffect, useMemo, useState } from 'react'
import { LuSave, LuTrash2, LuUndo2 } from 'react-icons/lu'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="flex h-full flex-col">
      <div className="border-foreground/8 flex shrink-0 items-center gap-2 border-b px-3 py-2">
        <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
          {row.label}
        </span>
        <span className="text-foreground-alt/50 shrink-0 text-xs tabular-nums">
          {row.byteLength} B
        </span>
      </div>
      <div className="flex-1 overflow-auto p-3">
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
    <div className="space-y-3">
      <KvValueEditor
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
          variant="inline"
          message={`Save failed: ${mutationError}`}
        />
      ) : null}
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => void handleSave()}
          disabled={!dirty || busy || parseError != null}
          className="h-7 gap-1 text-xs"
        >
          <LuSave className="size-3.5" />
          Save
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={handleDiscard}
          disabled={!dirty || busy}
          className="h-7 gap-1 text-xs"
        >
          <LuUndo2 className="size-3.5" />
          Discard
        </Button>
        <div className="flex-1" />
        {confirmingDelete ? (
          <div className="flex items-center gap-2">
            <span className="text-destructive/80 text-xs">Delete key?</span>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => void handleConfirmDelete()}
              disabled={busy}
              className="h-7 gap-1 text-xs"
            >
              <LuTrash2 className="size-3.5" />
              Confirm
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setConfirmingDelete(false)}
              disabled={busy}
              className="h-7 text-xs"
            >
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setConfirmingDelete(true)}
            disabled={busy}
            className={cn(
              'text-destructive/80 hover:text-destructive hover:bg-destructive/10',
              'h-7 gap-1 text-xs',
            )}
          >
            <LuTrash2 className="size-3.5" />
            Delete
          </Button>
        )}
      </div>
    </div>
  )
}
