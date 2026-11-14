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
import {
  resourceStatus,
  ViewerStatus,
} from '@s4wave/web/object/ViewerStatus.js'

interface TextFileToolbarProps {
  draft: string | null
  text: string
  complete: boolean
  reading: boolean
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
  reading,
  saved,
  saving,
  canSave,
  onEdit,
  onSave,
  onCancel,
}: TextFileToolbarProps) {
  // Never turn a truncated preview into a replacement for the complete file,
  // and never start a draft from text a re-read is about to replace.
  return (
    <div className="border-foreground/10 viewer-inset-x flex items-center gap-2 border-b py-2">
      {draft === null ? (
        <Button
          size="default"
          variant="ghost"
          className="pointer-coarse:min-h-11"
          disabled={!complete || reading}
          onClick={onEdit}
        >
          Edit file
        </Button>
      ) : (
        <>
          <Button
            size="default"
            className="pointer-coarse:min-h-11"
            disabled={saving || !canSave || draft === text}
            onClick={onSave}
          >
            {saving ? 'Saving…' : 'Save file'}
          </Button>
          <Button
            size="default"
            variant="ghost"
            className="pointer-coarse:min-h-11"
            disabled={saving}
            onClick={onCancel}
          >
            {draft === text ? 'Done editing' : 'Cancel edits'}
          </Button>
        </>
      )}
      {!complete && (
        <span className="text-xs">This preview is too large to edit here.</span>
      )}
      {complete && reading && draft === null && (
        <span className="text-xs">Reading the file again…</span>
      )}
      {saved && !saving && (
        <span role="status" className="text-xs">
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

  // A re-read keeps the old text drawn; the toolbar holds Edit until it lands.
  const status = resourceStatus('the file', content)
  if (status) return <ViewerStatus {...status} />
  if (!content.value) return null
  const text = acceptedText ?? content.value.text

  // Saving replaces one file in one accepted transaction, preserving sibling files.
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TextFileToolbar
        draft={draft}
        text={text}
        complete={content.value.complete}
        reading={content.loading}
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
        <div role="alert" className="text-destructive p-2 text-sm">
          <p>{error}</p>
          <Button
            size="default"
            variant="ghost"
            className="pointer-coarse:min-h-11"
            disabled={saving}
            onClick={() => void save()}
          >
            Retry save
          </Button>
        </div>
      )}
      {draft === null ? (
        <pre className="text-foreground viewer-inset min-h-0 min-w-0 flex-1 overflow-auto font-mono text-xs whitespace-pre-wrap max-sm:text-sm max-sm:leading-6 max-sm:break-words">
          {text}
        </pre>
      ) : (
        <textarea
          aria-label="File contents"
          value={draft}
          disabled={saving}
          onChange={(event) => setDraft(event.target.value)}
          spellCheck={false}
          className="bg-background text-foreground viewer-inset min-h-0 min-w-0 flex-1 resize-none font-mono text-xs focus-visible:outline-none pointer-coarse:text-base pointer-coarse:leading-6"
        />
      )}
    </div>
  )
}
