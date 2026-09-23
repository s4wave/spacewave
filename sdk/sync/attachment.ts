import { Engine } from '../world/engine.js'
import type { WorldStateResource } from '../world/world-state.js'
import { WorldErrorCode } from '../world/world.pb.js'

import { createAccess, type DatabaseAccess } from './access.js'
import type { AppDefinition, AppMutation } from './app.js'
import { SyncError, type SyncErrorCode } from './errors.js'
import { canonicalJSON, encodeJSON, type JsonValue } from './json.js'
import {
  appLimits,
  pinnedOperationID,
  readAppInstance,
  type AppInstance,
  type AppOperation,
  type AppRevision,
} from './instance.js'
import { AppQueries } from './live-query.js'
import type { Operation } from './operation.js'
import type {
  LiveQuerySnapshot,
  QueryContext,
  QueryDefinition,
} from './query.js'
import type {
  CallOptions,
  Input,
  Principal,
  Schema,
  Validator,
} from './schema.js'
import type { AppUpgradeOperation } from './upgrade.js'
import { ApplicationTransaction } from './transaction.js'

/** AppAttachment retains an authorized Engine without taking ownership of its lifetime. */
export class AppAttachment<S extends Schema>
  implements QueryContext<S>, AsyncDisposable
{
  private readonly engine: Engine
  private readonly controller = new AbortController()
  private readonly access: DatabaseAccess<S>
  private readonly queries: AppQueries<S, Principal>
  private readonly pending = new Set<Promise<unknown>>()
  private closing?: Promise<void>

  constructor(
    engine: Engine,
    private readonly app: AppDefinition<S>,
    readonly objectKey: string,
    readonly binding: AppInstance,
    private readonly principal: Principal,
    private readonly signal?: AbortSignal,
  ) {
    this.binding = Object.freeze({ ...binding })
    this.engine = new Engine(engine.resourceRef.createRef(engine.id))
    this.access = createAccess((operation, options) => {
      const pending = this.dispatch(operation, options)
      this.pending.add(pending)
      void pending.then(
        () => this.pending.delete(pending),
        () => this.pending.delete(pending),
      )
      return pending
    })
    this.queries = new AppQueries({
      ...this.options(),
      engine: this.engine,
      principal,
      signal: this.lifetime(),
      checkState: (state, abort) => this.checkBinding(state, abort),
    })
  }

  /** collection exposes snapshot reads; all writes use named application operations. */
  collection<K extends keyof S['collections'] & string>(name: K) {
    const { get, scan } = this.access.collection(name)
    return { get, scan }
  }

  /** mutate validates intent and retains its identity through host-managed retries. */
  mutate<K extends keyof S['mutations'] & string>(
    name: K,
    input: Input<S['mutations'][K]['input']>,
    options?: CallOptions,
  ) {
    return this.access.mutate(name, input, options)
  }

  /** watch shares a function query within this instance and authenticated attachment. */
  watch<A extends Validator, R>(
    definition: QueryDefinition<S, A, R>,
    args: Input<A>,
    signal?: AbortSignal,
  ): AsyncIterable<LiveQuerySnapshot<R>> {
    return this.queries.watch(definition, args, signal)
  }

  /** close cancels readers and joins accepted writes before releasing its Engine ref. */
  close(): Promise<void> {
    return (this.closing ??= (async () => {
      this.controller.abort()
      await Promise.allSettled(this.pending)
      await this.queries.close()
      this.engine.release()
    })())
  }

  [Symbol.asyncDispose](): Promise<void> {
    return this.close()
  }

  /** options shares the same data contract with reads, queries, and receipt recovery. */
  private options() {
    return {
      schema: this.app.schema,
      mutations: this.app.mutations,
      instance: this.binding.instance,
      revision: this.binding.manifestRoot,
      limits: appLimits,
      authorize: async () => {
        this.lifetime().throwIfAborted()
      },
    }
  }

  /** lifetime combines the owner, caller, and local attachment cancellation. */
  private lifetime(signal?: AbortSignal): AbortSignal {
    return AbortSignal.any([
      this.controller.signal,
      ...[signal, this.signal].filter(
        (value): value is AbortSignal => value !== undefined,
      ),
    ])
  }

  /** checkBinding prevents an old viewer from continuing against a replaced schema or module. */
  private async checkBinding(
    state: WorldStateResource,
    signal: AbortSignal,
  ): Promise<void> {
    const current = await readAppInstance(
      state,
      this.objectKey,
      this.app.schema,
      signal,
    )
    if (canonicalJSON(current) !== canonicalJSON(this.binding)) {
      throw new SyncError(
        'SCHEMA_MISMATCH',
        'Application changed; reopen its current viewer',
      )
    }
  }

  /** dispatch accepts writes through World and reads their results from the retained receipt. */
  private async dispatch(
    operation: Operation,
    options: CallOptions = {},
  ): Promise<JsonValue> {
    const signal = this.lifetime(options.signal)
    signal.throwIfAborted()
    const requestId = options.requestId ?? crypto.randomUUID()
    if (operation.kind === 'put' || operation.kind === 'delete') {
      throw new SyncError('DENIED', 'Use a named application mutation')
    }
    if (operation.kind === 'mutate') {
      await acceptAppOperation(
        this.engine,
        this.binding,
        `${this.app.schema.id}/mutate`,
        {
          objectKey: this.objectKey,
          requestId,
          mutation: operation,
        },
        signal,
      )
    }

    // The accepted receipt and collection reads come from one committed snapshot.
    const tx = await this.engine.newTransaction(false, signal)
    const unit = new ApplicationTransaction(tx, {
      ...this.options(),
      write: false,
      signal,
    })
    try {
      await this.checkBinding(tx, signal)
      if (operation.kind === 'mutate') {
        const accepted = await unit.readAcceptance(
          this.principal,
          operation,
          requestId,
        )
        if (!accepted)
          throw new SyncError(
            'UNCERTAIN',
            'Accepted result is not yet available; retry the same request',
            requestId,
          )
        return accepted.value
      }
      return (await unit.execute(this.principal, operation, requestId)).value
    } finally {
      await unit.release()
      try {
        await tx.discard()
      } finally {
        tx.release()
      }
    }
  }
}

