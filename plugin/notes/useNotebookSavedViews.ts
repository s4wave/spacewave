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
  const save = useAppMutation(app.value, 'saveView')
  const rename = useAppMutation(app.value, 'renameView')
  const remove = useAppMutation(app.value, 'deleteView')
  const [firstSave, setFirstSave] = useState<{
    view: SavedView
    requestId: string
    world: IWorldState
    handle: NotebookHandle
  } | null>(null)

  // The first save accepts creation and the named operation in one transaction.
  const creation = useResource(
    async (signal) => {
      if (
        !firstSave ||
        firstSave.world !== world.value ||
        firstSave.handle !== handle
      )
        return null
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
        if (!(error instanceof SyncError) || error.code !== 'CONFLICT')
          throw error
        const current = await attachApp(notebookViews, engine, key, signal)
        try {
          await current.mutate('saveView', firstSave.view, options)
        } finally {
          await current.close()
        }
      }
      return true
    },
    [firstSave, world.value, handle, key],
  )

  // Query results remain the sole shared state; first-save intent retains its retry ID.
  const pending =
    (firstSave !== null && creation.loading) ||
    [save.state, rename.state, remove.state].some(
      (state) => state.status === 'pending',
    )
  const operationError = [save, rename, remove].find(
    (operation) => operation.state.status === 'error',
  )
  const error =
    exists.error ??
    app.error ??
    creation.error ??
    (views.status === 'error' ? views.error : null) ??
    (operationError?.state.status === 'error'
      ? operationError.state.error
      : null)
  return {
    views: views.status === 'current' ? views.value : [],
    loading: exists.loading || (exists.value && views.status === 'pending'),
    pending,
    error,
    ready:
      !!world.value &&
      !world.value.getReadOnly() &&
      !!handle &&
      !exists.loading &&
      (!exists.value || !!app.value),
    save: (view: SavedView) => {
      if (app.value) {
        save.submit(view)
      } else if (world.value && handle) {
        setFirstSave({
          view,
          requestId: crypto.randomUUID(),
          world: world.value,
          handle,
        })
      }
    },
    rename: rename.submit,
    remove: remove.submit,
    retry: exists.error
      ? exists.retry
      : app.error
        ? app.retry
        : creation.error
          ? creation.retry
          : (operationError?.retry ?? app.retry),
  }
}
