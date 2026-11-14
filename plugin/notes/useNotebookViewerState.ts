import { useCallback, useEffect, useRef, useState } from 'react'

import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'

import type { NotebookSource } from './proto/notebook.pb.js'
import type { NotebookHandle } from './sdk/notebook.js'
import { NotebookEditOutcome } from './sdk/notebook.pb.js'
import type { SavedView } from './saved-views.js'

// SourceEdit is one edit of the Notebook's sources, holding what the person
// saw when they asked for it, so a retry resends the same request.
export type SourceEdit =
  | { kind: 'add'; source: NotebookSource }
  | { kind: 'remove'; index: number; source: NotebookSource }
  | { kind: 'reorder'; order: number[]; seen: NotebookSource[] }

// SourceEditState is the source edit control's state. One edit runs at a
// time: idle offers every edit; sending disables every control that starts
// an edit and keeps the sending edit's dialog open to show its result;
// changed means the edit was refused because another writer changed the
// sources first, and the watch already shows their list; failed holds the
// request and what failed, and offers the same request again. changed and failed last until the next edit
// starts, or until the person closes the dialog that shows them.
export type SourceEditState =
  | { status: 'idle' }
  | { status: 'sending'; edit: SourceEdit }
  | { status: 'changed'; edit: SourceEdit }
  | { status: 'failed'; edit: SourceEdit; error: string }

type SourceEditEvent =
  | { type: 'send'; edit: SourceEdit }
  | { type: 'applied' }
  | { type: 'changed' }
  | { type: 'failed'; error: string }
  | { type: 'dismiss' }

function sourceEditReducer(
  state: SourceEditState,
  event: SourceEditEvent,
): SourceEditState {
  switch (event.type) {
    case 'send':
      // A second edit while one sends is refused, so one edit's result is
      // never shown as another's.
      return state.status === 'sending'
        ? state
        : { status: 'sending', edit: event.edit }
    case 'applied':
      return state.status === 'sending' ? { status: 'idle' } : state
    case 'changed':
      return state.status === 'sending'
        ? { status: 'changed', edit: state.edit }
        : state
    case 'failed':
      return state.status === 'sending'
        ? { status: 'failed', edit: state.edit, error: event.error }
        : state
    case 'dismiss':
      return state.status === 'sending' ? state : { status: 'idle' }
  }
}

// RemoveTarget is the source the person asked to remove, with the index it
// had on their screen.
export interface RemoveTarget {
  index: number
  source: NotebookSource
}

interface UseNotebookViewerStateOptions {
  sources: NotebookSource[]
  notebookHandle: NotebookHandle | null | undefined
}