/** attachApp opens a typed app through the Engine capability supplied by its host or viewer. */
export async function attachApp<S extends Schema>(
  app: AppDefinition<S>,
  engine: Engine,
  objectKey: string,
  signal?: AbortSignal,
): Promise<AppAttachment<S>> {
  const info = await engine.getEngineInfo(signal)
  if (!info.sessionPeerId)
    throw new SyncError(
      'AUTHENTICATION',
      'Application attachment requires a session-bound World',
    )
  const tx = await engine.newTransaction(false, signal)
  try {
    const binding = await readAppInstance(tx, objectKey, app.schema, signal)
    return new AppAttachment(
      engine,
      app,
      objectKey,
      binding,
      { subject: info.sessionPeerId, scope: binding.scope },
      signal,
    )
  } finally {
    try {
      await tx.discard()
    } finally {
      tx.release()
    }
  }
}

/** createAppInstance accepts an instance and optional initial named mutation atomically. */
export async function createAppInstance<S extends Schema>(
  app: AppDefinition<S>,
  engine: Engine,
  revision: AppRevision,
  objectKey: string,
  options: CallOptions & {
    initial?: AppMutation<S>
  } = {},
): Promise<AppAttachment<S>> {
  const requestId = options.requestId ?? crypto.randomUUID()
  await acceptAppOperation(
    engine,
    revision,
    `${app.schema.id}/create`,
    {
      objectKey,
      requestId,
      ...(options.initial
        ? {
            mutation: options.initial as Extract<Operation, { kind: 'mutate' }>,
          }
        : {}),
    },
    options.signal,
  )
  return attachApp(app, engine, objectKey, options.signal)
}

/**
 * upgradeAppInstance accepts an explicit revision change and returns a new attachment.
 * The source binding is a concurrency precondition. Retry with the same binding
 * and request ID after uncertainty; existing attachments report a version conflict.
 */
export async function upgradeAppInstance<S extends Schema>(
  app: AppDefinition<S>,
  engine: Engine,
  revision: AppRevision,
  objectKey: string,
  from: AppInstance,
  options: CallOptions = {},
): Promise<AppAttachment<S>> {
  await acceptAppOperation(
    engine,
    revision,
    `${app.schema.id}/upgrade`,
    {
      objectKey,
      requestId: options.requestId ?? crypto.randomUUID(),
      from,
    },
    options.signal,
  )
  return attachApp(app, engine, objectKey, options.signal)
}

/** acceptAppOperation delegates retry to World's existing transaction owner. */
async function acceptAppOperation(
  engine: Engine,
  revision: AppRevision,
  handler: string,
  request: AppOperation | AppUpgradeOperation,
  signal?: AbortSignal,
): Promise<void> {
  const bytes = encodeJSON(request)
  signal?.throwIfAborted()
  try {
    const response = await engine.executeWorldOp(
      pinnedOperationID(revision, handler),
      bytes,
      signal,
    )
    if (response.errorCode === WorldErrorCode.UNHANDLED_OP) {
      throw new SyncError(
        'UNAVAILABLE',
        'The exact application executable is unavailable',
      )
    }
    if (response.rejectionCode) {
      throw new SyncError(
        response.rejectionCode as SyncErrorCode,
        response.rejectionMessage ?? 'Application operation was rejected',
      )
    }
    if (response.sysErr) {
      throw new SyncError(
        'UNCERTAIN',
        'Acceptance could not be confirmed; retry the same request',
        request.requestId,
      )
    }
    await engine.sync()
  } catch (error) {
    if (error instanceof SyncError) throw error
    throw new SyncError(
      'UNCERTAIN',
      'Acceptance could not be confirmed; retry the same request',
      request.requestId,
    )
  }
}
