import { useEffect, useReducer, type ReactNode } from 'react'
import { LuFileText, LuLoader, LuPencil, LuSave, LuX } from 'react-icons/lu'
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
import { cn } from '@s4wave/web/style/utils.js'
import {
  ViewerBar,
  type ViewerBarAction,
} from '@s4wave/web/object/ViewerBar.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'

import {
  deriveTextFileView,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
  type TextFileEvent,
  type TextFilePress,
  type TextFileView,
} from './text-file.js'

export interface UnixFSTextFileViewerProps {
  rootHandle: Resource<FSHandle>
  path: string
  // readOnly says why the file cannot be written, such as a commit's files;
  // Edit then never shows.
  readOnly?: string
  // description is one muted line of what the file is, for the viewer's own
  // bar.
  description?: string
  // renderBar draws the viewer's presses in the bar that owns the pane, such as the
  // file browser's navigator. Without it the viewer draws its own ViewerBar.
  renderBar?: (presses: ReactNode) => ReactNode
}

// rowClass is a notice row under the bar, padded as the bar is.
const rowClass =
  'viewer-inset-x min-h-viewer-fact-row flex shrink-0 items-center gap-2 border-b border-foreground/8 py-1 text-xs'

// pressClass draws a press in a bar that is not a ViewerBar. Below a 28rem
// pane the press is its icon alone, as a ViewerBar press folds.
const pressClass =
  'focus-ring h-viewer-press w-viewer-press inline-flex shrink-0 items-center justify-center gap-1 rounded px-0 text-xs whitespace-nowrap select-none transition-colors aria-disabled:pointer-events-none aria-disabled:opacity-50 [&_svg]:size-3.5 pointer-coarse:min-h-11 pointer-coarse:min-w-11 @pane-md:w-auto @pane-md:px-2 @pane-md:pointer-coarse:min-w-0'

/** errorText is a failure's message without the Error prefix. */
function errorText(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause)
}

/** baseName returns the last path segment. */
function baseName(path: string): string {
  return path.split('/').filter(Boolean).pop() ?? path
}

/** TextFilePresses draws the viewer's presses inside another component's bar. */
function TextFilePresses({ actions }: { actions: ViewerBarAction[] }) {
  if (actions.length === 0) return null
  return (
    <div className="flex items-center gap-1">
      {actions.map((a) => (
        <button
          key={a.id}
          type="button"
          aria-disabled={a.disabled || undefined}
          aria-label={a.label}
          title={a.label}
          onClick={a.disabled ? undefined : a.onClick}
          className={cn(
            pressClass,
            a.variant === 'primary'
              ? 'border-brand/30 bg-brand/10 text-foreground hover:border-brand/50 hover:bg-brand/15 border'
              : 'text-foreground-alt hover:bg-foreground/5 hover:text-foreground',
          )}
        >
          <span className="contents" aria-hidden>
            {a.icon}
          </span>
          <span className="@pane-md:inline hidden">{a.label}</span>
        </button>
      ))}
    </div>
  )
}

/**
 * UnixFSTextFileViewer shows a text file and edits it in a draft saved
 * through one atomic World upload. text-file.ts derives what it shows; the
 * draft lives in this viewer, keyed by the Space and path, and only Discard
 * edits or a landed save end it.
 */
