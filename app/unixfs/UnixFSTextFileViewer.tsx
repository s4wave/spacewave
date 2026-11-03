import { useState } from 'react'
import { useAbortSignal } from '@aptre/bldr-react'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { normalizeUnixFSLookupPath } from '@s4wave/sdk/unixfs/path.js'
import {
  useUnixFSHandle,
  unixFSHandleTextContentMaxBytes,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: { display: 'flex', minHeight: 0, flex: 1, flexDirection: 'column' },
  rule4: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule5: {
    minHeight: 0,
    minWidth: 0,
    flex: 1,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media not all and (min-width: 40rem)': themeFonts['--text-sm'],
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media not all and (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    whiteSpace: 'pre-wrap',
    color: themeColors['--color-foreground'],
    '--tw-leading': {
      default: null,
      '@media not all and (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    overflowWrap: {
      default: null,
      '@media not all and (min-width: 40rem)': 'break-word',
    },
  },
  rule6: {
    minHeight: 0,
    minWidth: 0,
    flex: 1,
    resize: 'none',
    backgroundColor: themeColors['--color-background'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: {
      default: themeFonts['--text-xs'],
      '@media (pointer: coarse)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    color: themeColors['--color-foreground'],
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    '--tw-leading': {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 6)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

interface TextFileToolbarProps {
  draft: string | null
  text: string
  complete: boolean
  saved: boolean
  saving: boolean
  canSave: boolean
  onEdit: () => void
  onSave: () => void
  onCancel: () => void
}

/** TextFileToolbar renders the edit, save, and cancel controls for a text file. */
function TextFileToolbar({
  draft,
  text,
  complete,
  saved,
  saving,
  canSave,
  onEdit,
  onSave,
  onCancel,
}: TextFileToolbarProps) {
  return (
    <div sx={stylexStyles2.rule1}>
      {draft === null ? (
        <Button
          size="default"
          variant="ghost"
          sx={stylexStyles.rule1}
          disabled={!complete}
          onClick={onEdit}
        >
          Edit file
        </Button>
      ) : (
        <>
          <Button
            size="default"
            sx={stylexStyles.rule1}
            disabled={saving || !canSave || draft === text}
            onClick={onSave}
          >
            {saving ? 'Saving…' : 'Save file'}
          </Button>
          <Button
            size="default"
            variant="ghost"
            sx={stylexStyles.rule1}
            disabled={saving}
            onClick={onCancel}
          >
            {draft === text ? 'Done editing' : 'Cancel edits'}
          </Button>
        </>
      )}
      {!complete && (
        <span sx={stylexStyles2.rule2}>
          This preview is too large to edit here.
        </span>
      )}
      {saved && !saving && (
        <span role="status" sx={stylexStyles2.rule2}>
          File saved
        </span>
      )}
    </div>
  )
}

/** UnixFSTextFileViewer edits complete UTF-8 files through atomic World uploads. */
export function UnixFSTextFileViewer({
  rootHandle,
  path,
}: {
  rootHandle: Resource<FSHandle>
  path: string
}) {
  // Drafts belong to this file view; Resource hooks own the read handles.
  const file = useUnixFSHandle(rootHandle, path)
  const content = useResource(
    file,
    async (handle, signal) => {
      if (!handle) return null
      const result = await handle.readAt(
        0n,
        BigInt(unixFSHandleTextContentMaxBytes + 1),
        signal,
      )
      const complete =
        result.eof && result.data.length <= unixFSHandleTextContentMaxBytes
      return {
        text: new TextDecoder('utf-8', { fatal: complete }).decode(result.data),
        complete,
      }
    },
    [],
  )
  const [draft, setDraft] = useState<string | null>(null)
  const [acceptedText, setAcceptedText] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const signal = useAbortSignal([path])

  // Submit only on an explicit save; reconnecting a Resource must not replay edits.
  async function save() {
    const root = rootHandle.value
    if (!root || draft === null || saving || signal.aborted) return
    const text = draft
    setSaving(true)
    setError('')
    try {
      const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
      await root.uploadTree(
        [
          {
            kind: 'file',
            path: normalizeUnixFSLookupPath(path),
            totalSize: BigInt(blob.size),
            stream: blob.stream(),
            mode: file.value?.getInfo().mode ?? 0o644,
          },
        ],
        undefined,
        signal,
      )
      if (!signal.aborted) setAcceptedText(text)
    } catch (cause) {
      if (!signal.aborted) setError(String(cause))
    } finally {
      setSaving(false)
    }
  }

  // Never turn a truncated preview into a replacement for the complete file.
  if (content.loading || content.error) {
    return (
      <LoadingCard
        view={
          content.error
            ? {
                state: 'error',
                title: 'Unable to read text file',
                error: content.error.message,
                onRetry: content.retry,
              }
            : { state: 'active', title: 'Loading file' }
        }
      />
    )
  }
  if (!content.value) return null
  const text = acceptedText ?? content.value.text

  // Saving replaces one file in one accepted transaction, preserving sibling files.
  return (
    <div sx={stylexStyles2.rule3}>
      <TextFileToolbar
        draft={draft}
        text={text}
        complete={content.value.complete}
        saved={acceptedText !== null && acceptedText === draft && !error}
        saving={saving}
        canSave={!!rootHandle.value}
        onEdit={() => setDraft(text)}
        onSave={() => void save()}
        onCancel={() => {
          setDraft(null)
          setError('')
        }}
      />

      {error && (
        <div role="alert" sx={stylexStyles2.rule4}>
          <p>{error}</p>
          <Button
            size="default"
            variant="ghost"
            sx={stylexStyles.rule1}
            disabled={saving}
            onClick={() => void save()}
          >
            Retry save
          </Button>
        </div>
      )}
      {draft === null ? (
        <pre sx={stylexStyles2.rule5}>{text}</pre>
      ) : (
        <textarea
          aria-label="File contents"
          value={draft}
          disabled={saving}
          onChange={(event) => setDraft(event.target.value)}
          spellCheck={false}
          sx={stylexStyles2.rule6}
        />
      )}
    </div>
  )
}
