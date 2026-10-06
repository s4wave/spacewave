import { useState } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { attachApp, createAppInstance } from '@s4wave/sdk/sync/attachment.js'
import { SyncError } from '@s4wave/sdk/sync/errors.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useAppAttachment } from '@s4wave/web/sync/useAppAttachment.js'
import { useAppMutation, useAppQuery } from '@s4wave/web/sync/app-hooks.js'
import { useObjectQuery } from '@s4wave/web/sync/useObjectQuery.js'

import type { NotebookHandle } from './sdk/notebook.js'
import {
  notebookSavedViews,
  notebookViews,
  notebookViewsKey,
  type SavedView,
} from './saved-views.js'

const objectExists = async () => true

interface FirstSave {
  view: SavedView
  requestId: string
  world: IWorldState
  handle: NotebookHandle
}

// saveFirstView creates the shared dataset and saves its first view in one
// transaction, or saves into the dataset another viewer created first.
async function saveFirstView(
  firstSave: FirstSave,
  key: string,
  signal: AbortSignal,
) {
  const engine = firstSave.world.getEngine?.()
  if (!engine) {
    throw new SyncError(
      'DENIED',
      'Open this Notebook in a live Space to save a shared view',
    )
  }
  const metadata = await firstSave.handle.getSavedViewsApp(signal)
  const options = { requestId: firstSave.requestId, signal }
  try {
    const created = await createAppInstance(
      notebookViews,
      engine,
      {
        pluginId: metadata.pluginId!,
        manifestRoot: metadata.manifestRoot!,
      },
      metadata.objectKey!,
      {
        ...options,
        initial: {
          kind: 'mutate',
          name: 'saveView',
          input: firstSave.view,
        },
      },
    )
    await created.close()
  } catch (error) {
    // Another viewer can win creation while this viewer is saving its first view.
    if (!(error instanceof SyncError) || error.code !== 'CONFLICT') throw error
    const current = await attachApp(notebookViews, engine, key, signal)
    try {
      await current.mutate('saveView', firstSave.view, options)
    } finally {
      await current.close()
    }
  }
}

// useFirstSavedView runs the first save of a Notebook that has no shared views
// dataset yet. The request keeps its ID so a retry is the same operation.
function useFirstSavedView(
  world: Resource<IWorldState>,
  handle: NotebookHandle | null,
  key: string,
) {
  const [firstSave, setFirstSave] = useState<FirstSave | null>(null)
  const creation = useResource(
    async (signal) => {
      if (
        !firstSave ||
        firstSave.world !== world.value ||
        firstSave.handle !== handle
      )
        return null
      await saveFirstView(firstSave, key, signal)
      return true
    },
    [firstSave, world.value, handle, key],
  )

  return {
    pending: firstSave !== null && creation.loading,
    error: creation.error,
    retry: creation.retry,
    request: (view: SavedView) => {
      if (world.value && handle) {
        setFirstSave({
          view,
          requestId: crypto.randomUUID(),
          world: world.value,
          handle,
        })
      }
    },
  }
}

// useSavedViewOperations binds the named save, rename, and delete operations
// and reports their combined pending and error state.
function useSavedViewOperations(app: ReturnType<typeof useAppAttachment>) {
  const save = useAppMutation(app.value, 'saveView')
  const rename = useAppMutation(app.value, 'renameView')
  const remove = useAppMutation(app.value, 'deleteView')
  const failed = [save, rename, remove].find(
    (operation) => operation.state.status === 'error',
  )

  return {
    save,
    rename,
    remove,
    pending: [save.state, rename.state, remove.state].some(
      (state) => state.status === 'pending',
    ),
    error: failed?.state.status === 'error' ? failed.state.error : null,
    retry: failed?.retry,
  }
}

/** useNotebookSavedViews creates shared data only on an explicit first save. */
export function useNotebookSavedViews(
  world: Resource<IWorldState>,
  notebook: string,
  handle: NotebookHandle | null,
) {
  // Existing datasets attach through the common query and operation bindings.
  const key = notebookViewsKey(notebook)
  const exists = useObjectQuery(world, key, objectExists)
  const app = useAppAttachment(world, notebookViews, exists.value ? key : '')
  const views = useAppQuery(app.value, notebookSavedViews, { notebook })
  const operations = useSavedViewOperations(app)
  const first = useFirstSavedView(world, handle, key)

  // The first failing source in precedence order is shown and retried.
  const fallbackRetry = operations.retry ?? app.retry
  const failure = [
    { error: exists.error, retry: exists.retry },
    { error: app.error, retry: app.retry },
    { error: first.error, retry: first.retry },
    {
      error: views.status === 'error' ? views.error : null,
      retry: fallbackRetry,
    },
    { error: operations.error, retry: fallbackRetry },
  ].find(({ error }) => error)

  // Query results remain the sole shared state; first-save intent retains its retry ID.
  return {
    views: views.status === 'current' ? views.value : [],
    loading: exists.loading || (exists.value && views.status === 'pending'),
    pending: first.pending || operations.pending,
    error: failure?.error ?? null,
    ready:
      !!world.value &&
      !world.value.getReadOnly() &&
      !!handle &&
      !exists.loading &&
      (!exists.value || !!app.value),
    save: (view: SavedView) => {
      if (app.value) {
        operations.save.submit(view)
      } else {
        first.request(view)
      }
    },
    rename: operations.rename.submit,
    remove: operations.remove.submit,
    retry: failure?.retry ?? fallbackRetry,
  }
}
