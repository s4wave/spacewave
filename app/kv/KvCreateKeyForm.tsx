import { useCallback, useMemo, useState } from 'react'
import { LuPlus, LuX } from 'react-icons/lu'

import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'

import { KvValueEditor } from './KvValueEditor.js'
import { parseValue, type KvDisplayMode } from './kv-encoding.js'

interface KvCreateKeyFormProps {
  onCreate: (key: Uint8Array, value: Uint8Array) => Promise<void>
  onCancel: () => void
  mutationError: string | null
}

// KvCreateKeyForm collects a new key and value using the shared display-mode
// editors and submits them as a single write.
export function KvCreateKeyForm({
  onCreate,
  onCancel,
  mutationError,
}: KvCreateKeyFormProps) {
  const [keyDraft, setKeyDraft] = useState('')
  const [keyMode, setKeyMode] = useState<KvDisplayMode>('text')
  const [valueDraft, setValueDraft] = useState('')
  const [valueMode, setValueMode] = useState<KvDisplayMode>('text')
  const [busy, setBusy] = useState(false)

  const keyError = useMemo(
    () => parseDraftError(keyDraft, keyMode),
    [keyDraft, keyMode],
  )
  const valueError = useMemo(
    () => parseDraftError(valueDraft, valueMode),
    [valueDraft, valueMode],
  )

  const keyEmpty = keyDraft.length === 0
  const canCreate = !keyEmpty && keyError == null && valueError == null && !busy

  const handleCreate = useCallback(async () => {
    let key: Uint8Array
    let value: Uint8Array
    try {
      key = parseValue(keyDraft, keyMode)
      value = parseValue(valueDraft, valueMode)
    } catch {
      return
    }
    if (key.length === 0) return
    setBusy(true)
    try {
      await onCreate(key, value)
    } finally {
      setBusy(false)
    }
  }, [keyDraft, keyMode, valueDraft, valueMode, onCreate])

  return (
    <div className="border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3">
      <div className="flex items-center justify-between">
        <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuPlus className="size-3.5" />
          New Key
        </h3>
        <Button
          variant="ghost"
          size="icon"
          onClick={onCancel}
          disabled={busy}
          aria-label="Cancel new key"
          className="size-6"
        >
          <LuX className="size-3.5" />
        </Button>
      </div>
      <KvValueEditor
        label="Key"
        ariaLabel="New key"
        draft={keyDraft}
        onDraftChange={setKeyDraft}
        mode={keyMode}
        onModeChange={setKeyMode}
        parseError={keyError}
        disabled={busy}
        rows={2}
      />
      <KvValueEditor
        label="Value"
        ariaLabel="New key value"
        draft={valueDraft}
        onDraftChange={setValueDraft}
        mode={valueMode}
        onModeChange={setValueMode}
        parseError={valueError}
        disabled={busy}
        rows={6}
      />
      {mutationError ? (
        <ErrorState
          variant="inline"
          message={`Create failed: ${mutationError}`}
        />
      ) : null}
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => void handleCreate()}
          disabled={!canCreate}
          className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 h-7 gap-1 text-xs"
        >
          <LuPlus className="size-3.5" />
          Create
        </Button>
      </div>
    </div>
  )
}

function parseDraftError(draft: string, mode: KvDisplayMode): string | null {
  if (draft.length === 0) return null
  try {
    parseValue(draft, mode)
    return null
  } catch (err) {
    return err instanceof Error ? err.message : String(err)
  }
}
