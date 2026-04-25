import { useMemo, useCallback } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import { StateAtom } from '@aptre/bldr-sdk/state/state.js'
import {
  WatchStateRequest,
  WatchStateResponse,
} from '@aptre/bldr-sdk/state/state.pb.js'
import { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

// useStateAtomResource provides access to a persisted, cross-window synchronized state atom.
// The state is stored on the backend and synchronized across all connected clients.
//
// Returns [state, setState] similar to React.useState.
// - state: The current parsed state value, or defaultValue if not yet loaded
// - setState: Function to update the state (can take a value or updater function)
//
// Usage:
//   const stateAtomResource = useResource(rootResource, async (root, signal, cleanup) =>
//     root ? cleanup(await root.accessStateAtom({}, signal)) : null, [])
//   const [uiState, setUiState] = useStateAtomResource(stateAtomResource, { tabs: [] })
export function useStateAtomResource<T>(
  stateAtomResource: Resource<StateAtom | null>,
  defaultValue: T,
): [T, (update: T | ((prev: T) => T)) => void] {
  const stateAtom = stateAtomResource.value

  // Create memoized watch function
  const watchFn = useCallback(
    (req: WatchStateRequest, signal: AbortSignal) =>
      stateAtom ? stateAtom.watchState(req, signal) : null,
    [stateAtom],
  )

  // Watch for state changes
  const watchedState = useWatchStateRpc(
    watchFn,
    {},

    WatchStateRequest.equals,

    WatchStateResponse.equals,
  )

  // Parse the JSON state
  const stateJson = watchedState?.stateJson
  const currentValue = useMemo<T>(() => {
    if (!stateJson) return defaultValue
    try {
      return JSON.parse(stateJson) as T
    } catch {
      return defaultValue
    }
  }, [stateJson, defaultValue])

  // Update function
  const setValue = useCallback(
    (update: T | ((prev: T) => T)) => {
      if (!stateAtom) return

      // Get the new value - for updater functions, we need current value
      const newValue =
        typeof update === 'function' ?
          (update as (prev: T) => T)(currentValue)
        : update

      stateAtom.setState(JSON.stringify(newValue)).catch(console.error)
    },
    [stateAtom, currentValue],
  )

  return [currentValue, setValue]
}
