import type { WorldStateResource } from '../world/world-state.js'

import type { AppDefinition } from './app.js'
import { SyncError } from './errors.js'
import { canonicalJSON, encodeJSON } from './json.js'
import {
  appLimits,
  readAppInstance,
  type AppInstance,
  type AppOperation,
  type AppRevision,
} from './instance.js'
import { collectionKey } from './keys.js'
import {
  validate,
  type Principal,
  type Schema,
  type Validator,
} from './schema.js'
import {
  ApplicationTransaction,
  withTransactionLifetime,
} from './transaction.js'

/** AppUpgradeOperation explicitly selects the source binding to replace. */
export interface AppUpgradeOperation extends AppOperation {
  readonly from: AppInstance
}

/** originalValue leaves historical JSON unknown until a migration parses its old schema. */
const originalValue: Validator = {
  '~standard': {
    version: 1,
    vendor: 'spacewave',
    validate: (value) => ({ value }),
  },
}

/**
 * executeAppUpgrade stages data conversion, binding replacement, and the receipt
 * in World's supplied writer. A failed migration leaves acceptance to fail with
 * it; it never commits the World or opens another writer.
 */
export async function executeAppUpgrade<S extends Schema>(
  app: AppDefinition<S>,
  revision: AppRevision,
  state: WorldStateResource,
  sender: string,
  request: AppUpgradeOperation,
  signal: AbortSignal,
): Promise<void> {
  // Require an explicit source binding, never a guess at the currently installed code.
  if (!sender)
    throw new SyncError('AUTHENTICATION', 'An authenticated sender is required')
  signal.throwIfAborted()
  if (!request.from || request.mutation) {
    throw new SyncError(
      'VALIDATION',
      'An upgrade requires its source binding and no mutation',
    )
  }
  const current = await readAppInstance(
    state,
    request.objectKey,
    { id: app.schema.id },
    signal,
  )
  const principal: Principal = { subject: sender, scope: current.scope }
  const options = {
    schema: app.schema,
    mutations: app.mutations,
    instance: current.instance,
    revision: revision.manifestRoot,
    limits: appLimits,
    write: true,
    signal,
    authorize: async () => signal.throwIfAborted(),
  }
  const unit = new ApplicationTransaction(state, options)
  let original: ApplicationTransaction<Schema, Principal> | undefined
  try {
    // A retained receipt wins before checking the binding, including after another upgrade.
    await unit.upgrade(
      principal,
      { kind: 'upgrade', from: request.from },
      request.requestId,
      async (collections) => {
        if (canonicalJSON(current) !== canonicalJSON(request.from)) {
          throw new SyncError(
            'CONFLICT',
            'Application changed before this upgrade was accepted',
          )
        }
        if (
          current.pluginId !== revision.pluginId ||
          current.version > app.schema.version
        ) {
          throw new SyncError(
            'SCHEMA_MISMATCH',
            'An upgrade must retain its plugin family and cannot reverse a schema migration',
          )
        }

        // Incompatible schemas require a declared conversion; compatible updates only validate.
        const migration = app.migrations[current.version]
        if (current.version !== app.schema.version) {
          if (!migration)
            throw new SyncError(
              'SCHEMA_MISMATCH',
              'Declare a migration from the stored schema version',
            )
          original = new ApplicationTransaction(state, {
            ...options,
            write: false,
            schema: {
              id: app.schema.id,
              version: current.version,
              collections: Object.fromEntries(
                current.collections.map((name) => [name, originalValue]),
              ),
              mutations: {},
            },
            mutations: {},
          })
          const previous = original.collections(principal)
          await withTransactionLifetime(async (wrap) => {
            const reads = wrap(previous)
            await migration({
              ...wrap(collections),
              principal,
              scope: principal.scope,
              signal,
              previous: {
                collection: (name) => {
                  const { get, scan } = reads.collection(name)
                  return { get, scan }
                },
              },
            })
          })
        } else if (
          canonicalJSON(current.collections) !==
          canonicalJSON(Object.keys(app.schema.collections).sort())
        ) {
          throw new SyncError(
            'SCHEMA_MISMATCH',
            'Changing collection names requires a schema migration',
          )
        }

        // New validators must accept every surviving record without silently transforming it.
        for (const [name, validator] of Object.entries(
          app.schema.collections,
        )) {
          // eslint-disable-next-line react-doctor/async-await-in-loop -- Bound live materialization to one collection at a time.
          const entries = await collections.collection(name).scan()
          for (const entry of entries) {
            // eslint-disable-next-line react-doctor/async-await-in-loop -- Stop on the first invalid record without scheduling every validator.
            const value = await validate(
              validator,
              entry.value,
              'Migrated record',
            )
            if (canonicalJSON(value) !== canonicalJSON(entry.value)) {
              throw new SyncError(
                'SCHEMA_MISMATCH',
                'Migration must write the validated record representation',
              )
            }
          }
        }

        // Publish collection roots and the new binding into the same World acceptance.
        for (const name of current.collections) {
          if (!Object.hasOwn(app.schema.collections, name)) {
            // eslint-disable-next-line react-doctor/async-await-in-loop -- World mutations share the supplied writer.
            await state.deleteObject(
              collectionKey(current.instance, current.scope, name),
              signal,
            )
          }
        }
        using object = await state.getObject(request.objectKey, signal)
        // eslint-disable-next-line react-doctor/server-sequential-independent-await -- Register object disposal before another Resource acquisition can fail.
        using cursor = await state.buildStorageCursor(signal)
        const binding: AppInstance = {
          ...current,
          ...revision,
          version: app.schema.version,
          collections: Object.keys(app.schema.collections).sort(),
        }
        const [block, storage] = await Promise.all([
          cursor.putBlock({ data: encodeJSON(binding) }, signal),
          cursor.getRef(signal),
        ])
        await object!.setRootRef({ ...storage.ref, rootRef: block.ref }, signal)
      },
    )
    await unit.flush()
  } finally {
    await Promise.all([unit.release(), original?.release()])
  }
}
