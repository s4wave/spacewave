import { useCallback, useSyncExternalStore, type ReactNode } from 'react'
import { type Atom, atomWithLocalStorage } from '@s4wave/web/state/persist.js'
import {
  StateAtomRegistryProvider,
  useStateAtomRegistryContext,
  type StateAtomRegistryContextValue,
  type InspectableStateAtom,
} from '@s4wave/web/state/StateAtomRegistry.js'
import type { StateType } from '@s4wave/web/state/persist.js'

export {
  useSelectedStateAtomId,
  useSelectedStatePath,
  useStateAtoms,
} from '@s4wave/web/state/StateAtomRegistry.js'
export type { StateAtomEntry } from '@s4wave/web/state/StateAtomRegistry.js'

export type StateDevToolsContextValue = StateAtomRegistryContextValue

// StateDevToolsProvider provides state atom tracking for DevTools.
export function StateDevToolsProvider({ children }: { children: ReactNode }) {
  return (
    <StateAtomRegistryProvider enabled={true}>
      {children}
    </StateAtomRegistryProvider>
  )
}

// useStateDevToolsContext returns the DevTools context, or null if not available.
export function useStateDevToolsContext() {
  return useStateAtomRegistryContext()
}

// useAtomValue subscribes to an atom's value changes.
const EMPTY_STATE_ATOM_VALUE = {}

export function useAtomValue(atom: InspectableStateAtom | null): unknown {
  const subscribe = useCallback(
    (callback: () => void) => {
      if (!atom) return () => {}
      return atom.subscribe(callback)
    },
    [atom],
  )

  const getSnapshot = useCallback(() => {
    if (!atom) return EMPTY_STATE_ATOM_VALUE
    return atom.get()
  }, [atom])

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

// devToolsStateAtom is a singleton persistent atom for State DevTools panel state.
export const stateDevToolsStateAtom: Atom<StateType> =
  atomWithLocalStorage<StateType>('spacewave-state-devtools', {})