export function UnixFSTextFileViewer({
  rootHandle,
  path,
  readOnly,
  description,
  renderBar,
}: UnixFSTextFileViewerProps) {
  // Read the file whole, or up to the limit as a preview.
  const file = useUnixFSHandle(rootHandle, path)
  const content = useResource(
    file,
    async (handle, signal): Promise<TextContent | null> => {
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
  const [state, dispatch] = useReducer(reduceTextFile, initialTextFileState)
  const signal = useAbortSignal([path])

  // Each landed read is the version a draft compares against, kept through a
  // later failed read.
  useEffect(() => {
    if (content.value) dispatch({ type: 'read', content: content.value })
  }, [content.value])

  // Derive what the viewer draws from the read and the draft.
  const view = deriveTextFileView(content, state, !readOnly)

  // Submit only on a press; reconnecting a Resource must not replay a save.
  async function save(over: boolean) {
    // Take the draft's text once; typing after the press is not saved.
    const root = rootHandle.value
    const draft = state.draft
    if (!root || !draft || draft.saving || signal.aborted) return
    const text = draft.text
    dispatch({ type: 'save', over })
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
      if (!signal.aborted) dispatch({ type: 'saved', text })
    } catch (cause) {
      if (!signal.aborted) {
        dispatch({ type: 'save-failed', error: errorText(cause) })
      }
    }
  }

  return (
    <TextFileFrame
      view={view}
      path={path}
      readOnly={readOnly}
      description={description}
      renderBar={renderBar}
      dispatch={dispatch}
      onSave={(over) => void save(over)}
      retryRead={content.retry}
    />
  )
}

export interface TextFileFrameProps {
  view: TextFileView
  path: string
  readOnly?: string
  description?: string
  renderBar?: (presses: ReactNode) => ReactNode
  dispatch: (event: TextFileEvent) => void
  onSave: (over: boolean) => void
  retryRead: () => void
}

/**
 * TextFileFrame draws a derived text file view: the presses in the bar and
 * the body under it. The viewer and its debug fixtures both draw through it.
 */
export function TextFileFrame({
  view,
  path,
  readOnly,
  description,
  renderBar,
  dispatch,
  onSave,
  retryRead,
}: TextFileFrameProps) {
  // Map each press the view offers to its bar action.
  const dirty = view.state === 'editing' && view.draft.text !== view.draft.base
  const pressAction: Record<TextFilePress, ViewerBarAction | null> = {
    edit: {
      id: 'edit',
      label: 'Edit',
      icon: <LuPencil />,
      variant: 'primary',
      onClick: () =>
        view.state === 'shown' && dispatch({ type: 'edit', text: view.text }),
    },
    save: {
      id: 'save',
      label: 'Save',
      icon: <LuSave />,
      variant: 'primary',
      onClick: () => onSave(false),
    },
    saveover: {
      id: 'save',
      label: 'Save over theirs',
      icon: <LuSave />,
      variant: 'primary',
      onClick: () => onSave(true),
    },
    saving: {
      id: 'save',
      label: 'Saving',
      icon: <LuLoader className="motion-safe:animate-spin" />,
      variant: 'primary',
      disabled: true,
      onClick: () => {},
    },
    discard: {
      id: 'discard',
      label: dirty ? 'Discard edits' : 'Done',
      icon: <LuX />,
      onClick: () => dispatch({ type: 'discard' }),
    },
    // Show theirs and Back to my edits sit in the changed notice.
    theirs: null,
    mine: null,
  }
  const presses =
    view.state === 'shown' || view.state === 'editing' ? view.presses : []
  const actions = presses
    .map((p) => pressAction[p])
    .filter((a): a is ViewerBarAction => a !== null)

  // Pick the bar's line: the dirty mark, else why it is read-only, else what
  // the file is.
  let note = description
  if (readOnly) note = readOnly
  if (dirty) note = 'Edited, not saved'
  const head = renderBar ? (
    renderBar(<TextFilePresses actions={actions} />)
  ) : (
    <ViewerBar
      icon={<LuFileText />}
      title={baseName(path)}
      description={note}
      actions={actions}
    />
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {head}
      <TextFileBody
        view={view}
        readOnly={readOnly}
        retryRead={retryRead}
        onType={(text) => dispatch({ type: 'type', text })}
        onSave={onSave}
        onTheirs={(show) => dispatch({ type: 'theirs', show })}
      />
    </div>
  )
}

interface TextFileBodyProps {
  view: TextFileView
  readOnly?: string
  retryRead: () => void
  onType: (text: string) => void
  onSave: (over: boolean) => void
  onTheirs: (show: boolean) => void
}

const textClass =
  'text-foreground viewer-inset min-h-0 min-w-0 flex-1 overflow-auto font-mono text-xs whitespace-pre-wrap max-sm:text-sm max-sm:leading-6 max-sm:break-words'

/** TextFileBody draws the file, or its draft with the notices above it. */
function TextFileBody({ view, ...props }: TextFileBodyProps) {
  if (view.state === 'editing') return <DraftBody view={view} {...props} />
  return <ReadBody view={view} {...props} />
}

/** ReadBody draws the file as the read brings it, with no draft open. */
function ReadBody({
  view,
  readOnly,
  retryRead,
}: Omit<TextFileBodyProps, 'view'> & {
  view: Exclude<TextFileView, { state: 'editing' }>
}) {
  if (view.state === 'waiting') {
    return <ViewerStatus state="waiting" what="the file" rows={0} />
  }
  if (view.state === 'failed') {
    return (
      <ViewerStatus
        state="failed"
        message={`Could not read the file: ${view.error.message}`}
        retry={retryRead}
      />
    )
  }
  if (view.complete && view.text === '') {
    return (
      <ViewerStatus
        state="empty"
        icon={<LuFileText />}
        message={
          readOnly ? 'Empty file.' : 'Empty file. Press Edit to write in it.'
        }
      />
    )
  }
  return (
    <>
      {!view.complete && (
        <p className={cn(rowClass, 'text-foreground-alt')}>
          This preview is too large to edit here.
        </p>
      )}
      {view.complete && view.rereading && (
        <p role="status" className={cn(rowClass, 'text-foreground-alt')}>
          Reading the file again
        </p>
      )}
      <pre className={textClass}>{view.text}</pre>
    </>
  )
}

/**
 * DraftBody draws an open draft. The draft stays in its textarea through every
 * read and save; notices sit above it so the element and its caret never move
 * to another node. The column scrolls so the textarea keeps a usable height
 * under the notices in a short pane.
 */
function DraftBody({
  view,
  retryRead,
  onSave,
  onTheirs,
  onType,
}: Omit<TextFileBodyProps, 'view'> & {
  view: Extract<TextFileView, { state: 'editing' }>
}) {
  const { draft, changed, readError } = view
  const over = changed !== null
  const showTheirs = draft.theirs && over
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      {readError && (
        <ViewerStatus
          state="failed"
          message={`Could not read the file: ${readError.message}. Your edits are still here.`}
          retry={retryRead}
        />
      )}
      {draft.error && !draft.saving && (
        <ViewerStatus
          state="failed"
          message={`Not saved: ${draft.error}. Your edits are still here.`}
          retry={() => onSave(over)}
          retryLabel={over ? 'Save over theirs' : 'Retry'}
        />
      )}
      {over && (
        <div
          role="alert"
          className={cn(rowClass, 'bg-warning/8 text-foreground')}
        >
          <span className="max-w-prose min-w-0 flex-1 leading-snug">
            Another writer saved this file after you began editing.
          </span>
          <Button
            variant="outline"
            size="xs"
            className="shrink-0"
            disabled={!!draft.saving}
            onClick={() => onTheirs(!draft.theirs)}
          >
            {draft.theirs ? 'Back to my edits' : 'Show theirs'}
          </Button>
        </div>
      )}
      {showTheirs && (
        <pre aria-label="Their version" className={textClass}>
          {changed}
        </pre>
      )}
      <textarea
        aria-label="File contents"
        hidden={showTheirs}
        value={draft.text}
        readOnly={!!draft.saving}
        onChange={(event) => onType(event.target.value)}
        spellCheck={false}
        className="bg-background text-foreground viewer-inset min-h-11 min-w-0 flex-1 resize-none font-mono text-xs focus-visible:outline-none pointer-coarse:text-base pointer-coarse:leading-6"
      />
    </div>
  )
}
