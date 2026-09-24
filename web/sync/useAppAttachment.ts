import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { AppDefinition } from '../../sdk/sync/app.js'
import { attachApp, type AppAttachment } from '../../sdk/sync/attachment.js'
import { SyncError } from '../../sdk/sync/errors.js'
import type { Schema } from '../../sdk/sync/schema.js'
import type { IWorldState } from '../../sdk/world/world-state.js'

/** useAppAttachment binds a live viewer World and releases the app on replacement or close. */
export function useAppAttachment<S extends Schema>(
  world: Resource<IWorldState>,
  definition: AppDefinition<S>,
  objectKey: string,
): Resource<AppAttachment<S>> {
  return useResource(
    world,
    async (state, signal, cleanup) => {
      if (!objectKey) return null

      // Snapshot viewers cannot acquire the live Engine authority required by an app.
      const engine = state.getEngine?.()
      if (!engine) {
        throw new SyncError(
          'UNAVAILABLE',
          'Open this application in a live Space',
        )
      }
      const app = await attachApp(definition, engine, objectKey, signal)
      cleanup({
        [Symbol.dispose]: () => {
          void app.close()
        },
      })
      return app
    },
    [definition, objectKey],
  )
}
