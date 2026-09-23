import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuMenu, LuX } from 'react-icons/lu'

import { Notebook } from './proto/notebook.pb.js'
import { NotebookHandle, NotebookTypeID } from './sdk/notebook.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'
import { ConfirmActionDialog, SourceInputDialog } from './NoteDialogs.js'

import NotebookSidebar from './NotebookSidebar.js'
import NoteList from './NoteList.js'
import NoteContentView from './NoteContentView.js'
import { useNotebookViewerState } from './useNotebookViewerState.js'
import NotebookSavedViews from './NotebookSavedViews.js'

// NotebookViewer is the three-panel viewer for Notes Notebook objects.
function NotebookViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

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

  const notebookHandle = resource.value
  const viewerState = useNotebookViewerState({ sources, notebookHandle })
  const {
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
    removeSourceIndex,
    currentSource,
    setFilterTag,
    setFilterStatus,
    setSidebarOpen,
    setAddSourceOpen,
    setRemoveSourceIndex,
    handleSelectSource,
    handleAddSource,
    handleConfirmAddSource,
    handleRemoveSource,
    handleConfirmRemoveSource,
    handleMoveSource,
    handleSelectNote,
    handleChangePath,
    handleNoteRenamed,
    handleNoteDeleted,
    handleToggleEdit,
  } = viewerState

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
          aria-label={
            sidebarOpen
              ? 'Close notebook navigation'
              : 'Open notebook navigation'
          }
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
            'border-border w-50 min-w-50 overflow-y-auto border-r',
            'md:relative md:block',
            sidebarOpen
              ? 'bg-background-primary absolute inset-y-0 left-0 z-20 block'
              : 'hidden',
          )}
        >
          <NotebookSavedViews
            world={worldState}
            notebook={objectKey}
            handle={notebookHandle}
            draft={{
              sourceRef: currentSource?.ref ?? '',
              path: currentPath,
              filterTag: filterTag ?? null,
              filterStatus: filterStatus ?? null,
              sort,
            }}
            onLoad={handleLoadView}
          />
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
            'border-border flex w-62.5 min-w-62.5 flex-col border-r',
            selectedNote ? 'hidden md:flex' : 'flex',
          )}
        >
          <label className="border-border flex items-center justify-between gap-2 border-b px-2 py-1 text-xs">
            Sort notes
            <select
              aria-label="Sort notes"
              value={sort}
              onChange={(event) => setSort(event.target.value as typeof sort)}
              className="bg-background-primary rounded p-1"
            >
              <option value="name">File name</option>
              <option value="title">Title</option>
            </select>
          </label>
          <div className="min-h-0 flex-1">
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
              sort={sort}
              onFilterTagChange={setFilterTag}
              onFilterStatusChange={setFilterStatus}
            />
          </div>
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

export { NotebookViewer }
export default NotebookViewer
