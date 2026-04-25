import { useStateAtom } from '@s4wave/web/state/persist.js'
import { stateDevToolsStateAtom } from './StateDevToolsContext.js'

export interface StateDevToolsPanelState {
  selectedAtomId: string | null
}

const DEFAULT_STATE: StateDevToolsPanelState = {
  selectedAtomId: null,
}

const stateDevToolsNamespace = {
  namespace: ['devtools', 'state'],
  stateAtom: stateDevToolsStateAtom,
}

// useStateDevToolsPanelState returns persisted panel state.
export function useStateDevToolsPanelState(): [
  StateDevToolsPanelState,
  (state: StateDevToolsPanelState) => void,
] {
  return useStateAtom<StateDevToolsPanelState>(
    stateDevToolsNamespace,
    'panel',
    DEFAULT_STATE,
  )
}
