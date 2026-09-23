import { useState } from 'react'

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
  const [dialog, setDialog] = useState<'save' | 'rename' | 'delete' | null>(
    null,
  )
  const [loadError, setLoadError] = useState<string | null>(null)
  const saved = useNotebookSavedViews(world, notebook, handle)
  const current = saved.views.find((view) => view.id === selected)
  const disabled = saved.pending || !saved.ready

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
      <summary className="cursor-pointer font-medium">Shared views</summary>
      <div className="mt-2 flex flex-col gap-2">
        <p className="text-muted-foreground">
          Save filters for everyone in this Notebook. Load a view to use it
          here.
        </p>
        <label className="flex flex-col gap-1">
          Saved view
          <select
            value={current?.id ?? ''}
            disabled={saved.loading || saved.pending}
            onChange={(event) => {
              setSelected(event.target.value)
              setLoadError(null)
            }}
            className="border-border bg-background-primary rounded border p-1"
          >
            <option value="">
              {saved.loading ? 'Loading views…' : 'Choose a saved view'}
            </option>
            {saved.views.map((view) => (
              <option key={view.id} value={view.id}>
                {view.name}
              </option>
            ))}
          </select>
        </label>
        {selected && !current && !saved.loading && !saved.pending && (
          <p role="status">
            The selected view is no longer available. Choose another view or
            save your current filters.
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <button type="button" disabled={!current || disabled} onClick={load}>
            Load view
          </button>
          <button
            type="button"
            disabled={!draft.sourceRef || disabled}
            onClick={() => setDialog('save')}
          >
            Save new view
          </button>
          <button
            type="button"
            disabled={!current || !draft.sourceRef || disabled}
            onClick={() =>
              current &&
              saved.save({
                ...draft,
                id: current.id,
                notebook,
                name: current.name,
              })
            }
          >
            Save changes
          </button>
          <button
            type="button"
            disabled={!current || disabled}
            onClick={() => setDialog('rename')}
          >
            Rename
          </button>
          <button
            type="button"
            disabled={!current || disabled}
            onClick={() => setDialog('delete')}
          >
            Delete
          </button>
        </div>
        {saved.pending && <p role="status">Saving shared view…</p>}
        {(saved.error || loadError) && (
          <div role="alert">
            <p>{saved.error?.message ?? loadError}</p>
            {saved.error && (
              <button type="button" onClick={saved.retry}>
                Retry shared views
              </button>
            )}
          </div>
        )}
      </div>

      <TextInputDialog
        open={dialog === 'save' || dialog === 'rename'}
        title={dialog === 'rename' ? 'Rename shared view' : 'Save shared view'}
        label="View name"
        defaultValue={dialog === 'rename' ? (current?.name ?? '') : ''}
        confirmLabel={dialog === 'rename' ? 'Rename' : 'Save for everyone'}
        requireValue
        onOpenChange={(open) => {
          if (!open) setDialog(null)
        }}
        onConfirm={confirmName}
      />
      <ConfirmActionDialog
        open={dialog === 'delete'}
        title="Delete shared view"
        description={`Delete “${current?.name ?? ''}” for everyone? Your current filters stay here.`}
        confirmLabel="Delete view"
        onOpenChange={(open) => {
          if (!open) setDialog(null)
        }}
        onConfirm={() => {
          if (current) saved.remove({ id: current.id })
          setDialog(null)
        }}
      />
    </details>
  )
}
