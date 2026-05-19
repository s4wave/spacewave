import { useCallback, useEffect, useMemo, useState } from 'react'
import { useLatestRef, useWatchStateRpc } from '@aptre/bldr-react'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { StateAtom } from '@aptre/bldr-sdk/state/state.js'
import {
  WatchStateRequest,
  WatchStateResponse,
} from '@aptre/bldr-sdk/state/state.pb.js'
import superjson from 'superjson'

// AccessStateAtomFn is the function signature for accessing a state atom.
export type AccessStateAtomFn = (
  storeId: string,
  abortSignal?: AbortSignal,
) => Promise<StateAtom>

// StateAtomAccessor provides access to backend-persisted state atoms.
// Each call to accessStateAtom creates a separate StateAtom resource
// identified by storeId, stored as an individual ObjectStore key.
export type StateAtomAccessor = Resource<AccessStateAtomFn | null>

export type BackendStateAtomValue<T> = {
  value: T
  loading: boolean
  setValue: (update: T | ((prev: T) => T)) => void
}

type PendingBackendState<T> = {
  storeId: string
  stateJson: string
  value: T
}

// useBackendStateAtom provides access to a single backend-persisted state atom.
// The storeId identifies the atom (derived from namespace + key).
// Values are superjson-encoded for Date/Map/Set support.
// When the backend atom is still loading, updates are kept locally and flushed
// once the backend atom becomes available.
export function useBackendStateAtomValue<T>(
  accessor: StateAtomAccessor,
  storeId: string,
  defaultValue: T,
): BackendStateAtomValue<T> {
  // Lazily create the StateAtom resource for this storeId.
  const stateAtomResource = useResource<AccessStateAtomFn | null, StateAtom>(
    accessor,
    async (accessFn, signal, cleanup) => {
      if (!accessFn) return null
      const atom = await accessFn(storeId, signal)
      return cleanup(atom)
    },
    [storeId],
  )

  const stateAtom = stateAtomResource.value
  const [pendingState, setPendingState] =
    useState<PendingBackendState<T> | null>(null)

  // Watch for state changes via streaming RPC.
  const watchFn = useCallback(
    (req: WatchStateRequest, signal: AbortSignal) =>
      stateAtom ? stateAtom.watchState(req, signal) : null,
    [stateAtom],
  )
  const watchedState = useWatchStateRpc(
    watchFn,
    {},
    WatchStateRequest.equals,
    WatchStateResponse.equals,
  )
  const loading =
    accessor.loading ||
    stateAtomResource.loading ||
    (!!stateAtom && watchedState == null)

  // Parse the superjson state.
  const stateJson = watchedState?.stateJson
  const currentValue = useMemo<T>(() => {
    if (!stateJson) return defaultValue
    try {
      const parsed = superjson.parse(stateJson)
      if (parsed === undefined || parsed === null) return defaultValue
      return parsed as T
    } catch {
      return defaultValue
    }
  }, [stateJson, defaultValue])
  const pendingForStore =
    pendingState?.storeId === storeId ? pendingState : null
  const activePendingState =
    pendingForStore && stateJson !== pendingForStore.stateJson
      ? pendingForStore
      : null
  const value = activePendingState?.value ?? currentValue

  // Keep a ref to currentValue for the updater function.
  const currentValueLatest = useLatestRef(value)

  useEffect(() => {
    if (!stateAtom || !pendingForStore) return
    if (stateJson === pendingForStore.stateJson) return
    stateAtom.setState(pendingForStore.stateJson).catch(console.error)
  }, [stateAtom, stateJson, pendingForStore])

  // Update function: superjson-encode and send to backend.
  const setValue = useCallback(
    (update: T | ((prev: T) => T)) => {
      const newValue =
        typeof update === 'function'
          ? (update as (prev: T) => T)(currentValueLatest.current)
          : update
      setPendingState({
        storeId,
        stateJson: superjson.stringify(newValue),
        value: newValue,
      })
    },
    [storeId, currentValueLatest],
  )

  return {
    value,
    loading,
    setValue,
  }
}

export function useBackendStateAtom<T>(
  accessor: StateAtomAccessor,
  storeId: string,
  defaultValue: T,
): [T, (update: T | ((prev: T) => T)) => void] {
  const backendState = useBackendStateAtomValue(accessor, storeId, defaultValue)
  return [backendState.value, backendState.setValue]
}
