import {
  defineSchema,
  type CallOptions,
  type Input,
  type Output,
  type Validator,
  type MutationHandlers,
  type Principal,
  type MutationContext,
  type Schema,
} from './schema.js'
import type {
  LiveQuerySnapshot,
  QueryContext,
  QueryDefinition,
} from './query.js'

/** AppDefinition contains the shared schema and named operations without starting a runtime. */
export interface AppDefinition<S extends Schema> {
  readonly schema: S
  readonly mutations: MutationHandlers<S, Principal>
  readonly migrations: Readonly<Record<number, AppMigration<S>>>
}

/** AppMigrationContext reads original values and writes the new schema in one World transaction. */
export interface AppMigrationContext<S extends Schema> extends MutationContext<
  S,
  Principal
> {
  /** previous exposes bounded original records; parse unknown values with the old schema. */
  readonly previous: QueryContext<Schema>
}

/** AppMigration explicitly converts a stored schema version to the declared version. */
export type AppMigration<S extends Schema> = (
  context: AppMigrationContext<S>,
) => Promise<void>

/** AppMutation pairs a named operation with its declared input type. */
export type AppMutation<S extends Schema> = {
  [K in keyof S['mutations'] & string]: {
    kind: 'mutate'
    name: K
    input: Input<S['mutations'][K]['input']>
  }
}[keyof S['mutations'] & string]

/**
 * defineApp binds typed handlers to a browser-safe schema. Importing a definition
 * opens no storage or connection; hosts bind it to an authorized World instance.
 * Validators may be any Standard Schema v1 implementation, including Zod.
 * Handlers remain functions in the bundled module and are never serialized.
 */
export function defineApp<const S extends Schema>(
  schema: S,
  mutations: MutationHandlers<S, Principal>,
  migrations: Readonly<Record<number, AppMigration<S>>> = {},
): AppDefinition<S> {
  return Object.freeze({ schema: defineSchema(schema), mutations, migrations })
}

/** AppSource is the host-independent read, query, and named-operation contract. */
export interface AppSource<S extends Schema> extends QueryContext<S> {
  /** mutate accepts a typed named operation with a stable retry identity. */
  mutate<K extends keyof S['mutations'] & string>(
    name: K,
    input: Input<S['mutations'][K]['input']>,
    options?: CallOptions,
  ): Promise<Output<S['mutations'][K]['output']>>

  /** watch streams snapshot-bound results for this attachment's lifetime. */
  watch<A extends Validator, R>(
    definition: QueryDefinition<S, A, R>,
    args: Input<A>,
    signal?: AbortSignal,
  ): AsyncIterable<LiveQuerySnapshot<R>>
}
