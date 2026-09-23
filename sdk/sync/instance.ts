import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex } from '@noble/hashes/utils.js'
import type { IWorldState, WorldStateResource } from '../world/world-state.js'
import { accessObjectRootWorldState } from '../world/utils.js'
import { getObjectType, setObjectType } from '../world/types/types.js'

import type { AppDefinition } from './app.js'
import { SyncError } from './errors.js'
import {
  canonicalJSON,
  decodeJSON,
  encodeJSON,
  type JsonValue,
} from './json.js'
import type { Operation } from './operation.js'
import type { Principal, Schema } from './schema.js'
import { ApplicationTransaction, type CollectionLimits } from './transaction.js'

/** appObjectTypeID identifies the SDK's JSON leaf binding independently of plugin code. */
export function appObjectTypeID(schema: Pick<Schema, 'id'>): string {
  return `sync/app/${schema.id}`
}

/** AppRevision identifies the immutable executable that interprets an operation. */
export interface AppRevision {
  readonly pluginId: string
  readonly manifestRoot: string
}

/** AppInstance binds an ordinary typed World object to its data and executable. */
export interface AppInstance extends AppRevision {
  readonly application: string
  readonly version: number
  readonly instance: string
  readonly scope: string
  readonly creation: string
  readonly collections: readonly string[]
}

/** AppOperation carries intent; the host supplies identity and shared-data authority. */
export interface AppOperation {
  readonly objectKey: string
  readonly requestId: string
  readonly mutation?: Extract<Operation, { kind: 'mutate' }>
}

/** appLimits bounds records and materialized results on every plugin host. */
export const appLimits: CollectionLimits = Object.freeze({
  maxRecords: 10_000,
  maxSnapshotBytes: 8 * 1024 * 1024,
  maxRecordBytes: 256 * 1024,
})

/** pinnedOperationID records executable identity in accepted World history. */
export function pinnedOperationID(
  revision: AppRevision,
  handler: string,
): string {
  return `plugin-op/${revision.pluginId}/${revision.manifestRoot}/${encodeURIComponent(handler)}`
}

/** readAppInstance reads one revision-bound binding from the supplied snapshot. */
export async function readAppInstance(
  state: IWorldState,
  objectKey: string,
  schema: Pick<Schema, 'id'> & Partial<Pick<Schema, 'version'>>,
  signal?: AbortSignal,
): Promise<AppInstance> {
  const object = await state.getObject(objectKey, signal)
  if (!object)
    throw new SyncError('UNAVAILABLE', 'Application instance is unavailable')
  try {
    if (
      (await getObjectType(state, objectKey, signal)) !==
      appObjectTypeID(schema)
    ) {
      throw new SyncError(
        'SCHEMA_MISMATCH',
        'Object has a different application type',
      )
    }
    using cursor = await accessObjectRootWorldState(object, signal)
    const block = await cursor.getBlock({}, signal)
    const binding = decodeJSON(
      block.data ?? new Uint8Array(),
    ) as unknown as AppInstance
    if (
      !binding ||
      binding.application !== schema.id ||
      !Number.isSafeInteger(binding.version) ||
      binding.version < 1 ||
      (schema.version !== undefined && binding.version !== schema.version) ||
      !Array.isArray(binding.collections) ||
      binding.collections.some((name) => typeof name !== 'string' || !name) ||
      new Set(binding.collections).size !== binding.collections.length ||
      typeof binding.instance !== 'string' ||
      !binding.instance ||
      typeof binding.creation !== 'string' ||
      binding.scope !== 'shared' ||
      typeof binding.pluginId !== 'string' ||
      !binding.pluginId ||
      typeof binding.manifestRoot !== 'string' ||
      !binding.manifestRoot
    ) {
      throw new SyncError(
        'SCHEMA_MISMATCH',
        'Application binding does not match this definition',
      )
    }
    return binding
  } finally {
    object.release()
  }
}

