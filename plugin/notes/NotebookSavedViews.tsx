import { useState, type ReactNode } from 'react'
import { LuChevronDown } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'

import type { NotebookHandle } from './sdk/notebook.js'
import type { SavedView } from './saved-views.js'
import { useNotebookSavedViews } from './useNotebookSavedViews.js'
import { ConfirmActionDialog, TextInputDialog } from './NoteDialogs.js'

interface NotebookSavedViewsProps {
  world: Resource<IWorldState>
  notebook: string
  handle: NotebookHandle | null
  draft: Pick<
    SavedView,
    'sourceRef' | 'path' | 'filterTag' | 'filterStatus' | 'sort'
  >
  onLoad(view: SavedView): string | null
}

type SavedViewDialog = 'save' | 'rename' | 'delete'

const actionClassName =
  'min-h-11 min-w-11 md:pointer-fine:min-h-0 md:pointer-fine:min-w-0'

// SavedViewButton renders one saved view action.
function SavedViewButton({
  disabled,
  onClick,
  children,
}: {
  disabled?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      className={actionClassName}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  )
}

// SavedViewSelect chooses the shared view that Load, Rename, and Delete act on.
function SavedViewSelect({
  currentId,
  views,
  loading,
  disabled,
  onSelect,
}: {
  currentId: string
  views: SavedView[]
  loading: boolean
  disabled: boolean
  onSelect: (id: string) => void
}) {
  return (
    <label className="flex flex-col gap-1">
      Saved view
      <span className="relative">
        <select
          value={currentId}
          disabled={disabled}
          onChange={(event) => onSelect(event.target.value)}
          className="border-border bg-background-primary h-11 w-full appearance-none rounded border py-1 pr-6 pl-1 md:pointer-fine:h-auto md:pointer-fine:appearance-auto md:pointer-fine:pr-1"
        >
          <option value="">
            {loading ? 'Loading views…' : 'Choose a saved view'}
          </option>
          {views.map((view) => (
            <option key={view.id} value={view.id}>
              {view.name}
            </option>
          ))}
        </select>
        <LuChevronDown
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 right-1 size-3 -translate-y-1/2 md:pointer-fine:hidden"
        />
      </span>
    </label>
  )
}

// SavedViewActions renders the load, save, rename, and delete actions.
function SavedViewActions({
  current,
  canSaveDraft,
  disabled,
  onLoad,
  onSaveNew,
  onSaveChanges,
  onRename,
  onDelete,
}: {
  current: SavedView | undefined
  canSaveDraft: boolean
  disabled: boolean
  onLoad: () => void
  onSaveNew: () => void
  onSaveChanges: () => void
  onRename: () => void
  onDelete: () => void
}) {
  return (
    <div className="flex flex-wrap gap-2">
      <SavedViewButton disabled={!current || disabled} onClick={onLoad}>
        Load view
      </SavedViewButton>
      <SavedViewButton disabled={!canSaveDraft || disabled} onClick={onSaveNew}>
        Save new view
      </SavedViewButton>
      <SavedViewButton
        disabled={!current || !canSaveDraft || disabled}
        onClick={onSaveChanges}
      >
        Save changes
      </SavedViewButton>
      <SavedViewButton disabled={!current || disabled} onClick={onRename}>
        Rename
      </SavedViewButton>
      <SavedViewButton disabled={!current || disabled} onClick={onDelete}>
        Delete
      </SavedViewButton>
    </div>
  )
}

// SavedViewStatus shows the saving state and the latest failure, with a retry
// action when the failure came from the shared views.
function SavedViewStatus({
  pending,
  message,
  onRetry,
}: {
  pending: boolean
  message: string | null | undefined
  onRetry?: () => void
}) {
  return (
    <>
      {pending && <p role="status">Saving shared view…</p>}
      {message && (
        <div role="alert">
          <p>{message}</p>
          {onRetry && (
            <SavedViewButton onClick={onRetry}>
              Retry shared views
            </SavedViewButton>
          )}
        </div>
      )}
    </>
  )
}

