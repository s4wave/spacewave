import { createHandler, createMux } from 'starpc'
import type { Mux } from 'starpc'

import { ResourceServer } from '@aptre/bldr-sdk/resource/server/server.js'
import { getResourceCall } from '@aptre/bldr-sdk/resource/server/context.js'
import { encodeJSON, type JsonValue } from '../../sdk/sync/json.js'
import { toFailure, readInput } from '../../sdk/sync/wire.js'
import type { Principal, RecordEntry, Schema } from '../../sdk/sync/schema.js'
import {
  ApplicationDefinition,
  CollectionDefinition,
} from '../../sdk/sync/sync_srpc.pb.js'
import type {
  ApplicationHandler,
  CollectionHandler,
} from '../../sdk/sync/sync_srpc.pb.js'
import type { Snapshot } from '../../sdk/sync/sync.pb.js'
import type { Application } from './application.js'

// applicationResources exposes a capability tree bound to one authenticated principal.
export function applicationResources<S extends Schema, P extends Principal>(
  application: Application<S, P>,
  principal: P,
): Mux {
  const root = createMux()
  const handler: ApplicationHandler = {
    OpenCollection: async (request, _signal, context) => {
      try {
        const name = request.name ?? ''
        await application.openCollection(principal, name)
        const child = getResourceCall(context).constructChildResource(
          (signal) => ({
            mux: collectionMux(application, principal, name, signal),
            result: undefined,
          }),
        )
        return { resourceId: child.resourceId }
      } catch (error) {
        return { error: toFailure(error) }
      }
    },
    Mutate: async (request, signal) => {
      try {
        const result = await application.execute(
          principal,
          {
            kind: 'mutate',
            name: request.name ?? '',
            input: readInput(request.input),
          },
          { signal, requestId: request.requestId },
        )
        return { result: encodeJSON(result) }
      } catch (error) {
        return { error: toFailure(error) }
      }
    },
  }
  root.register(createHandler(ApplicationDefinition, handler))
  const resources = createMux()
  new ResourceServer(root).register(resources)
  return resources
}

function collectionMux<S extends Schema, P extends Principal>(
  application: Application<S, P>,
  principal: P,
  name: string,
  lifetime: AbortSignal,
): Mux {
  const handler: CollectionHandler = {
    Get: async (request, signal) => {
      try {
        const result = (await application.execute(
          principal,
          { kind: 'get', collection: name, key: request.key ?? '' },
          { signal: AbortSignal.any([lifetime, signal]) },
        )) as { found: boolean; value?: JsonValue }
        return {
          found: result.found,
          data: result.found ? encodeJSON(result.value) : undefined,
        }
      } catch (error) {
        return { error: toFailure(error) }
      }
    },
    Scan: async (request, signal) => {
      try {
        const result = (await application.execute(
          principal,
          { kind: 'scan', collection: name, prefix: request.prefix ?? '' },
          { signal: AbortSignal.any([lifetime, signal]) },
        )) as unknown as RecordEntry<JsonValue>[]
        return snapshot(result)
      } catch (error) {
        return { error: toFailure(error) }
      }
    },
    Put: async (request, signal) => {
      try {
        await application.execute(
          principal,
          {
            kind: 'put',
            collection: name,
            key: request.key ?? '',
            value: readInput(request.data),
          },
          {
            signal: AbortSignal.any([lifetime, signal]),
            requestId: request.requestId,
          },
        )
        return {}
      } catch (error) {
        return { error: toFailure(error) }
      }
    },
    Delete: async (request, signal) => {
      try {
        await application.execute(
          principal,
          { kind: 'delete', collection: name, key: request.key ?? '' },
          {
            signal: AbortSignal.any([lifetime, signal]),
            requestId: request.requestId,
          },
        )
        return {}
      } catch (error) {
        return { error: toFailure(error) }
      }
    },
    Watch: async function* (request, signal) {
      try {
        for await (const entries of application.watch(
          principal,
          name,
          request.prefix ?? '',
          AbortSignal.any([lifetime, signal]),
        ))
          yield snapshot(entries)
      } catch (error) {
        yield { error: toFailure(error) }
      }
    },
  }
  const mux = createMux()
  mux.register(createHandler(CollectionDefinition, handler))
  return mux
}

function snapshot(entries: readonly RecordEntry<JsonValue>[]): Snapshot {
  return {
    entries: entries.map((entry) => ({
      key: entry.key,
      data: encodeJSON(entry.value),
    })),
  }
}
