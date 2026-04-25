import {
  useStateAtom,
  atomWithLocalStorage,
  type Atom,
  type StateType,
} from '@s4wave/web/state/persist.js'

export interface ResourceDevToolsPanelState {
  errorsExpanded: boolean
}

const DEFAULT_STATE: ResourceDevToolsPanelState = {
  errorsExpanded: true,
}

// devToolsStateAtom is a singleton persistent atom for DevTools panel state.
const devToolsStateAtom: Atom<StateType> = atomWithLocalStorage<StateType>(
  'spacewave-devtools-state',
  {},
)

// devToolsNamespace is the namespace used by useResourceDevToolsPanelState.
const devToolsNamespace = {
  namespace: ['devtools', 'resources'],
  stateAtom: devToolsStateAtom,
}

// useResourceDevToolsPanelState returns persisted panel state.
export function useResourceDevToolsPanelState(): [
  ResourceDevToolsPanelState,
  (state: ResourceDevToolsPanelState) => void,
] {
  return useStateAtom<ResourceDevToolsPanelState>(
    devToolsNamespace,
    'panel',
    DEFAULT_STATE,
  )
}