// SavedViewDialogs renders the name and delete confirmation dialogs.
function SavedViewDialogs({
  dialog,
  current,
  onConfirmName,
  onDelete,
  onClose,
}: {
  dialog: SavedViewDialog | null
  current: SavedView | undefined
  onConfirmName: (name: string) => void
  onDelete: () => void
  onClose: () => void
}) {
  const renaming = dialog === 'rename'
  const handleOpenChange = (open: boolean) => {
    if (!open) onClose()
  }

  return (
    <>
      <TextInputDialog
        open={dialog === 'save' || renaming}
        title={renaming ? 'Rename shared view' : 'Save shared view'}
        label="View name"
        defaultValue={renaming ? (current?.name ?? '') : ''}
        confirmLabel={renaming ? 'Rename' : 'Save for everyone'}
        requireValue
        onOpenChange={handleOpenChange}
        onConfirm={onConfirmName}
      />
      <ConfirmActionDialog
        open={dialog === 'delete'}
        title="Delete shared view"
        description={`Delete “${current?.name ?? ''}” for everyone? Your current filters stay here.`}
        confirmLabel="Delete view"
        onOpenChange={handleOpenChange}
        onConfirm={onDelete}
      />
    </>
  )
}

/** NotebookSavedViews shares explicit definitions while keeping selection and drafts personal. */
export default function NotebookSavedViews({
  world,
  notebook,
  handle,
  draft,
  onLoad,
}: NotebookSavedViewsProps) {
  // The selected definition has the same personal audience as the other Notes StateAtoms.
  const namespace = useStateNamespace(['notes', notebook])
  const [selected, setSelected] = useStateAtom(
    namespace,
    'selectedSavedView',
    '',
  )
  const [dialog, setDialog] = useState<SavedViewDialog | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const saved = useNotebookSavedViews(world, notebook, handle)
  const current = saved.views.find((view) => view.id === selected)
  const disabled = saved.pending || !saved.ready
  const canSaveDraft = !!draft.sourceRef

  // Loading is a personal action; incoming shared edits never replace a local draft.
  const load = () => {
    if (current) setLoadError(onLoad(current))
  }
  const confirmName = (name: string) => {
    if (dialog === 'rename' && current) {
      saved.rename({ id: current.id, name })
    } else {
      const id = crypto.randomUUID()
      saved.save({ ...draft, id, notebook, name })
      setSelected(id)
    }
    setDialog(null)
  }

  // Recovery and a missing selected view remain visible without applying stale filters.
  return (
    <details className="border-border border-b p-2 text-xs">
      <summary className="flex min-h-11 cursor-pointer items-center font-medium md:pointer-fine:min-h-0">
        Shared views
      </summary>
      <div className="mt-2 flex flex-col gap-2">
        <p className="text-muted-foreground">
          Save filters for everyone in this Notebook. Load a view to use it
          here.
        </p>
        <SavedViewSelect
          currentId={current?.id ?? ''}
          views={saved.views}
          loading={saved.loading}
          disabled={saved.loading || saved.pending}
          onSelect={(id) => {
            setSelected(id)
            setLoadError(null)
          }}
        />
        {selected && !current && !saved.loading && !saved.pending && (
          <p role="status">
            The selected view is no longer available. Choose another view or
            save your current filters.
          </p>
        )}
        <SavedViewActions
          current={current}
          canSaveDraft={canSaveDraft}
          disabled={disabled}
          onLoad={load}
          onSaveNew={() => setDialog('save')}
          onSaveChanges={() =>
            current &&
            saved.save({
              ...draft,
              id: current.id,
              notebook,
              name: current.name,
            })
          }
          onRename={() => setDialog('rename')}
          onDelete={() => setDialog('delete')}
        />
        <SavedViewStatus
          pending={saved.pending}
          message={saved.error?.message ?? loadError}
          onRetry={saved.error ? saved.retry : undefined}
        />
      </div>

      <SavedViewDialogs
        dialog={dialog}
        current={current}
        onConfirmName={confirmName}
        onDelete={() => {
          if (current) saved.remove({ id: current.id })
          setDialog(null)
        }}
        onClose={() => setDialog(null)}
      />
    </details>
  )
}
