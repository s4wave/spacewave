import { useCallback, useState } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuMenu, LuX } from 'react-icons/lu'

import { Notebook } from './proto/notebook.pb.js'
import { NotebookHandle, NotebookTypeID } from './sdk/notebook.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'
import { ConfirmActionDialog, SourceInputDialog } from './NoteDialogs.js'

import NotebookSidebar from './NotebookSidebar.js'
import NoteList from './NoteList.js'
import NoteContentView from './NoteContentView.js'

// NotebookViewer is the three-panel viewer for Notes Notebook objects.
function NotebookViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['notes'])

  const resource = useAccessTypedHandle(
    worldState,
    objectKey,
    NotebookHandle,
    NotebookTypeID,
  )
  const { state, sources } = useWorldObjectMessageState(
    worldState,
    objectKey,
    Notebook.fromBinary,
  )

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

  // Responsive sidebar visibility.
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [addSourceOpen, setAddSourceOpen] = useState(false)
  const [removeSourceIndex, setRemoveSourceIndex] = useState<number | null>(
    null,
  )

  const currentSource = sources[selectedSource]
  const notebookHandle = resource.value

  const handleSelectSource = useCallback(
    (index: number) => {
      setSelectedSource(index)
      setCurrentPath('')
      setSelectedNote('')
      setEditing(false)
      setFilterTag(undefined)
      setFilterStatus(undefined)
    },
    [setSelectedSource, setCurrentPath, setSelectedNote, setEditing],
  )

  const handleAddSource = useCallback(() => {
    setAddSourceOpen(true)
  }, [])

  const handleConfirmAddSource = useCallback(
    async ({ name, ref }: { name: string; ref: string }) => {
      if (!notebookHandle) return
      if (!ref) return

      await notebookHandle.addSource({ name, ref })
      setAddSourceOpen(false)
      setSelectedSource(sources.length)
      setCurrentPath('')
      setSelectedNote('')
      setEditing(false)
      setFilterTag(undefined)
      setFilterStatus(undefined)
    },
    [
      notebookHandle,
      setSelectedSource,
      setCurrentPath,
      setSelectedNote,
      setEditing,
      sources.length,
    ],
  )

  const handleRemoveSource = useCallback((index: number) => {
    setRemoveSourceIndex(index)
  }, [])

  const handleConfirmRemoveSource = useCallback(async () => {
    if (!notebookHandle || removeSourceIndex === null) return

    const index = removeSourceIndex
    await notebookHandle.removeSource(index)
    setRemoveSourceIndex(null)
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
    notebookHandle,
    removeSourceIndex,
    setSelectedSource,
    setCurrentPath,
    setSelectedNote,
    setEditing,
  ])

  const handleMoveSource = useCallback(
    async (index: number, delta: -1 | 1) => {
      if (!notebookHandle) return
      const nextIndex = index + delta
      if (nextIndex < 0 || nextIndex >= sources.length) return

      const order = sources.map((_, idx) => idx)
      ;[order[index], order[nextIndex]] = [order[nextIndex], order[index]]
      await notebookHandle.reorderSources(order)
      setSelectedSource((prev) => {
        if (prev === index) return nextIndex
        if (prev === nextIndex) return index
        return prev
      })
    },
    [notebookHandle, sources, setSelectedSource],
  )

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

  return (
    <ViewerStatusShell
      resource={state}
      state={state}
      loadingText="Loading notebook..."
    >
      <div className="bg-background-primary flex h-full w-full overflow-hidden">
        {/* Mobile hamburger toggle */}
        <button
          type="button"
          className="bg-background-primary text-foreground-alt hover:text-foreground absolute top-2 left-2 z-30 rounded p-1 md:hidden"
          onClick={() => setSidebarOpen(!sidebarOpen)}
        >
          {sidebarOpen ? (
            <LuX className="size-5" />
          ) : (
            <LuMenu className="size-5" />
          )}
        </button>

        {/* Sidebar - responsive: hidden on mobile unless toggled */}
        <div
          className={cn(
            'border-border border-r',
            'md:relative md:block',
            sidebarOpen
              ? 'bg-background-primary absolute inset-y-0 left-0 z-20 block'
              : 'hidden',
          )}
          style={{ width: 200, minWidth: 200 }}
        >
          <NotebookSidebar
            sources={sources}
            selectedSource={selectedSource}
            onSelectSource={handleSelectSource}
            onAddSource={handleAddSource}
            onRemoveSource={handleRemoveSource}
            onMoveSource={(index, delta) => void handleMoveSource(index, delta)}
            namespace={ns}
          />
        </div>

        {/* Note list - responsive: hidden on mobile when note is selected */}
        <div
          className={cn(
            'border-border border-r',
            selectedNote ? 'hidden md:block' : 'block',
          )}
          style={{ width: 250, minWidth: 250 }}
        >
          <NoteList
            source={currentSource}
            worldState={worldState}
            selectedNote={selectedNote}
            currentPath={currentPath}
            onSelectNote={handleSelectNote}
            onChangePath={handleChangePath}
            onNoteRenamed={handleNoteRenamed}
            onNoteDeleted={handleNoteDeleted}
            filterTag={filterTag}
            filterStatus={filterStatus}
            onFilterTagChange={setFilterTag}
            onFilterStatusChange={setFilterStatus}
          />
        </div>

        {/* Content area */}
        <div className="min-w-0 flex-1">
          {currentSource?.ref && selectedNote ? (
            <NoteContentView
              worldState={worldState}
              sourceRef={currentSource.ref}
              noteName={selectedNote}
              editing={editing}
              onToggleEdit={handleToggleEdit}
              onFilterTag={setFilterTag}
              onFilterStatus={setFilterStatus}
            />
          ) : (
            <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
              {sources.length === 0
                ? 'No sources configured for this notebook'
                : 'Select a note to view'}
            </div>
          )}
        </div>

        {/* Backdrop for mobile sidebar */}
        {sidebarOpen && (
          <button
            type="button"
            aria-label="Close sidebar"
            className="fixed inset-0 z-10 bg-black/40 md:hidden"
            onClick={() => setSidebarOpen(false)}
          />
        )}
        <SourceInputDialog
          open={addSourceOpen}
          onOpenChange={setAddSourceOpen}
          onConfirm={(source) => void handleConfirmAddSource(source)}
        />
        <ConfirmActionDialog
          open={removeSourceIndex !== null}
          title="Remove source"
          description="Remove this source from the notebook?"
          confirmLabel="Remove"
          onOpenChange={(open) => {
            if (!open) setRemoveSourceIndex(null)
          }}
          onConfirm={() => void handleConfirmRemoveSource()}
        />
      </div>
    </ViewerStatusShell>
  )
}

function getParentPath(path: string): string {
  const parts = path.split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
}

export { NotebookViewer }
export default NotebookViewer
