import { useMemo, useRef, useState } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { AppSource } from '../../sdk/sync/app.js'
import { SyncError, publicError } from '../../sdk/sync/errors.js'
import { canonicalJSON, decodeJSON, encodeJSON } from '../../sdk/sync/json.js'
import type {
  LiveQuerySnapshot,
  QueryDefinition,
} from '../../sdk/sync/query.js'
import type { Input, Output, Schema, Validator } from '../../sdk/sync/schema.js'

/** AppQueryState reports the initial read, a current snapshot, or a query failure. */
export type AppQueryState<R> =
  | { readonly status: 'pending' }
  | LiveQuerySnapshot<R>

/** useAppQuery shares a function query and releases this subscription on unmount. */
export function useAppQuery<S extends Schema, A extends Validator, R>(
  source: AppSource<S> | null,
  query: QueryDefinition<S, A, R>,
  args: Input<A>,
): AppQueryState<R> {
  // Canonical arguments keep inline objects from restarting the subscription.
  const key = canonicalJSON(args)
  const parent = useMemo<Resource<AppSource<S>>>(
    () => ({ value: source, loading: false, error: null, retry: () => {} }),
    [source],
  )
  const result = useStreamingResource(
    parent,
    (app, signal) => app.watch(query, JSON.parse(key) as Input<A>, signal),
    [source, query, key],
  )

  // A changed or missing source immediately hides the previous source's data.
  if (!source || result.loading) return { status: 'pending' }
  if (result.error) return { status: 'error', error: publicError(result.error) }
  return result.value ?? { status: 'pending' }
}

/** AppMutationState retains the request identity needed to retry uncertain acceptance. */
export type AppMutationState<R> =
  | { readonly status: 'idle' }
  | { readonly status: 'pending'; readonly requestId: string }
  | {
      readonly status: 'accepted'
      readonly requestId: string
      readonly value: R
    }
  | {
      readonly status: 'error'
      readonly requestId: string
      readonly error: SyncError
    }

/** useAppMutation owns one pending intent and retries it with the same identity. */
export function useAppMutation<
  S extends Schema,
  K extends keyof S['mutations'] & string,
>(source: AppSource<S> | null, name: K) {
  type Args = Input<S['mutations'][K]['input']>
  type Result = Output<S['mutations'][K]['output']>
  const [intent, setIntent] = useState<{
    source: AppSource<S>
    name: K
    input: Args
    requestId: string
  } | null>(null)
  const submitted = useRef(intent)
  const active =
    source && intent?.source === source && intent.name === name ? intent : null
  const result = useResource(
    async (signal) =>
      active
        ? active.source.mutate(active.name, active.input, {
            signal,
            requestId: active.requestId,
          })
        : null,
    [active],
  )

  // Only a user action creates an intent; resource retries preserve its bytes and ID.
  const submit = (input: Args) => {
    if (!source) throw new SyncError('UNAVAILABLE', 'Application is not ready')
    if (
      submitted.current?.source === source &&
      submitted.current.name === name &&
      (submitted.current !== active || result.loading)
    )
      throw new SyncError('CONFLICT', 'An operation is still pending')
    const next = {
      source,
      name,
      input: decodeJSON(encodeJSON(input)) as Args,
      requestId: crypto.randomUUID(),
    }
    submitted.current = next
    setIntent(next)
  }
  let state: AppMutationState<Result> = { status: 'idle' }
  if (active) {
    const requestId = active.requestId
    state = result.loading
      ? { status: 'pending', requestId }
      : result.error
        ? { status: 'error', requestId, error: publicError(result.error) }
        : { status: 'accepted', requestId, value: result.value as Result }
  }
  return { submit, state, retry: result.retry }
}
