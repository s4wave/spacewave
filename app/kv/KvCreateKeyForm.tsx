import { useCallback, useMemo, useState } from 'react'
import { LuPlus, LuX } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'

import { KvValueEditor } from './KvValueEditor.js'
import { parseValue, type KvDisplayMode } from './kv-encoding.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

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
    <div sx={stylexStyles4.rule1}>
      <div sx={[nativeChildSpacing.s3, stylexStyles3.rule1]}>
        <h3 sx={stylexStyles3.rule2}>
          <LuPlus sx={stylexStyles2.rule1} />
          New Key
        </h3>
        <Button
          variant="ghost"
          size="icon"
          onClick={onCancel}
          disabled={busy}
          aria-label="Cancel new key"
          sx={stylexStyles.rule1}
        >
          <LuX sx={stylexStyles2.rule1} />
        </Button>
      </div>
      <KvValueEditor
        sx={nativeChildSpacing.s3}
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
        sx={nativeChildSpacing.s3}
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
          sx={nativeChildSpacing.s3}
          variant="inline"
          message={`Create failed: ${mutationError}`}
        />
      ) : null}
      <div sx={[nativeChildSpacing.s3, stylexStyles3.rule3]}>
        <Button
          onClick={() => void handleCreate()}
          disabled={!canCreate}
          variant="brandOutline"
          size="toolbar"
        >
          <LuPlus sx={stylexStyles2.rule1} />
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