/** parseAppOperation validates intent before the bridge supplies writable state. */
export function parseAppOperation(bytes: Uint8Array): AppOperation {
  if (bytes.length > appLimits.maxRecordBytes) {
    throw new SyncError('QUERY_LIMIT', 'Operation exceeds the input byte limit')
  }
  const value = decodeJSON(bytes) as unknown as AppOperation
  if (
    !value ||
    typeof value.objectKey !== 'string' ||
    !value.objectKey ||
    typeof value.requestId !== 'string' ||
    !value.requestId ||
    value.requestId.length > 128
  ) {
    throw new SyncError(
      'VALIDATION',
      'Object key and stable request ID are required',
    )
  }
  if (
    value.mutation &&
    (value.mutation.kind !== 'mutate' ||
      typeof value.mutation.name !== 'string')
  ) {
    throw new SyncError(
      'VALIDATION',
      'Only declared named mutations are accepted',
    )
  }
  return value
}

/**
 * executeAppOperation runs a trusted module against World's supplied transaction.
 * It never opens or accepts a World writer. The bridge authenticates sender;
 * callers cannot select another principal, scope, or collection binding.
 */
export async function executeAppOperation<S extends Schema>(
  app: AppDefinition<S>,
  revision: AppRevision,
  state: WorldStateResource,
  sender: string,
  create: boolean,
  request: AppOperation,
  signal: AbortSignal,
): Promise<JsonValue> {
  if (!sender)
    throw new SyncError('AUTHENTICATION', 'An authenticated sender is required')
  signal.throwIfAborted()
  let binding: AppInstance
  if (create) {
    binding = {
      ...revision,
      application: app.schema.id,
      version: app.schema.version,
      instance: bytesToHex(
        sha256(encodeJSON([sender, request.requestId, request.objectKey])),
      ),
      scope: 'shared',
      collections: Object.keys(app.schema.collections).sort(),
      creation: bytesToHex(sha256(encodeJSON(request.mutation ?? null))),
    }
    const existing = await state.getObject(request.objectKey, signal)
    if (existing) {
      existing.release()
      const current = await readAppInstance(
        state,
        request.objectKey,
        app.schema,
        signal,
      )
      if (canonicalJSON(current) !== canonicalJSON(binding)) {
        throw new SyncError(
          'CONFLICT',
          'Object key is already bound to another application instance',
        )
      }
    } else {
      using cursor = await state.buildStorageCursor(signal)
      const block = await cursor.putBlock({ data: encodeJSON(binding) }, signal)
      const storage = await cursor.getRef(signal)
      const object = await state.createObject(
        request.objectKey,
        { ...storage.ref, rootRef: block.ref },
        signal,
      )
      object.release()
      await setObjectType(
        state,
        request.objectKey,
        appObjectTypeID(app.schema),
        signal,
      )
    }
  } else {
    binding = await readAppInstance(
      state,
      request.objectKey,
      app.schema,
      signal,
    )
  }
  if (
    binding.pluginId !== revision.pluginId ||
    binding.manifestRoot !== revision.manifestRoot
  ) {
    throw new SyncError(
      'SCHEMA_MISMATCH',
      'Operation executable differs from the instance binding',
    )
  }
  if (!request.mutation) {
    if (!create)
      throw new SyncError('VALIDATION', 'A named mutation is required')
    return null
  }

  // The enclosing World capability grants shared collection access. Application
  // handlers receive only the declared collection facade, never the raw World.
  const principal: Principal = { subject: sender, scope: binding.scope }
  const unit = new ApplicationTransaction(state, {
    schema: app.schema,
    mutations: app.mutations,
    instance: binding.instance,
    revision: revision.manifestRoot,
    limits: appLimits,
    write: true,
    signal,
    authorize: async () => signal.throwIfAborted(),
  })
  try {
    const result = await unit.execute(
      principal,
      request.mutation,
      request.requestId,
    )
    await unit.flush()
    return result.value
  } finally {
    await unit.release()
  }
}
