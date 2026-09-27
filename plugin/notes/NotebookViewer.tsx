import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuArrowLeft, LuChevronDown, LuMenu, LuX } from 'react-icons/lu'

import { Notebook } from './proto/notebook.pb.js'
import { NotebookHandle, NotebookTypeID } from './sdk/notebook.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'
import { ConfirmActionDialog, SourceInputDialog } from './NoteDialogs.js'

import NotebookSidebar from './NotebookSidebar.js'
import NoteList from './NoteList.js'
import NoteContentView from './NoteContentView.js'
import { useNotebookViewerState } from './useNotebookViewerState.js'
import NotebookSavedViews from './NotebookSavedViews.js'

/** NotebookViewer presents the notebook's source, note list, and active note. */
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
      <div className="bg-background-primary relative flex h-full min-h-0 w-full min-w-0 flex-col overflow-hidden md:pointer-fine:flex-row">
        <div className="border-border z-40 flex h-11 shrink-0 items-center border-b md:pointer-fine:hidden">
          <button
            type="button"
            aria-label={
              sidebarOpen
                ? 'Close notebook navigation'
                : 'Open notebook navigation'
            }
            aria-expanded={sidebarOpen}
            className="text-foreground-alt hover:text-foreground flex size-11 shrink-0 items-center justify-center"
            onClick={() => setSidebarOpen(!sidebarOpen)}
          >
            {sidebarOpen ? (
              <LuX className="size-5" />
            ) : (
              <LuMenu className="size-5" />
            )}
          </button>
          {selectedNote && (
            <button
              type="button"
              aria-label="Back to notes"
              className="text-foreground-alt hover:text-foreground flex size-11 shrink-0 items-center justify-center"
              onClick={() => handleChangePath(currentPath)}
            >
              <LuArrowLeft className="size-5" />
            </button>
          )}
          <span className="min-w-0 truncate px-2 text-sm font-medium">
            {currentSource?.name || 'Notebook'}
          </span>
        </div>

        {/* Touch navigation overlays the list so landscape retains room for the note. */}
        <div
          className={cn(
            'border-border w-72 max-w-full overflow-y-auto border-r md:pointer-fine:relative md:pointer-fine:block md:pointer-fine:w-50 md:pointer-fine:min-w-50',
            sidebarOpen
              ? 'bg-background-primary absolute top-11 bottom-0 left-0 z-30 block md:pointer-fine:inset-y-0'
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

        {/* Touch navigation shows either the list or the selected note. */}
        <div
          className={cn(
            'border-border min-h-0 min-w-0 flex-1 flex-col border-r md:pointer-fine:w-62.5 md:pointer-fine:min-w-62.5 md:pointer-fine:flex-none',
            selectedNote ? 'hidden md:pointer-fine:flex' : 'flex',
          )}
        >
          <label className="border-border flex min-h-11 items-center justify-between gap-2 border-b px-2 py-1 text-xs md:pointer-fine:min-h-0">
            Sort notes
            <span className="relative">
              <select
                aria-label="Sort notes"
                value={sort}
                onChange={(event) => setSort(event.target.value as typeof sort)}
                className="bg-background-primary h-11 appearance-none rounded py-1 pr-6 pl-1 md:pointer-fine:h-auto md:pointer-fine:appearance-auto md:pointer-fine:pr-1"
              >
                <option value="name">File name</option>
                <option value="title">Title</option>
              </select>
              <LuChevronDown
                aria-hidden="true"
                className="pointer-events-none absolute top-1/2 right-1 size-3 -translate-y-1/2 md:pointer-fine:hidden"
              />
            </span>
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
        <div
          className={cn(
            'min-h-0 min-w-0 flex-1',
            selectedNote ? 'block' : 'hidden md:pointer-fine:block',
          )}
        >
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
            className="absolute inset-0 z-20 bg-black/40 md:pointer-fine:hidden"
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