// useNotebookViewerState owns notebook selection, filtering, responsive
// navigation, and source mutation transitions for the viewer.
export function useNotebookViewerState({
  sources,
  notebookHandle,
}: UseNotebookViewerStateOptions) {
  const ns = useStateNamespace(['notes'])

  // Persisted state for selected source and note.
  const [selectedSource, setSelectedSource] = useStateAtom<number>(
    ns,
    'selectedSource',
    0,
  )
  const [selectedNote, setSelectedNote] = useStateAtom<string>(
    ns,
    'selectedNote',
    '',
  )
  const [currentPath, setCurrentPath] = useStateAtom<string>(
    ns,
    'currentPath',
    '',
  )
  const [editing, setEditing] = useStateAtom<boolean>(ns, 'editing', false)

  // Tag filter state.
  const [filterTag, setFilterTag] = useState<string | undefined>(undefined)
  const [filterStatus, setFilterStatus] = useState<string | undefined>(
    undefined,
  )

  const [sort, setSort] = useState<SavedView['sort']>('name')

  // Responsive sidebar visibility.
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [addSourceOpen, setAddSourceOpen] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<RemoveTarget | null>(null)
  // The edit state lives in a ref as well as in React state, so a send
  // refused by the reducer is known at once, before the next render.
  const sourceEditRef = useRef<SourceEditState>({ status: 'idle' })
  const [sourceEdit, setSourceEdit] = useState(sourceEditRef.current)
  const dispatchSourceEdit = useCallback((event: SourceEditEvent) => {
    const next = sourceEditReducer(sourceEditRef.current, event)
    sourceEditRef.current = next
    setSourceEdit(next)
    return next
  }, [])
  const editSending = sourceEdit.status === 'sending'

  // addedSource is the source an applied Add made, selected once the watch
  // shows it, so a concurrent edit by another writer cannot shift which
  // source gets selected. It changes nothing shown until then, so it is a
  // ref, and the selection it waits for is written to the persisted atom.
  const addedSource = useRef<NotebookSource | null>(null)

  const currentSource = sources[selectedSource]

  const handleSelectSource = useCallback(
    (index: number) => {
      setSelectedSource(index)
      setCurrentPath('')
      setSelectedNote('')
      setEditing(false)
      setFilterTag(undefined)
      setFilterStatus(undefined)
      setSidebarOpen(false)
    },
    [setSelectedSource, setCurrentPath, setSelectedNote, setEditing],
  )

  /** handleLoadView applies a shared definition only on this viewer's explicit action. */
  const handleLoadView = useCallback(
    (view: SavedView): string | null => {
      // A deleted source cannot redirect the user to an unrelated source index.
      const index = sources.findIndex((source) => source.ref === view.sourceRef)
      if (index < 0)
        return 'This view’s source was removed. Choose a current source and save the view again.'

      // Keep the loaded configuration in the existing personal and draft state owners.
      setSelectedSource(index)
      setCurrentPath(view.path)
      setSelectedNote('')
      setEditing(false)
      setFilterTag(view.filterTag ?? undefined)
      setFilterStatus(view.filterStatus ?? undefined)
      setSort(view.sort)
      setSidebarOpen(false)
      return null
    },
    [sources, setSelectedSource, setCurrentPath, setSelectedNote, setEditing],
  )

  // sendSourceEdit applies one edit and moves the edit state by its result.
  // It returns busy, sending nothing, when another edit is still sending.
  const sendSourceEdit = useCallback(
    async (edit: SourceEdit): Promise<SourceEditState['status'] | 'busy'> => {
      const sent = dispatchSourceEdit({ type: 'send', edit })
      if (sent.status !== 'sending' || sent.edit !== edit) return 'busy'
      if (!notebookHandle) {
        dispatchSourceEdit({
          type: 'failed',
          error: 'The notebook is still opening.',
        })
        return 'failed'
      }
      let outcome: NotebookEditOutcome
      try {
        outcome = await applySourceEdit(notebookHandle, edit)
      } catch (err) {
        const error = err instanceof Error ? err.message : String(err)
        dispatchSourceEdit({ type: 'failed', error })
        return 'failed'
      }
      if (outcome === NotebookEditOutcome.CHANGED) {
        dispatchSourceEdit({ type: 'changed' })
        return 'changed'
      }
      dispatchSourceEdit({ type: 'applied' })
      return 'idle'
    },
    [notebookHandle, dispatchSourceEdit],
  )

  const handleAddSource = useCallback(() => {
    if (dispatchSourceEdit({ type: 'dismiss' }).status === 'sending') return
    setAddSourceOpen(true)
  }, [dispatchSourceEdit])

  // handleAddSourceOpenChange closes the Add dialog and drops a failure it
  // showed. It refuses while an edit sends, so the dialog stays to show the
  // result.
  const handleAddSourceOpenChange = useCallback(
    (open: boolean) => {
      if (open) return
      if (dispatchSourceEdit({ type: 'dismiss' }).status === 'sending') return
      setAddSourceOpen(false)
    },
    [dispatchSourceEdit],
  )

  // Select the added source once the watch shows it. The last source with
  // its name and ref is the one just added.
  useEffect(() => {
    const added = addedSource.current
    if (!added) return
    const index = sources.findLastIndex(
      (source) => source.ref === added.ref && source.name === added.name,
    )
    if (index < 0) return
    addedSource.current = null
    setSelectedSource(index)
  }, [sources, setSelectedSource])

  const handleConfirmAddSource = useCallback(
    async ({ name, ref }: { name: string; ref: string }) => {
      if (!ref) return
      const source = { name, ref }
      // Wait for the source before the send, since the watch may show it
      // before the send returns.
      addedSource.current = source
      const result = await sendSourceEdit({ kind: 'add', source })
      if (result !== 'idle') {
        if (addedSource.current === source) addedSource.current = null
        return
      }
      setAddSourceOpen(false)
      setCurrentPath('')
      setSelectedNote('')
      setEditing(false)
      setFilterTag(undefined)
      setFilterStatus(undefined)
    },
    [sendSourceEdit, setCurrentPath, setSelectedNote, setEditing],
  )

  // handleRemoveSource holds the source the person clicked with its index,
  // so the confirm removes that source even if the list changes meanwhile.
  const handleRemoveSource = useCallback(
    (index: number) => {
      const source = sources[index]
      if (!source) return
      if (dispatchSourceEdit({ type: 'dismiss' }).status === 'sending') return
      setRemoveTarget({ index, source })
    },
    [sources, dispatchSourceEdit],
  )

  // handleRemoveSourceOpenChange closes the Remove dialog and drops a failure
  // it showed. It refuses while an edit sends, so the dialog stays to show
  // the result.
  const handleRemoveSourceOpenChange = useCallback(
    (open: boolean) => {
      if (open) return
      if (dispatchSourceEdit({ type: 'dismiss' }).status === 'sending') return
      setRemoveTarget(null)
    },
    [dispatchSourceEdit],
  )

  const handleConfirmRemoveSource = useCallback(async () => {
    if (!removeTarget) return
    const { index, source } = removeTarget
    // A failed Remove keeps its dialog open, so its confirm tries again.
    const result = await sendSourceEdit({ kind: 'remove', index, source })
    if (result === 'failed' || result === 'busy') return
    setRemoveTarget(null)
    if (result !== 'idle') return
    setSelectedSource((prev) => {
      if (prev > index) return prev - 1
      if (prev === index) return Math.max(0, prev - 1)
      return prev
    })
    setCurrentPath('')
    setSelectedNote('')
    setEditing(false)
    setFilterTag(undefined)
    setFilterStatus(undefined)
  }, [
    removeTarget,
    sendSourceEdit,
    setSelectedSource,
    setCurrentPath,
    setSelectedNote,
    setEditing,
  ])

  // reorder applies an order and keeps the selected source selected.
  const reorder = useCallback(
    async (edit: Extract<SourceEdit, { kind: 'reorder' }>) => {
      if ((await sendSourceEdit(edit)) !== 'idle') return
      setSelectedSource((prev) => Math.max(0, edit.order.indexOf(prev)))
    },
    [sendSourceEdit, setSelectedSource],
  )

  const handleMoveSource = useCallback(
    async (index: number, delta: -1 | 1) => {
      const nextIndex = index + delta
      if (nextIndex < 0 || nextIndex >= sources.length) return

      const order = sources.map((_, idx) => idx)
      ;[order[index], order[nextIndex]] = [order[nextIndex], order[index]]
      await reorder({ kind: 'reorder', order, seen: sources })
    },
    [reorder, sources],
  )

  // handleRetryReorder resends a failed Reorder as the person first asked it;
  // Add and Remove retry from their dialog's confirm.
  const handleRetryReorder = useCallback(() => {
    if (sourceEdit.status !== 'failed' || sourceEdit.edit.kind !== 'reorder')
      return
    void reorder(sourceEdit.edit)
  }, [sourceEdit, reorder])

  const handleSelectNote = useCallback(
    (path: string) => {
      setCurrentPath(getParentPath(path))
      setSelectedNote(path)
      setEditing(false)
      setSidebarOpen(false)
    },
    [setCurrentPath, setSelectedNote, setEditing],
  )

  const handleChangePath = useCallback(
    (path: string) => {
      setCurrentPath(path)
      setSelectedNote('')
      setEditing(false)
    },
    [setCurrentPath, setSelectedNote, setEditing],
  )

  const handleNoteRenamed = useCallback(
    (prevPath: string, nextPath: string) => {
      if (selectedNote === prevPath) {
        setSelectedNote(nextPath)
      }
    },
    [selectedNote, setSelectedNote],
  )

  const handleNoteDeleted = useCallback(
    (path: string) => {
      if (selectedNote !== path) return
      setSelectedNote('')
      setEditing(false)
    },
    [selectedNote, setSelectedNote, setEditing],
  )

  const handleToggleEdit = useCallback(() => {
    setEditing((prev) => !prev)
  }, [setEditing])

  return {
    ns,
    selectedSource,
    selectedNote,
    currentPath,
    editing,
    filterTag,
    filterStatus,
    sort,
    setSort,
    handleLoadView,
    sidebarOpen,
    addSourceOpen,
    removeTarget,
    sourceEdit,
    editSending,
    currentSource,
    setFilterTag,
    setFilterStatus,
    setSidebarOpen,
    handleSelectSource,
    handleAddSource,
    handleAddSourceOpenChange,
    handleConfirmAddSource,
    handleRemoveSource,
    handleRemoveSourceOpenChange,
    handleConfirmRemoveSource,
    handleMoveSource,
    handleRetryReorder,
    handleSelectNote,
    handleChangePath,
    handleNoteRenamed,
    handleNoteDeleted,
    handleToggleEdit,
  }
}

function getParentPath(path: string): string {
  const parts = path.split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
}

// applySourceEdit sends one source edit; Add has no list guard, so it is
// either applied or throws.
async function applySourceEdit(
  handle: NotebookHandle,
  edit: SourceEdit,
): Promise<NotebookEditOutcome> {
  switch (edit.kind) {
    case 'add':
      await handle.addSource(edit.source)
      return NotebookEditOutcome.APPLIED
    case 'remove':
      return handle.removeSource(edit.index, edit.source)
    case 'reorder':
      return handle.reorderSources(edit.order, edit.seen)
  }
}
