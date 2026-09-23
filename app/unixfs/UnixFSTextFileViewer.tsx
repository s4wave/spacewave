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
      if (!signal.aborted) setSaving(false)
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
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-foreground/10 flex items-center gap-2 border-b p-2">
        {draft === null ? (
          <Button
            size="sm"
            variant="ghost"
            disabled={!content.value.complete}
            onClick={() => setDraft(text)}
          >
            Edit file
          </Button>
        ) : (
          <>
            <Button
              size="sm"
              disabled={saving || !rootHandle.value || draft === text}
              onClick={() => void save()}
            >
              {saving ? 'Saving…' : 'Save file'}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={saving}
              onClick={() => {
                setDraft(null)
                setError('')
              }}
            >
              {draft === text ? 'Done editing' : 'Cancel edits'}
            </Button>
          </>
        )}
        {!content.value.complete && (
          <span className="text-xs">
            This preview is too large to edit here.
          </span>
        )}
        {acceptedText !== null &&
          !saving &&
          !error &&
          acceptedText === draft && (
            <span role="status" className="text-xs">
              File saved
            </span>
          )}
      </div>

      {error && (
        <div role="alert" className="text-destructive p-2 text-sm">
          <p>{error}</p>
          <Button
            size="sm"
            variant="ghost"
            disabled={saving}
            onClick={() => void save()}
          >
            Retry save
          </Button>
        </div>
      )}
      {draft === null ? (
        <pre className="text-foreground min-h-0 flex-1 overflow-auto p-4 font-mono text-xs whitespace-pre-wrap">
          {text}
        </pre>
      ) : (
        <textarea
          aria-label="File contents"
          value={draft}
          disabled={saving}
          onChange={(event) => setDraft(event.target.value)}
          spellCheck={false}
          className="bg-background text-foreground min-h-64 flex-1 resize-none p-4 font-mono text-xs focus-visible:outline-none"
        />
      )}
    </div>
  )
}
