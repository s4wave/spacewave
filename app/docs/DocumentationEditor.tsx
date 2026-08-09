import { useCallback, useEffect, useRef, useState } from 'react'
import Markdown from 'markdown-to-jsx'
import { LuPenLine, LuRefreshCw, LuX } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { docsMarkdownOverrides } from './markdown-overrides.js'
import { saveDocumentationPage } from './documentation-operations.js'

interface DocumentationEditorProps {
  page: string
  handle: Resource<FSHandle>
  text: Resource<string>
  editing: boolean
  setEditing: (editing: boolean) => void
}

// DocumentationEditor owns the draft and save lifecycle for one selected page.
export function DocumentationEditor({
  page,
  handle,
  text,
  editing,
  setEditing,
}: DocumentationEditorProps) {
  const [draft, setDraft] = useState<string | null>(() =>
    editing ? text.value : null,
  )
  const [saveState, setSaveState] = useState<{
    pending: boolean
    error: Error | null
  }>({ pending: false, error: null })
  const saving = saveState.pending
  const saveError = saveState.error
  const effectiveDraft = draft ?? text.value
  const operation = useRef(0)
  const abort = useRef<AbortController | null>(null)

  useEffect(
    () => () => {
      operation.current++
      abort.current?.abort()
    },
    [],
  )

  const beginEdit = useCallback(() => {
    setDraft(text.value ?? '')
    setSaveState({ pending: false, error: null })
    setEditing(true)
  }, [setEditing, text.value])

  const cancelEdit = useCallback(() => {
    operation.current++
    abort.current?.abort()
    abort.current = null
    setDraft(null)
    setSaveState({ pending: false, error: null })
    setEditing(false)
  }, [setEditing])

  const save = useCallback(async () => {
    if (saving || effectiveDraft === null || !handle.value) return
    const current = ++operation.current
    const controller = new AbortController()
    abort.current?.abort()
    abort.current = controller
    setSaveState({ pending: true, error: null })
    try {
      await saveDocumentationPage(
        handle.value,
        effectiveDraft,
        controller.signal,
      )
      if (operation.current !== current) return
      setDraft(null)
      setEditing(false)
    } catch (error) {
      if (operation.current !== current || controller.signal.aborted) return
      setSaveState({
        pending: false,
        error: error instanceof Error ? error : new Error(String(error)),
      })
    } finally {
      if (operation.current === current) {
        abort.current = null
        setSaveState((state) => ({ ...state, pending: false }))
      }
    }
  }, [effectiveDraft, handle.value, saving, setEditing])

  if (!page) {
    return (
      <div className="text-muted-foreground flex flex-1 items-center justify-center text-xs">
        Select a page to view
      </div>
    )
  }

  return (
    <>
      <div className="border-border flex items-center justify-between border-b px-3 py-1.5">
        <span className="text-xs font-medium">{page.replace(/\.md$/, '')}</span>
        <div className="flex items-center gap-1">
          {editing && (
            <button
              type="button"
              className="text-foreground-alt hover:bg-list-hover-background flex items-center gap-1 rounded px-2 py-0.5 text-xs"
              onClick={cancelEdit}
              title="Cancel editing"
            >
              <LuX className="size-3" />
              Cancel
            </button>
          )}
          <button
            type="button"
            className={cn(
              'flex items-center gap-1 rounded px-2 py-0.5 text-xs',
              'hover:bg-list-hover-background disabled:opacity-50',
              editing ? 'text-brand' : 'text-foreground-alt',
            )}
            onClick={editing ? () => void save() : beginEdit}
            disabled={saving || (editing && !handle.value)}
            title={editing ? 'Save and preview' : 'Edit page'}
          >
            <LuPenLine className="size-3" />
            {saving ? 'Saving…' : editing ? 'Save' : 'Edit'}
          </button>
        </div>
      </div>

      {text.loading ? (
        <div className="flex flex-1 items-center justify-center p-4">
          <LoadingInline label="Loading page" tone="muted" size="sm" />
        </div>
      ) : text.error ? (
        <div className="text-destructive flex flex-1 flex-col items-center justify-center gap-2 p-4 text-xs">
          <span>Failed to load page</span>
          <span className="text-foreground-alt/50 text-xs">
            {text.error.message}
          </span>
        </div>
      ) : editing ? (
        <div className="flex flex-1 flex-col overflow-auto">
          {saveError && (
            <div
              role="alert"
              className="border-destructive/30 bg-destructive/10 text-destructive flex items-center justify-between gap-3 border-b px-4 py-2 text-xs"
            >
              <span>Could not save: {saveError.message}</span>
              <button
                type="button"
                className="flex items-center gap-1 font-medium"
                onClick={() => void save()}
                disabled={saving}
              >
                <LuRefreshCw className="size-3" />
                Retry
              </button>
            </div>
          )}
          <textarea
            aria-label="Documentation page content"
            className="bg-background-primary text-editor-foreground min-h-0 w-full flex-1 resize-none border-none p-4 font-mono text-xs outline-none"
            value={effectiveDraft ?? ''}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
      ) : (
        <div className="flex-1 overflow-auto p-4">
          <div className="docs-prose">
            <Markdown options={docsMarkdownOverrides}>
              {text.value ?? ''}
            </Markdown>
          </div>
        </div>
      )}
    </>
  )
}
