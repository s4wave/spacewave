import type { BackendAPI } from '@aptre/bldr-sdk'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { getResourceCall } from '@aptre/bldr-sdk/resource/server/context.js'
import { Hash } from '../../net/hash/hash.pb.js'
import { base58Encode } from '../../net/peer/base58.js'
import {
  WorldOpRegistryResourceServiceClient,
  type WorldOpHandlerServiceHandler,
} from '../worldop/registry/registry_srpc.pb.js'
import { WorldStateResource } from '../world/world-state.js'

import type { AppDefinition } from './app.js'
import { SyncError } from './errors.js'
import {
  executeAppOperation,
  parseAppOperation,
  pinnedOperationID,
  type AppRevision,
} from './instance.js'
import type { Schema } from './schema.js'
import { executeAppUpgrade, type AppUpgradeOperation } from './upgrade.js'

/**
 * createAppPluginOperations composes typed operations into an existing plugin.
 * The caller installs the handler in its Resource mux before registering it,
 * retains registration refs, and releases them with the worker lifetime.
 * Historical workers expose the same handler without current registrations.
 */
export function createAppPluginOperations<S extends Schema>(
  api: BackendAPI,
  app: AppDefinition<S>,
  signal: AbortSignal,
) {
  // Resolve this worker's exact executable once, including historical workers.
  const info = api.pluginHost.GetPluginInfo({}, signal)
  const revision = info.then((info): AppRevision => {
    const hash = info.manifestRef?.manifestRef?.rootRef?.hash
    if (!info.pluginId || !hash) {
      throw new Error('Plugin has no immutable manifest')
    }
    return {
      pluginId: info.pluginId,
      manifestRoot: base58Encode(Hash.toBinary(hash)),
    }
  })
  const handles = (id: string) =>
    ['create', 'mutate', 'upgrade'].some(
      (action) => id === `${app.schema.id}/${action}`,
    )

  // Execute only against the attached World transaction and authenticated sender.
  const handler: WorldOpHandlerServiceHandler = {
    async ApplyWorldOp(request, abort, context) {
      if (!handles(request.operationTypeId ?? '')) {
        throw new Error('Unknown application operation')
      }
      const executable = await revision
      const intent = parseAppOperation(request.opData ?? new Uint8Array())
      using ref = getResourceCall(context).getAttachedRef(
        request.attachedWorldStateResourceId ?? 0,
      )
      try {
        const state = new WorldStateResource(ref)
        const lifetime = AbortSignal.any([signal, abort])
        if (request.operationTypeId === `${app.schema.id}/upgrade`) {
          await executeAppUpgrade(
            app,
            executable,
            state,
            request.sender ?? '',
            intent as AppUpgradeOperation,
            lifetime,
          )
        } else {
          await executeAppOperation(
            app,
            executable,
            state,
            request.sender ?? '',
            request.operationTypeId === `${app.schema.id}/create`,
            intent,
            lifetime,
          )
        }
        return {}
      } catch (error) {
        if (!(error instanceof SyncError)) throw error
        return { rejectionCode: error.code, rejectionMessage: error.message }
      }
    },
    async ApplyWorldObjectOp() {
      throw new Error('Application operations require World state')
    },
    async ValidateOp(request) {
      try {
        parseAppOperation(request.opData ?? new Uint8Array())
        return {}
      } catch {
        return { error: 'Invalid application operation' }
      }
    },
  }

  // The host resolves pinned operations even after the current worker is replaced.
  const register = async (
    root: ClientResourceRef,
    retain: (ref: ClientResourceRef) => void,
  ) => {
    const executable = await revision
    const operations = new WorldOpRegistryResourceServiceClient(root.client)
    for (const action of ['create', 'mutate', 'upgrade']) {
      // eslint-disable-next-line react-doctor/async-await-in-loop -- Retain each registration before another can fail.
      const response = await operations.RegisterWorldOp(
        {
          operationTypeId: pinnedOperationID(
            executable,
            `${app.schema.id}/${action}`,
          ),
          pluginId: executable.pluginId,
        },
        signal,
      )
      if (!response.resourceId)
        throw new Error('Application registration returned no Resource')
      retain(root.createRef(response.resourceId))
    }
  }
  return { info, revision, handles, handler, register }
}
